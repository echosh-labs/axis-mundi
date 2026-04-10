// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/mcp/protocol.go
Description: Core MCP (Model Context Protocol) data structures and specifications.
Defines the wire format for serving Google Keep notes and other workspace content
to authorized cloud code agents.
*/
package mcp

import "time"

// ProtocolVersion is the MCP specification version implemented by this server.
const ProtocolVersion = "2025-03-26"

// ServerInfo identifies this MCP server implementation.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities advertises supported MCP features.
type Capabilities struct {
	Resources *ResourceCapability `json:"resources,omitempty"`
	Tools     *ToolCapability     `json:"tools,omitempty"`
}

// ResourceCapability describes resource serving capabilities.
type ResourceCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ToolCapability describes tool invocation capabilities.
type ToolCapability struct {
	ListChanged bool `json:"listChanged"`
}

// --- JSON-RPC 2.0 Envelope ---

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Error represents a JSON-RPC 2.0 error object.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	ErrCodeParse      = -32700
	ErrCodeInvalidReq = -32600
	ErrCodeNoMethod   = -32601
	ErrCodeBadParams  = -32602
	ErrCodeInternal   = -32603
)

// --- MCP Resources ---

// Resource describes an MCP-served resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent holds the text content for a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// --- MCP Tools ---

// Tool describes an invocable MCP tool.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"inputSchema"`
}

// ToolResult contains the output from a tool invocation.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single piece of tool output content.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// --- Keep-Specific MCP Structures ---

// KeepNoteResource is the MCP-formatted representation of a Google Keep note.
type KeepNoteResource struct {
	URI       string    `json:"uri"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	NoteType  string    `json:"noteType"`
	Created   time.Time `json:"created,omitempty"`
	Updated   time.Time `json:"updated,omitempty"`
	Trashed   bool      `json:"trashed"`
	ItemCount int       `json:"itemCount,omitempty"`
}

// --- Method Parameters ---

// ReadResourceParams are the parameters for resources/read.
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// CallToolParams are the parameters for tools/call.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// StatusManager abstracts item status operations so the MCP handler
// can read and write statuses without importing the server package.
type StatusManager interface {
	// GetStatus returns the current status for an item, or "" if unset.
	GetStatus(id string) string
	// SetStatus sets the status for an item. Returns an error for invalid statuses.
	SetStatus(id, status string) error
	// ListStatuses returns all item IDs mapped to their current status.
	ListStatuses() map[string]string
	// AllowedStatuses returns the ordered list of valid status values.
	AllowedStatuses() []string
}
