// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/server/server.go
Description: HTTP server implementation for Axis Mundi. Provides API routing,
Server-Sent Events (SSE) for telemetry, an in-memory registry cache, and
asynchronous persistence for operational state.
*/
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"axis/internal/database"
	"axis/internal/mcp"
	"axis/internal/workspace"
)

const (
	stateFileName    = "axis.state.json"
	dbFileName       = "axis.db"
	cacheTTL         = 5 * time.Minute
	persistInterval  = 10 * time.Second
	pollInterval     = 1 * time.Second
	autoRefreshTicks = 60
)

var allowedStatuses = map[string]bool{
	"Pending":  true,
	"Execute":  true,
	"Active":   true,
	"Blocked":  true,
	"Review":   true,
	"Complete": true,
	"Error":    true,
}

// RegistryCache stores the latest registry snapshot with a TTL.
type RegistryCache struct {
	items     []workspace.RegistryItem
	expiresAt time.Time
	mu        sync.RWMutex
}

// SSEMessage wraps data with an optional event type.
type SSEMessage struct {
	Event string
	Data  []byte
}

// persistentState defines the structure for disk storage.
type persistentState struct {
	Mode     string            `json:"mode"`
	Statuses map[string]string `json:"statuses"`
}

// Server handles HTTP communication and TUI orchestration.
type Server struct {
	ws       *workspace.Service
	db       *database.DB
	user     *workspace.User
	mode     string
	statuses map[string]string
	modeMu   sync.RWMutex

	registryCache RegistryCache

	clients   map[chan SSEMessage]bool
	clientsMu sync.Mutex
	logger    *slog.Logger

	telemetryBuffer chan string
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

// NewServer initializes the server with the workspace service and user context.
func NewServer(ws *workspace.Service, user *workspace.User) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := database.NewDB(dbFileName)
	if err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	s := &Server{
		ws:              ws,
		db:              db,
		user:            user,
		mode:            "AUTO",
		statuses:        make(map[string]string),
		clients:         make(map[chan SSEMessage]bool),
		logger:          logger,
		telemetryBuffer: make(chan string, 100),
	}
	s.loadState()
	return s
}

// loadState restores mode/statuses from SQLite, migrating from JSON if necessary.
func (s *Server) loadState() {
	start := time.Now()

	// 1. Check if we need to migrate from JSON
	if _, err := os.Stat(stateFileName); err == nil {
		s.logger.Info("found legacy state file, migrating to SQLite...")
		s.migrateFromJSON()
	}

	// 2. Load mode from DB
	mode, err := s.db.GetMode()
	if err != nil {
		s.logger.Error("failed to load mode from db", "error", err)
	} else {
		s.mode = mode
	}

	// 3. Load statuses from DB
	statuses, err := s.db.GetStatuses()
	if err != nil {
		s.logger.Error("failed to load statuses from db", "error", err)
	} else {
		s.statuses = statuses
	}

	s.logger.Info("state restored from SQLite", "duration", time.Since(start), "items", len(s.statuses))
}

// migrateFromJSON reads the legacy JSON state and persists it to SQLite.
func (s *Server) migrateFromJSON() {
	data, err := os.ReadFile(stateFileName)
	if err != nil {
		s.logger.Error("failed to read legacy state file", "error", err)
		return
	}

	var ps persistentState
	if err := json.Unmarshal(data, &ps); err != nil {
		s.logger.Error("corrupt legacy state file", "error", err)
		return
	}

	if ps.Mode != "" {
		if err := s.db.SetMode(ps.Mode); err != nil {
			s.logger.Error("failed to migrate mode", "error", err)
		}
	}

	if ps.Statuses != nil {
		for id, status := range ps.Statuses {
			// Migrate old state values to new ones
			if status == "Keep" || status == "Delete" {
				status = "Pending"
			}
			if _, ok := allowedStatuses[status]; !ok {
				status = "Pending"
			}
			if err := s.db.SetStatus(id, status); err != nil {
				s.logger.Error("failed to migrate status", "id", id, "error", err)
			}
		}
	}

	// Backup legacy file
	backupName := stateFileName + ".bak"
	if err := os.Rename(stateFileName, backupName); err != nil {
		s.logger.Error("failed to backup legacy state file", "error", err)
	} else {
		s.logger.Info("legacy state migrated and backed up", "backup", backupName)
	}
}

