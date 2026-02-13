/*
File: internal/server/server.go
Description: HTTP server implementation for Axis Mundi. Handles API routing,
Server-Sent Events (SSE) for real-time telemetry, and persistent state management
for operational modes.
*/
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"axis/internal/workspace"
)

const stateFileName = "axis.state.json"

// SSEMessage wraps data with an optional event type.
type SSEMessage struct {
	Event string
	Data  []byte
}

// Server handles HTTP communication and TUI orchestration.
type Server struct {
	ws        *workspace.Service
	user      *workspace.User
	mode      string
	statuses  map[string]string
	modeMu    sync.RWMutex
	clients   map[chan SSEMessage]bool
	clientsMu sync.Mutex
}

// UserResponse provides minimal operator context for the UI.
type UserResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	ID    string `json:"id"`
}

// ModeResponse wraps the mode string for JSON output.
type ModeResponse struct {
	Mode string `json:"mode"`
}

// persistentState defines the structure for disk storage.
type persistentState struct {
	Mode     string            `json:"mode"`
	Statuses map[string]string `json:"statuses"`
}

// NewServer initializes the server with the workspace service and user context.
func NewServer(ws *workspace.Service, user *workspace.User) *Server {
	s := &Server{
		ws:       ws,
		user:     user,
		mode:     "AUTO", // Default safe state
		statuses: make(map[string]string),
		clients:  make(map[chan SSEMessage]bool),
	}
	s.loadState() // Attempt to restore state from disk
	return s
}

// loadState reads the configuration file and restores the mode.
func (s *Server) loadState() {
	data, err := os.ReadFile(stateFileName)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Warning: Failed to read state file: %v", err)
		}
		return
	}

	var ps persistentState
	if err := json.Unmarshal(data, &ps); err != nil {
		log.Printf("Warning: Corrupt state file: %v", err)
		return
	}

	if ps.Mode == "AUTO" || ps.Mode == "MANUAL" {
		s.mode = ps.Mode
		log.Printf("State restored: %s", s.mode)
	}
	if ps.Statuses != nil {
		s.statuses = ps.Statuses
		log.Printf("Item statuses restored: %d items", len(s.statuses))
	}
}

// saveState writes the current mode to disk.
// Note: Must be called while s.modeMu is locked.
func (s *Server) saveState() {
	ps := persistentState{
		Mode:     s.mode,
		Statuses: s.statuses,
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		log.Printf("Error marshaling state: %v", err)
		return
	}

	if err := os.WriteFile(stateFileName, data, 0644); err != nil {
		log.Printf("Error writing state file: %v", err)
	}
}

// Start launches the HTTP server and background automation ticker.
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/notes", s.handleNotes)
	mux.HandleFunc("/api/notes/delete", s.handleDelete)
	mux.HandleFunc("/api/notes/detail", s.handleNoteDetail)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/user", s.handleUser)
	mux.HandleFunc("/api/sheets", s.handleGetSheet)
	mux.HandleFunc("/api/sheets/delete", s.handleDeleteSheet)
	mux.HandleFunc("/api/docs", s.handleGetDoc)
	mux.HandleFunc("/api/docs/delete", s.handleDeleteDoc)
	mux.HandleFunc("/api/registry", s.handleRegistry)
	mux.HandleFunc("/api/status", s.handleStatus)

	// SSE Endpoint
	mux.HandleFunc("/api/events", s.handleEvents)

	// Static Asset Mounting
	fileServer := http.FileServer(http.Dir("./web/dist"))
	mux.Handle("/", fileServer)

	// Background Poller (The Heartbeat)
	go s.runPoller()

	log.Printf("Axis Server active on port %s (SSE Enabled)", port)
	return http.ListenAndServe(":"+port, mux)
}

func (s *Server) runPoller() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	remaining := 60
	for range ticker.C {
		s.modeMu.RLock()
		mode := s.mode
		s.modeMu.RUnlock()

		if mode == "AUTO" {
			remaining--
			s.broadcastTick(remaining)

			if remaining <= 0 {
				s.broadcastRegistry()
				remaining = 60
			}
		} else {
			remaining = 60
		}
	}
}

