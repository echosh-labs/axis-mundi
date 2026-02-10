package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"axis/internal/workspace"
)

// Server handles UI delivery and API proxying
type Server struct {
	workspace *workspace.Service
}

// NoteResponse for JSON delivery
type NoteResponse struct {
	Notes []workspace.Note `json:"notes"`
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.workspace.ListNotes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(NoteResponse{Notes: notes})
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	err := s.workspace.DeleteNote(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StartServer initializes the routes and begins listening for HTTP requests
func StartServer(ws *workspace.Service) {
	s := &Server{workspace: ws}

	http.HandleFunc("/api/notes", s.handleListNotes)
	http.HandleFunc("/api/notes/delete", s.handleDeleteNote)

	// Serve static files (React build) from a web directory
	// Ensure this directory exists or adjust to your frontend build path
	fs := http.FileServer(http.Dir("./web/dist"))
	http.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Axis Terminal active at http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