// Start launches the HTTP server and background automation ticker.
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/notes/delete", s.handleDelete)
	mux.HandleFunc("/api/notes/detail", s.handleNoteDetail)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/user", s.handleUser)
	mux.HandleFunc("/api/sheets/detail", s.handleGetSheet)
	mux.HandleFunc("/api/sheets/delete", s.handleDeleteSheet)
	mux.HandleFunc("/api/docs/detail", s.handleGetDoc)
	mux.HandleFunc("/api/docs/delete", s.handleDeleteDoc)
	mux.HandleFunc("/api/gmail/detail", s.handleGetGmailThread)
	mux.HandleFunc("/api/gmail/delete", s.handleDeleteGmailThread)
	mux.HandleFunc("/api/registry", s.handleRegistry)
	mux.HandleFunc("/api/status", s.handleStatus)
	// Google Chat Webhook
	mux.HandleFunc("/api/chat/webhook", s.handleChatWebhook)

	// SSE Endpoint
	mux.HandleFunc("/api/events", s.handleEvents)

	// MCP Endpoint
	mcpKey := os.Getenv("MCP_API_KEY")
	mcpSrv := mcp.NewServer(s.ws, s, mcpKey, s.logger)
	mcpSrv.RegisterRoutes(mux)

	// Static Asset Mounting
	fileServer := http.FileServer(http.Dir("./web/dist"))
	mux.Handle("/", fileServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.runPoller(ctx)
	go s.runTelemetryFlusher(ctx)

	s.logger.Info("axis server active", "port", port, "sse", true)
	return http.ListenAndServe(":"+port, mux)
}

func (s *Server) bufferTelemetry(msg string) {
	select {
	case s.telemetryBuffer <- msg:
	default:
		s.logger.Warn("telemetry buffer full, dropping message")
	}
}

// runTelemetryFlusher processes periodically batches telemetry events and sends them via Chat API.
func (s *Server) runTelemetryFlusher(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var batch []string

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.telemetryBuffer:
			batch = append(batch, msg)
		case <-ticker.C:
			if len(batch) > 0 {
				digest := "🔔 *System Telemetry Digest*\n"
				for _, m := range batch {
					digest += "- " + m + "\n"
				}
				err := s.ws.SendDirectMessage(s.user.Email, digest)
				if err != nil {
					s.logger.Error("failed to send telemetry dm", "error", err)
				}
				batch = nil // clear batch
			}
		}
	}
}

