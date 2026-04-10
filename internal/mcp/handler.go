// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/mcp/handler.go
Description: MCP JSON-RPC handler. Implements the MCP protocol lifecycle including
initialization, resource listing/reading, and tool invocation. Integrates with the
workspace service for Keep note retrieval and formatting.
*/
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"axis/internal/workspace"

	keepapi "google.golang.org/api/keep/v1"
)

const (
	keepURIPrefix   = "keep://notes/"
	docsURIPrefix   = "docs://documents/"
	sheetsURIPrefix = "sheets://spreadsheets/"
	gmailURIPrefix  = "gmail://threads/"
	mcpServerName   = "axis-mundi"
	mcpVersion      = "0.1.0"
)

// Handler processes MCP JSON-RPC requests using the workspace service.
type Handler struct {
	ws       *workspace.Service
	statuses StatusManager
	logger   *slog.Logger
}

// NewHandler creates a new MCP handler backed by the given workspace service.
func NewHandler(ws *workspace.Service, statuses StatusManager, logger *slog.Logger) *Handler {
	return &Handler{
		ws:       ws,
		statuses: statuses,
		logger:   logger,
	}
}

// HandleRequest dispatches a raw JSON-RPC request to the appropriate method handler.
func (h *Handler) HandleRequest(ctx context.Context, raw json.RawMessage) *Response {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   &Error{Code: ErrCodeParse, Message: "parse error"},
		}
	}

	if req.JSONRPC != "2.0" {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeInvalidReq, Message: "invalid jsonrpc version"},
		}
	}

	switch req.Method {
	case "initialize":
		return h.handleInitialize(req)
	case "resources/list":
		return h.handleResourcesList(ctx, req)
	case "resources/read":
		return h.handleResourcesRead(ctx, req)
	case "tools/list":
		return h.handleToolsList(req)
	case "tools/call":
		return h.handleToolsCall(ctx, req)
	case "ping":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{}}
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeNoMethod, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

// handleInitialize returns server capabilities per the MCP spec.
func (h *Handler) handleInitialize(req Request) *Response {
	result := map[string]interface{}{
		"protocolVersion": ProtocolVersion,
		"serverInfo": ServerInfo{
			Name:    mcpServerName,
			Version: mcpVersion,
		},
		"capabilities": Capabilities{
			Resources: &ResourceCapability{ListChanged: false},
			Tools:     &ToolCapability{ListChanged: false},
		},
	}
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// handleResourcesList returns all workspace items as MCP resources.
func (h *Handler) handleResourcesList(_ context.Context, req Request) *Response {
	items, err := h.ws.ListRegistryItems()
	if err != nil {
		h.logger.Error("mcp: failed to list registry items", "error", err)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeInternal, Message: "failed to list resources"},
		}
	}

	resources := make([]Resource, 0, len(items))
	for _, item := range items {
		uri, mime := resourceURIAndMime(item)
		status := ""
		if h.statuses != nil {
			status = h.statuses.GetStatus(item.ID)
		}
		if status == "" {
			status = "Pending"
		}
		desc := item.Snippet
		if desc != "" {
			desc = fmt.Sprintf("[%s] %s", status, desc)
		} else {
			desc = fmt.Sprintf("[%s]", status)
		}
		resources = append(resources, Resource{
			URI:         uri,
			Name:        item.Title,
			Description: desc,
			MimeType:    mime,
		})
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"resources": resources},
	}
}

