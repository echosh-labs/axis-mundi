// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/server/server.go
Description: HTTP server implementation for Axis Mundi. Provides server lifecycle,
routing orchestration, in-memory registry caching, and database decoupling.
*/
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
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

// persistentState defines the structure for legacy JSON disk storage migration.
type persistentState struct {
	Mode     string            `json:"mode"`
	Statuses map[string]string `json:"statuses"`
}

// Server handles HTTP communication and TUI orchestration.
type Server struct {
	ws       *workspace.Service
	db       database.Store
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

// NewServer initializes the server with the workspace service and user context.
func NewServer(ws *workspace.Service, user *workspace.User) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := database.NewDB(dbFileName)
	if err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	return NewServerWithStore(ws, user, db, logger)
}

// NewServerWithStore initializes the server with an explicit database Store repository interface.
func NewServerWithStore(ws *workspace.Service, user *workspace.User, store database.Store, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	s := &Server{
		ws:              ws,
		db:              store,
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

// loadState restores mode/statuses from the database Store, migrating from JSON if necessary.
func (s *Server) loadState() {
	if s.db == nil {
		return
	}

	start := time.Now()

	// 1. Check if we need to migrate from legacy JSON
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

	s.logger.Info("state restored from store", "duration", time.Since(start), "items", len(s.statuses))
}

// migrateFromJSON reads legacy JSON state and persists it to the database Store.
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

func (s *Server) triggerStateSnapshot() {
	if s.db == nil {
		return
	}

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

// Start launches the HTTP server and background automation tickers.
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/notes/delete", s.handleDelete)
	mux.HandleFunc("/api/notes/detail", s.handleNoteDetail)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/user", s.handleUser)
	mux.HandleFunc("/api/sheets/detail", s.handleGetSheet)
	mux.HandleFunc("/api/sheets/delete", s.handleDeleteSheet)
	mux.HandleFunc("/api/docs/detail", s.handleGetDoc)
	mux.HandleFunc("/api/docs/delete", s.handleDeleteDoc)
	mux.HandleFunc("/api/gmail/detail", s.handleGetGmailThread)
	mux.HandleFunc("/api/gmail/delete", s.handleDeleteGmailThread)
	mux.HandleFunc("/api/calendar/detail", s.handleGetCalendarEvent)
	mux.HandleFunc("/api/calendar/delete", s.handleDeleteCalendarEvent)
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