// runPoller processes periodic refreshes for AUTO mode.
func (s *Server) runPoller(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	remaining := autoRefreshTicks
	for {
		select {
		case <-ticker.C:
			s.modeMu.RLock()
			mode := s.mode
			s.modeMu.RUnlock()

			if mode == "AUTO" {
				remaining--
				s.broadcastTick(remaining)
				if remaining <= 0 {
					s.refreshRegistryCache()
					s.broadcastRegistry()
					remaining = autoRefreshTicks
				}
			} else {
				remaining = autoRefreshTicks
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) refreshRegistryCache() {
	start := time.Now()
	items, err := s.ws.ListRegistryItems()
	if err != nil {
		s.logger.Error("workspace fetch failed", "error", err)
		return
	}

	needsSnapshot := s.backfillStatuses(items)

	// Clean up statuses for notes that no longer exist
	if s.cleanupStaleStatuses(items) {
		needsSnapshot = true
	}

	s.registryCache.mu.Lock()
	s.registryCache.items = cloneItems(items)
	s.registryCache.expiresAt = time.Now().Add(cacheTTL)
	s.registryCache.mu.Unlock()

	if needsSnapshot {
		s.triggerStateSnapshot()
	}

	s.logger.Info("cache refreshed", "duration", time.Since(start), "count", len(items))
}

func (s *Server) cachedItemsFresh() ([]workspace.RegistryItem, bool) {
	s.registryCache.mu.RLock()
	defer s.registryCache.mu.RUnlock()
	fresh := time.Now().Before(s.registryCache.expiresAt)
	return cloneItems(s.registryCache.items), fresh
}

func cloneItems(items []workspace.RegistryItem) []workspace.RegistryItem {
	if len(items) == 0 {
		return nil
	}
	dup := make([]workspace.RegistryItem, len(items))
	copy(dup, items)
	return dup
}

func (s *Server) enrichItems(items []workspace.RegistryItem) []workspace.RegistryItem {
	s.modeMu.RLock()
	defer s.modeMu.RUnlock()

	res := make([]workspace.RegistryItem, len(items))
	for i, item := range items {
		res[i] = item
		if status, ok := s.statuses[item.ID]; ok {
			res[i].Status = status
		} else {
			res[i].Status = "Pending"
		}
	}
	return res
}

func (s *Server) broadcastRegistry() {
	items, _ := s.cachedItemsFresh()
	if len(items) == 0 {
		s.refreshRegistryCache()
		items, _ = s.cachedItemsFresh()
	}
	data, err := json.Marshal(s.enrichItems(items))
	if err != nil {
		s.logger.Error("registry marshal failed", "error", err)
		return
	}

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for clientChan := range s.clients {
		select {
		case clientChan <- SSEMessage{Data: data}:
		default:
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

func (s *Server) broadcastStatusChange(id, status, title string) {
	payload := map[string]string{
		"id":     id,
		"status": status,
		"title":  title,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("status change marshal failed", "error", err)
		return
	}

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for clientChan := range s.clients {
		select {
		case clientChan <- SSEMessage{Event: "status", Data: data}:
		default:
		}
	}
}

func (s *Server) triggerStateSnapshot() {
	s.modeMu.RLock()
	mode := s.mode
	statuses := make(map[string]string, len(s.statuses))
	for k, v := range s.statuses {
		statuses[k] = v
	}
	s.modeMu.RUnlock()

	// Persist mode
	if err := s.db.SetMode(mode); err != nil {
		s.logger.Error("failed to persist mode", "error", err)
	}

	// Persist statuses
	for id, status := range statuses {
		if err := s.db.SetStatus(id, status); err != nil {
			s.logger.Error("failed to persist status", "id", id, "error", err)
		}
	}
}

func (s *Server) isManualMode() bool {
	s.modeMu.RLock()
	defer s.modeMu.RUnlock()
	return s.mode == "MANUAL"
}

func (s *Server) getItemTitle(id string) string {
	s.registryCache.mu.RLock()
	defer s.registryCache.mu.RUnlock()
	for _, item := range s.registryCache.items {
		if item.ID == id {
			return item.Title
		}
	}
	return ""
}

// --- StatusManager interface for MCP ---

// GetStatus returns the current status for an item, or "" if unset.
func (s *Server) GetStatus(id string) string {
	s.modeMu.RLock()
	defer s.modeMu.RUnlock()
	return s.statuses[id]
}

// SetStatus sets the status for an item. Returns an error for invalid statuses.
// This mirrors the handleStatus HTTP handler behavior: updates in-memory state,
// broadcasts the change via SSE, persists to SQLite, and refreshes the registry.
func (s *Server) SetStatus(id, status string) error {
	if _, ok := allowedStatuses[status]; !ok {
		return fmt.Errorf("invalid status: %s", status)
	}

	s.modeMu.Lock()
	s.statuses[id] = status
	s.modeMu.Unlock()

	title := s.getItemTitle(id)
	if title != "" {
		s.broadcastStatusChange(id, status, title)

		if status == "Error" {
			s.bufferTelemetry(fmt.Sprintf("Item %s ('%s') transitioned to Error state", id, title))
		}
	}

	s.triggerStateSnapshot()
	s.broadcastRegistry()
	return nil
}

// ListStatuses returns all item IDs mapped to their current status.
func (s *Server) ListStatuses() map[string]string {
	s.modeMu.RLock()
	defer s.modeMu.RUnlock()
	result := make(map[string]string, len(s.statuses))
	for k, v := range s.statuses {
		result[k] = v
	}
	return result
}

// AllowedStatuses returns the ordered list of valid status values.
func (s *Server) AllowedStatuses() []string {
	return []string{"Pending", "Execute", "Active", "Blocked", "Review", "Complete", "Error"}
}

func (s *Server) backfillStatuses(items []workspace.RegistryItem) bool {
	needSnapshot := false
	s.modeMu.Lock()
	var newItems []workspace.RegistryItem
	for _, item := range items {
		if _, exists := s.statuses[item.ID]; exists {
			continue
		}
		s.statuses[item.ID] = "Pending"
		needSnapshot = true
		newItems = append(newItems, item)
	}
	s.modeMu.Unlock()

	// Broadcast telemetry for new items initialized to Pending
	for _, item := range newItems {
		s.broadcastStatusChange(item.ID, "Pending", item.Title)
	}

	return needSnapshot
}

// cleanupStaleStatuses removes statuses for items that no longer exist in the registry
func (s *Server) cleanupStaleStatuses(items []workspace.RegistryItem) bool {
	// Build a set of all current registry item IDs
	activeIDs := make(map[string]bool, len(items))
	for _, item := range items {
		activeIDs[item.ID] = true
	}

	needSnapshot := false
	s.modeMu.Lock()
	for id := range s.statuses {
		if !activeIDs[id] {
			delete(s.statuses, id)
			s.db.DeleteStatus(id)
			needSnapshot = true
			s.logger.Info("removed stale status", "id", id)
		}
	}
	s.modeMu.Unlock()
	return needSnapshot
}

func (s *Server) ensureStatusDefault(id, defaultStatus string) (string, bool) {
	s.modeMu.Lock()
	defer s.modeMu.Unlock()

	if status, ok := s.statuses[id]; ok {
		return status, false
	}

	s.statuses[id] = defaultStatus
	return defaultStatus, true
}

func (s *Server) statusForKeep(id string) string {
	status, created := s.ensureStatusDefault(id, "Pending")
	if created {
		s.triggerStateSnapshot()
	}
	return status
}

func (s *Server) ensureKeepNoteCached(id, title string) bool {
	if id == "" {
		return false
	}

	status, created := s.ensureStatusDefault(id, "Pending")
	needSnapshot := created
	added := false
	item := workspace.RegistryItem{
		ID:      id,
		Type:    "keep",
		Title:   sanitizeNoteTitle(title),
		Snippet: "Google Keep Note",
		Status:  status,
	}

	s.registryCache.mu.Lock()
	replaced := false
	for i := range s.registryCache.items {
		if s.registryCache.items[i].ID == id {
			s.registryCache.items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		s.registryCache.items = append(s.registryCache.items, item)
		added = true
	}
	s.registryCache.expiresAt = time.Now().Add(cacheTTL)
	s.registryCache.mu.Unlock()

	if needSnapshot {
		s.triggerStateSnapshot()
	}

	return added
}

func sanitizeNoteTitle(raw string) string {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "Untitled"
	}
	return t
}

func truthyParam(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "force", "refresh":
		return true
	default:
		return false
	}
}

func (s *Server) handleNoteDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
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

	s.refreshRegistryCache()
	s.broadcastRegistry()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	newMode := r.URL.Query().Get("set")

	s.modeMu.Lock()
	if newMode == "" {
		mode := s.mode
		s.modeMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ModeResponse{Mode: mode})
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
	json.NewEncoder(w).Encode(ModeResponse{Mode: newMode})
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
	manual := s.isManualMode()
	forceRefresh := manual && truthyParam(r.URL.Query().Get("refresh"))
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

	// Look up the note title for telemetry
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

	valuesResp, err := s.ws.GetSheetValues(id, "A1:Z100")
	var values [][]interface{}
	if err == nil && valuesResp != nil {
		values = valuesResp.Values
	}

	response := map[string]interface{}{
		"title":         sheet.Properties.Title,
		"spreadsheetId": sheet.SpreadsheetId,
		"values":        values,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
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

	if s.isManualMode() {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	} else {
		go s.refreshAndBroadcast()
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

	content := ""
	if doc.Body != nil {
		content = workspace.ExtractDocContent(doc.Body.Content)
	}

	response := map[string]interface{}{
		"title":      doc.Title,
		"documentId": doc.DocumentId,
		"content":    content,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
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

	if s.isManualMode() {
		s.refreshRegistryCache()
		s.broadcastRegistry()
	} else {
		go s.refreshAndBroadcast()
	}
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

	msgChan := make(chan SSEMessage, 10)
	s.clientsMu.Lock()
	s.clients[msgChan] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, msgChan)
		s.clientsMu.Unlock()
		close(msgChan)
	}()

	go s.sendInitialRegistrySnapshot(msgChan)

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

func (s *Server) handleGetGmailThread(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	thread, err := s.ws.GetGmailThread(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content := workspace.ExtractThreadContent(thread)

	response := map[string]interface{}{
		"title":    "Gmail Thread",
		"threadId": thread.Id,
		"content":  content,
		"raw":      thread,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDeleteGmailThread(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
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

func (s *Server) sendInitialRegistrySnapshot(ch chan<- SSEMessage) {
	items, fresh := s.cachedItemsFresh()
	if !fresh || len(items) == 0 {
		s.refreshRegistryCache()
		items, _ = s.cachedItemsFresh()
	}
	if len(items) == 0 {
		return
	}
	data, err := json.Marshal(s.enrichItems(items))
	if err != nil {
		s.logger.Error("initial snapshot marshal failed", "error", err)
		return
	}
	select {
	case ch <- SSEMessage{Data: data}:
	default:
	}
}

func (s *Server) refreshAndBroadcast() {
	s.refreshRegistryCache()
	s.broadcastRegistry()
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

// handleChatWebhook receives and processes events from Google Chat API.
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
		// Simple echo or acknowledgment logic.
		replyText := fmt.Sprintf("Axis Mundi received your message: %s", event.Message.Text)
		response = map[string]string{"text": replyText}
	case "REMOVED_FROM_SPACE":
		// We don't need to reply when removed.
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
