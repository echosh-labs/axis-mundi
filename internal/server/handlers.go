// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/server/handlers.go
Description: HTTP request handlers for Axis Mundi REST APIs. Serves system telemetry,
user context, workspace entities, operational mode controls, and Google Chat webhooks.
*/
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

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

// ChatEvent represents the inbound payload from Google Chat.
type ChatEvent struct {
	Type    string `json:"type"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Space struct {
		Name string `json:"name"`
	} `json:"space"`
	User struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"user"`
}

func truthyParam(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "force", "refresh":
		return true
	default:
		return false
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.modeMu.RLock()
	currentMode := s.mode
	s.modeMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "UP",
		"service": "axis-mundi",
		"mode":    currentMode,
		"port":    "8088",
	})
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	if s.user == nil {
		http.Error(w, "user profile unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(UserResponse{Name: s.user.Name, Email: s.user.Email, ID: s.user.ID})
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	newMode := r.URL.Query().Get("set")

	s.modeMu.Lock()
	if newMode == "" {
		mode := s.mode
		s.modeMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ModeResponse{Mode: mode})
		return
	}

	if newMode != "AUTO" && newMode != "MANUAL" {
		s.modeMu.Unlock()
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	s.mode = newMode
	s.modeMu.Unlock()

	if newMode == "MANUAL" {
		s.bufferTelemetry("Operational mode critically overridden to MANUAL by ui")
	}

	s.triggerStateSnapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ModeResponse{Mode: newMode})
}

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	forceRefresh := truthyParam(r.URL.Query().Get("refresh"))
	if forceRefresh {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	}

	items, fresh := s.cachedItemsFresh()
	if !fresh || len(items) == 0 {
		s.refreshRegistryCache()
		items, _ = s.cachedItemsFresh()
	}

	enriched := s.enrichItems(items)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(enriched); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")

	if id == "" || status == "" {
		http.Error(w, "missing id or status", http.StatusBadRequest)
		return
	}

	if _, ok := allowedStatuses[status]; !ok {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	s.modeMu.Lock()
	s.statuses[id] = status
	s.modeMu.Unlock()

	// Look up item title for telemetry
	title := s.getItemTitle(id)
	if title != "" {
		s.broadcastStatusChange(id, status, title)

		if status == "Error" {
			s.bufferTelemetry(fmt.Sprintf("Item %s ('%s') transitioned to Error state", id, title))
		}
	}

	s.triggerStateSnapshot()
	s.broadcastRegistry()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Disable write deadline for persistent SSE streaming
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	msgChan := make(chan SSEMessage, 10)
	s.clientsMu.Lock()
	s.clients[msgChan] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, msgChan)
		s.clientsMu.Unlock()
	}()

	go s.sendInitialRegistrySnapshot(msgChan)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case msg := <-msgChan:
			if msg.Event != "" {
				if _, err := fmt.Fprintf(w, "event: %s\n", msg.Event); err != nil {
					return
				}
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg.Data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleNoteDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	note, err := s.ws.GetNote(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if note != nil {
		added := s.ensureKeepNoteCached(note.Name, note.Title)
		if added {
			s.broadcastRegistry()
		}
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

	if !s.isManualMode() {
		http.Error(w, "delete requires MANUAL mode", http.StatusForbidden)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.ws.DeleteNote(context.Background(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.refreshRegistryCache()
	s.broadcastRegistry()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	detail, err := s.ws.GetDocDetail(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(detail); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.ws.DeleteDoc(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.isManualMode() {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	} else {
		go s.refreshAndBroadcast()
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetSheet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	detail, err := s.ws.GetSheetDetail(id, "A1:Z100")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(detail); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteSheet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.ws.DeleteSheet(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.isManualMode() {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	} else {
		go s.refreshAndBroadcast()
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetGmailThread(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	detail, err := s.ws.GetGmailThreadDetail(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(detail); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteGmailThread(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.ws.TrashGmailThread(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.isManualMode() {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	} else {
		go s.refreshAndBroadcast()
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetCalendarEvent(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	detail, err := s.ws.GetCalendarEventDetail(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(detail); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteCalendarEvent(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if s.ws == nil {
		http.Error(w, "workspace service unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.ws.DeleteCalendarEvent(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.isManualMode() {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	} else {
		go s.refreshAndBroadcast()
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleChatWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event ChatEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		s.logger.Error("failed to decode chat event", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.logger.Info("received chat event", "type", event.Type, "user", event.User.DisplayName)

	var response map[string]string

	switch event.Type {
	case "ADDED_TO_SPACE":
		response = map[string]string{"text": "Hello! I am Axis Mundi. I am ready to assist."}
	case "MESSAGE":
		replyText := fmt.Sprintf("Axis Mundi received your message: %s", event.Message.Text)
		response = map[string]string{"text": replyText}
	case "REMOVED_FROM_SPACE":
		w.WriteHeader(http.StatusOK)
		return
	default:
		s.logger.Warn("unknown chat event type", "type", event.Type)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("failed to encode chat response", "error", err)
	}
}