// handleResourcesRead retrieves a resource by URI and returns its content.
func (h *Handler) handleResourcesRead(ctx context.Context, req Request) *Response {
	params, err := decodeParams[ReadResourceParams](req.Params)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeBadParams, Message: "invalid params: " + err.Error()},
		}
	}

	var content string
	var readErr error

	switch {
	case strings.HasPrefix(params.URI, keepURIPrefix):
		noteID := strings.TrimPrefix(params.URI, keepURIPrefix)
		if noteID == "" {
			readErr = fmt.Errorf("empty note ID")
		} else {
			content, readErr = h.readKeepNote(ctx, noteID)
		}
	case strings.HasPrefix(params.URI, docsURIPrefix):
		docID := strings.TrimPrefix(params.URI, docsURIPrefix)
		if docID == "" {
			readErr = fmt.Errorf("empty document ID")
		} else {
			content, readErr = h.readDoc(docID)
		}
	case strings.HasPrefix(params.URI, sheetsURIPrefix):
		sheetID := strings.TrimPrefix(params.URI, sheetsURIPrefix)
		if sheetID == "" {
			readErr = fmt.Errorf("empty spreadsheet ID")
		} else {
			content, readErr = h.readSheet(sheetID)
		}
	case strings.HasPrefix(params.URI, gmailURIPrefix):
		threadID := strings.TrimPrefix(params.URI, gmailURIPrefix)
		if threadID == "" {
			readErr = fmt.Errorf("empty thread ID")
		} else {
			content, readErr = h.readGmailThread(threadID)
		}
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeBadParams, Message: "unsupported resource URI scheme"},
		}
	}

	if readErr != nil {
		h.logger.Error("mcp: failed to read resource", "uri", params.URI, "error", readErr)
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeInternal, Message: "failed to read resource"},
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"contents": []ResourceContent{
				{
					URI:      params.URI,
					MimeType: "text/plain",
					Text:     content,
				},
			},
		},
	}
}

// handleToolsList returns the available MCP tools.
func (h *Handler) handleToolsList(req Request) *Response {
	tools := []Tool{
		// Keep tools
		{
			Name:        "search_notes",
			Description: "Search Google Keep notes by keyword. Returns matching note titles and content snippets.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search keyword to filter notes by title or content",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "get_note",
			Description: "Retrieve the full content of a specific Google Keep note by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"noteId": map[string]interface{}{
						"type":        "string",
						"description": "The Keep note ID (e.g. 'notes/abc123' or just 'abc123')",
					},
				},
				"required": []string{"noteId"},
			},
		},
		{
			Name:        "list_notes",
			Description: "List all available Google Keep notes with titles and snippets.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		// Docs tools
		{
			Name:        "get_doc",
			Description: "Retrieve the plain text content of a Google Doc by its document ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"documentId": map[string]interface{}{
						"type":        "string",
						"description": "The Google Doc document ID",
					},
				},
				"required": []string{"documentId"},
			},
		},
		// Sheets tools
		{
			Name:        "get_sheet",
			Description: "Retrieve the title and cell values from a Google Sheet. Returns the first sheet's data from A1:Z100 by default.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"spreadsheetId": map[string]interface{}{
						"type":        "string",
						"description": "The Google Sheets spreadsheet ID",
					},
					"range": map[string]interface{}{
						"type":        "string",
						"description": "Cell range to read (default: A1:Z100)",
					},
				},
				"required": []string{"spreadsheetId"},
			},
		},
		// Gmail tools
		{
			Name:        "get_gmail_thread",
			Description: "Retrieve the full conversation content of a Gmail thread by thread ID, including all messages, headers, and bodies.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"threadId": map[string]interface{}{
						"type":        "string",
						"description": "The Gmail thread ID",
					},
				},
				"required": []string{"threadId"},
			},
		},
		// Registry tool
		{
			Name:        "list_workspace",
			Description: "List all items across Google Keep, Docs, Sheets, and Gmail. Returns a unified registry with type, title, snippet, and current status for each item.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		// Status tools
		{
			Name:        "get_status",
			Description: "Get the current workflow status of a workspace item by ID. Returns the status string (e.g. Pending, Execute, Active, Blocked, Review, Complete, Error).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "The item ID (e.g. 'notes/abc123', a doc ID, sheet ID, or Gmail thread ID)",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "set_status",
			Description: "Set the workflow status of a workspace item. Valid statuses: Pending, Execute, Active, Blocked, Review, Complete, Error.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "The item ID (e.g. 'notes/abc123', a doc ID, sheet ID, or Gmail thread ID)",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "The new status to set",
						"enum":        []string{"Pending", "Execute", "Active", "Blocked", "Review", "Complete", "Error"},
					},
				},
				"required": []string{"id", "status"},
			},
		},
		{
			Name:        "list_statuses",
			Description: "List the current workflow status of all tracked workspace items. Returns item IDs mapped to their status values.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"tools": tools},
	}
}

