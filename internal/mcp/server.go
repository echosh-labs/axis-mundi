// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/mcp/server.go
Description: HTTP transport for the MCP server. Provides a Streamable HTTP endpoint
that accepts JSON-RPC 2.0 requests over POST and serves MCP-formatted Keep note
data to authorized cloud code agents.
*/
package mcp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"axis/internal/workspace"
)

const maxRequestSize = 1 << 20 // 1 MB

// Server wraps the MCP handler and provides HTTP endpoint registration.
type Server struct {
	handler *Handler
	apiKey  string
	logger  *slog.Logger
}

// NewServer creates an MCP server backed by the workspace service.
// If apiKey is non-empty, requests must include a matching Authorization bearer token.
func NewServer(ws *workspace.Service, apiKey string, logger *slog.Logger) *Server {
	return &Server{
		handler: NewHandler(ws, logger),
		apiKey:  apiKey,
		logger:  logger,
	}
}

// RegisterRoutes adds MCP endpoints to the given ServeMux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.handleMCP)
}

// handleMCP is the Streamable HTTP transport endpoint for MCP JSON-RPC.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.authenticate(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestSize))
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		http.Error(w, "empty request body", http.StatusBadRequest)
		return
	}

	resp := s.handler.HandleRequest(r.Context(), json.RawMessage(body))

	w.Header().Set("Content-Type", "application/json")
	if resp.Error != nil {
		s.logger.Warn("mcp request error",
			"method", extractMethod(body),
			"code", resp.Error.Code,
			"message", resp.Error.Message,
		)
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("mcp: failed to encode response", "error", err)
	}
}

func (s *Server) authenticate(r *http.Request) bool {
	if s.apiKey == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	return strings.TrimPrefix(auth, prefix) == s.apiKey
}

func extractMethod(raw []byte) string {
	var partial struct {
		Method string `json:"method"`
	}
	json.Unmarshal(raw, &partial)
	return partial.Method
}