func (s *Server) broadcastTick(remaining int) {
	data := []byte(fmt.Sprintf(`{"seconds_remaining": %d}`, remaining))

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for clientChan := range s.clients {
		select {
		case clientChan <- SSEMessage{Event: "tick", Data: data}:
		default:
		}
	}
}

// enrichItems adds stored status to the registry items.
func (s *Server) enrichItems(items []workspace.RegistryItem) []workspace.RegistryItem {
	s.modeMu.Lock()
	defer s.modeMu.Unlock()

	modified := false
	enriched := make([]workspace.RegistryItem, len(items))
	for i, item := range items {
		enriched[i] = item
		if status, ok := s.statuses[item.ID]; ok {
			enriched[i].Status = status
		} else if item.Type == "keep" {
			enriched[i].Status = "Keep" // Default
			if s.statuses == nil {
				s.statuses = make(map[string]string)
			}
			s.statuses[item.ID] = "Keep"
			modified = true
		}
	}

	if modified {
		s.saveState()
	}
	return enriched
}

func (s *Server) broadcastRegistry() {
	rawItems, err := s.ws.ListRegistryItems()
	if err != nil {
		log.Printf("Error fetching registry for broadcast: %v", err)
		return
	}
	items := s.enrichItems(rawItems)

	data, err := json.Marshal(items)
	if err != nil {
		log.Printf("Error marshaling registry: %v", err)
		return
	}

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for clientChan := range s.clients {
		select {
		case clientChan <- SSEMessage{Data: data}:
		default:
			// If client channel is blocked, skip to prevent server blocking
		}
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// SSE Headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Register Client
	msgChan := make(chan SSEMessage, 10) // Buffer 10 to prevent slight blocking
	s.clientsMu.Lock()
	s.clients[msgChan] = true
	s.clientsMu.Unlock()

	// Cleanup on disconnect
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, msgChan)
		s.clientsMu.Unlock()
		close(msgChan)
	}()

	// Send initial state immediately
	go func() {
		rawItems, err := s.ws.ListRegistryItems()
		if err == nil {
			items := s.enrichItems(rawItems)
			data, _ := json.Marshal(items)
			msgChan <- SSEMessage{Data: data}
		}
	}()

	// Event Loop
	for {
		select {
		case msg := <-msgChan:
			if msg.Event != "" {
				fmt.Fprintf(w, "event: %s\n", msg.Event)
			}
			fmt.Fprintf(w, "data: %s\n\n", msg.Data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.ws.ListNotes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notes)
}

func (s *Server) handleNoteDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	note, err := s.ws.GetNote(context.Background(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(note); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.modeMu.RLock()
	currentMode := s.mode
	s.modeMu.RUnlock()

	if currentMode != "MANUAL" {
		http.Error(w, "delete requires MANUAL mode", http.StatusForbidden)
		return
	}

	if err := s.ws.DeleteNote(context.Background(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Immediate update to all clients
	go s.broadcastRegistry()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	newMode := r.URL.Query().Get("set")

	s.modeMu.Lock()
	defer s.modeMu.Unlock()

	// GET Request: Return current mode
	if newMode == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ModeResponse{Mode: s.mode})
		return
	}

	// SET Request: Update mode
	if newMode != "AUTO" && newMode != "MANUAL" {
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	s.mode = newMode
	s.saveState() // Persist to disk
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	if s.user == nil {
		http.Error(w, "user profile unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UserResponse{Name: s.user.Name, Email: s.user.Email, ID: s.user.ID})
}

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	rawItems, err := s.ws.ListRegistryItems()
	if err != nil {
		log.Printf("Error fetching registry items: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := s.enrichItems(rawItems)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")

	if id == "" || status == "" {
		http.Error(w, "missing id or status", http.StatusBadRequest)
		return
	}

	s.modeMu.Lock()
	if s.statuses == nil {
		s.statuses = make(map[string]string)
	}
	s.statuses[id] = status
	s.saveState()
	s.modeMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetSheet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	sheet, err := s.ws.GetSheet(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sheet); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteSheet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := s.ws.DeleteSheet(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	doc, err := s.ws.GetDoc(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(doc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := s.ws.DeleteDoc(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
