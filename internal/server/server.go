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

// Server handles HTTP communication and TUI orchestration.
type Server struct {
	ws        *workspace.Service
	user      *workspace.User
	mode      string
	modeMu    sync.RWMutex
	clients   map[chan []byte]bool
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
	Mode string `json:"mode"`
}

// NewServer initializes the server with the workspace service and user context.
func NewServer(ws *workspace.Service, user *workspace.User) *Server {
	s := &Server{
		ws:      ws,
		user:    user,
		mode:    "AUTO", // Default safe state
		clients: make(map[chan []byte]bool),
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
}

// saveState writes the current mode to disk.
// Note: Must be called while s.modeMu is locked.
func (s *Server) saveState() {
	ps := persistentState{Mode: s.mode}
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
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.modeMu.RLock()
		mode := s.mode
		s.modeMu.RUnlock()

		if mode == "AUTO" {
			s.broadcastNotes()
		}
	}
}

func (s *Server) broadcastNotes() {
	notes, err := s.ws.ListNotes()
	if err != nil {
		log.Printf("Error fetching notes for broadcast: %v", err)
		return
	}

	data, err := json.Marshal(notes)
	if err != nil {
		log.Printf("Error marshaling notes: %v", err)
		return
	}

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for clientChan := range s.clients {
		select {
		case clientChan <- data:
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
	msgChan := make(chan []byte, 1) // Buffer 1 to prevent slight blocking
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
		notes, err := s.ws.ListNotes()
		if err == nil {
			data, _ := json.Marshal(notes)
			msgChan <- data
		}
	}()

	// Event Loop
	for {
		select {
		case msg := <-msgChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
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
	go s.broadcastNotes()

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
