package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"axis/internal/workspace"
)

// Server handles HTTP communication and TUI orchestration.
type Server struct {
	ws        *workspace.Service
	mode      string
	modeMu    sync.RWMutex
	eventChan chan string
}

// NewServer initializes the server with the workspace service.
func NewServer(ws *workspace.Service) *Server {
	return &Server{
		ws:        ws,
		mode:      "AUTO",
		eventChan: make(chan string, 100),
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
	mux.HandleFunc("/api/stream", s.handleStream)

	// Static Asset Mounting
	// Serves index.html and associated assets from the /web directory
	fileServer := http.FileServer(http.Dir("web"))
	mux.Handle("/", fileServer)

	go s.runAutomation()

	log.Printf("Axis Server active on port %s", port)
	return http.ListenAndServe(":"+port, mux)
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
	s.broadcast(fmt.Sprintf("Manual Delete Success: %s", id))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	newMode := r.URL.Query().Get("set")
	if newMode != "AUTO" && newMode != "MANUAL" {
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	s.modeMu.Lock()
	s.mode = newMode
	s.modeMu.Unlock()
	s.broadcast(fmt.Sprintf("System Mode shifted to %s", newMode))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	for msg := range s.eventChan {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}
}

func (s *Server) broadcast(msg string) {
	select {
	case s.eventChan <- msg:
	default:
		// Drop message if buffer is full to prevent blocking
	}
}

func (s *Server) runAutomation() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.modeMu.RLock()
		active := s.mode == "AUTO"
		s.modeMu.RUnlock()

		if active {
			s.broadcast("Cyclical retraction: Monitoring Workspace state...")
		}
	}
}