// handleToolsCall dispatches tool invocations.
func (h *Handler) handleToolsCall(ctx context.Context, req Request) *Response {
	params, err := decodeParams[CallToolParams](req.Params)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeBadParams, Message: "invalid params: " + err.Error()},
		}
	}

	switch params.Name {
	case "search_notes":
		return h.toolSearchNotes(ctx, req.ID, params.Arguments)
	case "get_note":
		return h.toolGetNote(ctx, req.ID, params.Arguments)
	case "list_notes":
		return h.toolListNotes(ctx, req.ID)
	case "get_doc":
		return h.toolGetDoc(req.ID, params.Arguments)
	case "get_sheet":
		return h.toolGetSheet(req.ID, params.Arguments)
	case "get_gmail_thread":
		return h.toolGetGmailThread(req.ID, params.Arguments)
	case "list_workspace":
		return h.toolListWorkspace(req.ID)
	case "get_status":
		return h.toolGetStatus(req.ID, params.Arguments)
	case "set_status":
		return h.toolSetStatus(req.ID, params.Arguments)
	case "list_statuses":
		return h.toolListStatuses(req.ID)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &Error{Code: ErrCodeNoMethod, Message: fmt.Sprintf("unknown tool: %s", params.Name)},
		}
	}
}

func (h *Handler) toolSearchNotes(ctx context.Context, id interface{}, args map[string]interface{}) *Response {
	query, _ := args["query"].(string)
	if query == "" {
		return toolError(id, "query parameter is required")
	}

	notes, err := h.ws.ListAllKeepNotes(ctx, workspace.ListNotesOptions{})
	if err != nil {
		h.logger.Error("mcp: search_notes failed", "error", err)
		return toolError(id, "failed to list notes")
	}

	queryLower := strings.ToLower(query)
	var matches []map[string]interface{}
	for _, note := range notes {
		if note.Trashed {
			continue
		}
		title := strings.ToLower(noteTitle(note))
		content := strings.ToLower(workspace.ExtractFullContent(note.Body))
		if strings.Contains(title, queryLower) || strings.Contains(content, queryLower) {
			matches = append(matches, map[string]interface{}{
				"noteId":  extractNoteID(note.Name),
				"title":   noteTitle(note),
				"snippet": noteSnippet(note),
			})
		}
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"matches": matches,
		"total":   len(matches),
	})

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

func (h *Handler) toolGetNote(ctx context.Context, id interface{}, args map[string]interface{}) *Response {
	noteID, _ := args["noteId"].(string)
	if noteID == "" {
		return toolError(id, "noteId parameter is required")
	}

	note, err := h.ws.GetNote(ctx, noteID)
	if err != nil {
		h.logger.Error("mcp: get_note failed", "noteID", noteID, "error", err)
		return toolError(id, "failed to retrieve note")
	}

	content := formatNoteContent(note)

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: content}},
		},
	}
}

func (h *Handler) toolListNotes(ctx context.Context, id interface{}) *Response {
	notes, err := h.ws.ListAllKeepNotes(ctx, workspace.ListNotesOptions{})
	if err != nil {
		h.logger.Error("mcp: list_notes failed", "error", err)
		return toolError(id, "failed to list notes")
	}

	var summaries []map[string]string
	for _, note := range notes {
		if note.Trashed {
			continue
		}
		summaries = append(summaries, map[string]string{
			"noteId":  extractNoteID(note.Name),
			"title":   noteTitle(note),
			"snippet": noteSnippet(note),
		})
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"notes": summaries,
		"total": len(summaries),
	})

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

// --- Docs, Sheets, Gmail Tool Handlers ---

