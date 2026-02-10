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
	user      *workspace.User
}

// NoteResponse for JSON delivery
type NoteResponse struct {
	Notes []workspace.Note `json:"notes"`
}

// UserResponse provides minimal operator context for the UI.
type UserResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	ID    string `json:"id"`
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.workspace.ListNotes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(NoteResponse{Notes: notes})
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	if s.user == nil {
		http.Error(w, "user profile unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UserResponse{Name: s.user.Name, Email: s.user.Email, ID: s.user.ID})
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
func StartServer(ws *workspace.Service, user *workspace.User) {
	s := &Server{workspace: ws, user: user}

	http.HandleFunc("/api/notes", s.handleListNotes)
	http.HandleFunc("/api/notes/delete", s.handleDeleteNote)
	http.HandleFunc("/api/user", s.handleUser)

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
