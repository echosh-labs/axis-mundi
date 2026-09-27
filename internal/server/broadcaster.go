// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/server/broadcaster.go
Description: Server-Sent Events (SSE) broadcaster, registry caching, and item status
management for Axis Mundi. Keeps web UI and external listeners synchronized.
*/
package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"axis/internal/workspace"
)

// SSEMessage wraps data with an optional event type.
type SSEMessage struct {
	Event string
	Data  []byte
}

func (s *Server) refreshRegistryCache() {
	start := time.Now()
	if s.ws == nil {
		return
	}
	items, err := s.ws.ListRegistryItems()
	if err != nil {
		s.logger.Error("workspace fetch failed", "error", err)
		return
	}

	needsSnapshot := s.backfillStatuses(items)

	// Clean up statuses for items that no longer exist
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

// --- StatusManager interface implementation for MCP ---

// GetStatus returns the current status for an item, or "" if unset.
func (s *Server) GetStatus(id string) string {
	s.modeMu.RLock()
	defer s.modeMu.RUnlock()
	return s.statuses[id]
}

// SetStatus sets the status for an item. Returns an error for invalid statuses.
// Updates in-memory state, broadcasts change via SSE, persists to database Store, and refreshes registry.
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
		defaultStatus := "Pending"
		if s.mode == "AUTO" && item.Type == "keep" {
			defaultStatus = "Execute"
		}
		s.statuses[item.ID] = defaultStatus
		needSnapshot = true
		newItems = append(newItems, item)
	}
	s.modeMu.Unlock()

	// Broadcast telemetry for new items initialized
	for _, item := range newItems {
		s.broadcastStatusChange(item.ID, s.statuses[item.ID], item.Title)
	}

	return needSnapshot
}

// cleanupStaleStatuses removes statuses for items that no longer exist in the registry
func (s *Server) cleanupStaleStatuses(items []workspace.RegistryItem) bool {
	activeIDs := make(map[string]bool, len(items))
	for _, item := range items {
		activeIDs[item.ID] = true
	}

	needSnapshot := false
	s.modeMu.Lock()
	for id := range s.statuses {
		if !activeIDs[id] {
			delete(s.statuses, id)
			if s.db != nil {
				s.db.DeleteStatus(id)
			}
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

	actualDefault := defaultStatus
	if s.mode == "AUTO" && strings.HasPrefix(id, "notes/") {
		actualDefault = "Execute"
	}

	s.statuses[id] = actualDefault
	return actualDefault, true
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