func (h *Handler) toolGetDoc(id interface{}, args map[string]interface{}) *Response {
	docID, _ := args["documentId"].(string)
	if docID == "" {
		return toolError(id, "documentId parameter is required")
	}

	content, err := h.readDoc(docID)
	if err != nil {
		h.logger.Error("mcp: get_doc failed", "documentId", docID, "error", err)
		return toolError(id, "failed to retrieve document")
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: content}},
		},
	}
}

func (h *Handler) toolGetSheet(id interface{}, args map[string]interface{}) *Response {
	sheetID, _ := args["spreadsheetId"].(string)
	if sheetID == "" {
		return toolError(id, "spreadsheetId parameter is required")
	}

	readRange, _ := args["range"].(string)
	if readRange == "" {
		readRange = "A1:Z100"
	}

	content, err := h.readSheetRange(sheetID, readRange)
	if err != nil {
		h.logger.Error("mcp: get_sheet failed", "spreadsheetId", sheetID, "error", err)
		return toolError(id, "failed to retrieve spreadsheet")
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: content}},
		},
	}
}

func (h *Handler) toolGetGmailThread(id interface{}, args map[string]interface{}) *Response {
	threadID, _ := args["threadId"].(string)
	if threadID == "" {
		return toolError(id, "threadId parameter is required")
	}

	content, err := h.readGmailThread(threadID)
	if err != nil {
		h.logger.Error("mcp: get_gmail_thread failed", "threadId", threadID, "error", err)
		return toolError(id, "failed to retrieve gmail thread")
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: content}},
		},
	}
}

func (h *Handler) toolListWorkspace(id interface{}) *Response {
	items, err := h.ws.ListRegistryItems()
	if err != nil {
		h.logger.Error("mcp: list_workspace failed", "error", err)
		return toolError(id, "failed to list workspace items")
	}

	var entries []map[string]string
	for _, item := range items {
		uri, _ := resourceURIAndMime(item)
		status := ""
		if h.statuses != nil {
			status = h.statuses.GetStatus(item.ID)
		}
		if status == "" {
			status = "Pending"
		}
		entries = append(entries, map[string]string{
			"id":      item.ID,
			"type":    item.Type,
			"title":   item.Title,
			"snippet": item.Snippet,
			"status":  status,
			"uri":     uri,
		})
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"items": entries,
		"total": len(entries),
	})

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

// --- Status Tool Handlers ---

func (h *Handler) toolGetStatus(id interface{}, args map[string]interface{}) *Response {
	itemID, _ := args["id"].(string)
	if itemID == "" {
		return toolError(id, "id parameter is required")
	}

	if h.statuses == nil {
		return toolError(id, "status management not available")
	}

	status := h.statuses.GetStatus(itemID)
	if status == "" {
		status = "Pending"
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"id":     itemID,
		"status": status,
	})

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

func (h *Handler) toolSetStatus(id interface{}, args map[string]interface{}) *Response {
	itemID, _ := args["id"].(string)
	if itemID == "" {
		return toolError(id, "id parameter is required")
	}

	status, _ := args["status"].(string)
	if status == "" {
		return toolError(id, "status parameter is required")
	}

	if h.statuses == nil {
		return toolError(id, "status management not available")
	}

	if err := h.statuses.SetStatus(itemID, status); err != nil {
		h.logger.Error("mcp: set_status failed", "id", itemID, "status", status, "error", err)
		return toolError(id, "failed to set status: "+err.Error())
	}

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"id":     itemID,
		"status": status,
		"ok":     true,
	})

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

func (h *Handler) toolListStatuses(id interface{}) *Response {
	if h.statuses == nil {
		return toolError(id, "status management not available")
	}

	statuses := h.statuses.ListStatuses()
	allowed := h.statuses.AllowedStatuses()

	resultJSON, _ := json.Marshal(map[string]interface{}{
		"statuses":        statuses,
		"total":           len(statuses),
		"allowedStatuses": allowed,
	})

	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

// --- Resource Readers ---

func (h *Handler) readKeepNote(ctx context.Context, noteID string) (string, error) {
	note, err := h.ws.GetNote(ctx, noteID)
	if err != nil {
		return "", err
	}
	return formatNoteContent(note), nil
}

func (h *Handler) readDoc(docID string) (string, error) {
	doc, err := h.ws.GetDoc(docID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(doc.Title)
	b.WriteString("\n\n")

	if doc.Body != nil {
		b.WriteString(workspace.ExtractDocContent(doc.Body.Content))
	}

	return b.String(), nil
}

func (h *Handler) readSheet(sheetID string) (string, error) {
	return h.readSheetRange(sheetID, "A1:Z100")
}

func (h *Handler) readSheetRange(sheetID, readRange string) (string, error) {
	sheet, err := h.ws.GetSheet(sheetID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(sheet.Properties.Title)
	b.WriteString("\n\n")

	valuesResp, err := h.ws.GetSheetValues(sheetID, readRange)
	if err != nil {
		b.WriteString("[unable to read cell values]\n")
		return b.String(), nil
	}

	if valuesResp != nil {
		for _, row := range valuesResp.Values {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = fmt.Sprintf("%v", cell)
			}
			b.WriteString(strings.Join(cells, "\t"))
			b.WriteString("\n")
		}
	}

	return b.String(), nil
}

func (h *Handler) readGmailThread(threadID string) (string, error) {
	thread, err := h.ws.GetGmailThread(threadID)
	if err != nil {
		return "", err
	}
	return workspace.ExtractThreadContent(thread), nil
}

// --- Helpers ---

func resourceURIAndMime(item workspace.RegistryItem) (string, string) {
	switch item.Type {
	case "keep":
		return keepURIPrefix + extractNoteID(item.ID), "text/plain"
	case "doc":
		return docsURIPrefix + item.ID, "text/plain"
	case "sheet":
		return sheetsURIPrefix + item.ID, "text/plain"
	case "gmail":
		return gmailURIPrefix + item.ID, "text/plain"
	default:
		return "axis://" + item.Type + "/" + item.ID, "text/plain"
	}
}

func toolError(id interface{}, msg string) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolResult{
			Content: []ContentBlock{{Type: "text", Text: msg}},
			IsError: true,
		},
	}
}

func decodeParams[T any](raw interface{}) (T, error) {
	var result T
	data, err := json.Marshal(raw)
	if err != nil {
		return result, fmt.Errorf("failed to marshal params: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal params: %w", err)
	}
	return result, nil
}

func parseKeepURI(uri string) (string, bool) {
	if !strings.HasPrefix(uri, keepURIPrefix) {
		return "", false
	}
	noteID := strings.TrimPrefix(uri, keepURIPrefix)
	if noteID == "" {
		return "", false
	}
	return noteID, true
}

func extractNoteID(name string) string {
	return strings.TrimPrefix(name, "notes/")
}

func noteTitle(note *keepapi.Note) string {
	if note == nil {
		return "Untitled"
	}
	title := strings.TrimSpace(note.Title)
	if title == "" {
		return "Untitled"
	}
	return title
}

func noteSnippet(note *keepapi.Note) string {
	if note == nil || note.Body == nil {
		return ""
	}
	content := workspace.ExtractFullContent(note.Body)
	if len(content) > 200 {
		return content[:197] + "..."
	}
	return content
}

func formatNoteContent(note *keepapi.Note) string {
	if note == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(noteTitle(note))
	b.WriteString("\n\n")

	if note.CreateTime != "" {
		if t, err := time.Parse(time.RFC3339, note.CreateTime); err == nil {
			b.WriteString(fmt.Sprintf("Created: %s\n", t.Format("2006-01-02 15:04:05")))
		}
	}
	if note.UpdateTime != "" {
		if t, err := time.Parse(time.RFC3339, note.UpdateTime); err == nil {
			b.WriteString(fmt.Sprintf("Updated: %s\n", t.Format("2006-01-02 15:04:05")))
		}
	}
	b.WriteString("\n")

	content := workspace.ExtractFullContent(note.Body)
	if content != "" {
		b.WriteString(content)
	}

	return b.String()
}
