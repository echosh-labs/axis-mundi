// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"axis/internal/workspace"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func testHandler() *Handler {
	return NewHandler(nil, nil, testLogger())
}

func call(t *testing.T, h *Handler, method string, params interface{}) *Response {
	t.Helper()
	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return h.HandleRequest(context.Background(), json.RawMessage(raw))
}

func TestInitialize(t *testing.T) {
	h := testHandler()
	resp := call(t, h, "initialize", nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("expected protocol version %s, got %v", ProtocolVersion, result["protocolVersion"])
	}

	info, ok := result["serverInfo"].(ServerInfo)
	if !ok {
		t.Fatal("serverInfo missing or wrong type")
	}
	if info.Name != mcpServerName {
		t.Errorf("expected server name %s, got %s", mcpServerName, info.Name)
	}
}

func TestPing(t *testing.T) {
	h := testHandler()
	resp := call(t, h, "ping", nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestInvalidJSON(t *testing.T) {
	h := testHandler()
	resp := h.HandleRequest(context.Background(), json.RawMessage(`{invalid`))

	if resp.Error == nil {
		t.Fatal("expected parse error")
	}
	if resp.Error.Code != ErrCodeParse {
		t.Errorf("expected error code %d, got %d", ErrCodeParse, resp.Error.Code)
	}
}

func TestInvalidVersion(t *testing.T) {
	h := testHandler()
	raw := `{"jsonrpc":"1.0","id":1,"method":"ping"}`
	resp := h.HandleRequest(context.Background(), json.RawMessage(raw))

	if resp.Error == nil {
		t.Fatal("expected invalid version error")
	}
	if resp.Error.Code != ErrCodeInvalidReq {
		t.Errorf("expected error code %d, got %d", ErrCodeInvalidReq, resp.Error.Code)
	}
}

func TestMethodNotFound(t *testing.T) {
	h := testHandler()
	resp := call(t, h, "nonexistent/method", nil)

	if resp.Error == nil {
		t.Fatal("expected method not found error")
	}
	if resp.Error.Code != ErrCodeNoMethod {
		t.Errorf("expected error code %d, got %d", ErrCodeNoMethod, resp.Error.Code)
	}
}

func TestToolsList(t *testing.T) {
	h := testHandler()
	resp := call(t, h, "tools/list", nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	tools, ok := result["tools"].([]Tool)
	if !ok {
		t.Fatal("tools missing or wrong type")
	}

	expectedTools := map[string]bool{
		"search_notes":     false,
		"get_note":         false,
		"list_notes":       false,
		"get_doc":          false,
		"get_sheet":        false,
		"get_gmail_thread":   false,
		"get_calendar_event": false,
		"list_workspace":     false,
		"get_status":         false,
		"set_status":         false,
		"list_statuses":      false,
	}

	for _, tool := range tools {
		if _, exists := expectedTools[tool.Name]; exists {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %q not found", name)
		}
	}
}

func TestParseKeepURI(t *testing.T) {
	tests := []struct {
		uri    string
		wantID string
		wantOK bool
	}{
		{"keep://notes/abc123", "abc123", true},
		{"keep://notes/some-long-id", "some-long-id", true},
		{"keep://notes/", "", false},
		{"https://example.com/notes/abc", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		id, ok := parseKeepURI(tt.uri)
		if ok != tt.wantOK {
			t.Errorf("parseKeepURI(%q): ok=%v, want %v", tt.uri, ok, tt.wantOK)
		}
		if id != tt.wantID {
			t.Errorf("parseKeepURI(%q): id=%q, want %q", tt.uri, id, tt.wantID)
		}
	}
}

func TestResourceURIAndMime(t *testing.T) {
	tests := []struct {
		item    workspace.RegistryItem
		wantURI string
	}{
		{workspace.RegistryItem{ID: "notes/abc", Type: "keep"}, "keep://notes/abc"},
		{workspace.RegistryItem{ID: "doc123", Type: "doc"}, "docs://documents/doc123"},
		{workspace.RegistryItem{ID: "sheet456", Type: "sheet"}, "sheets://spreadsheets/sheet456"},
		{workspace.RegistryItem{ID: "thread789", Type: "gmail"}, "gmail://threads/thread789"},
		{workspace.RegistryItem{ID: "event000", Type: "calendar"}, "calendar://events/event000"},
	}

	for _, tt := range tests {
		uri, mime := resourceURIAndMime(tt.item)
		if uri != tt.wantURI {
			t.Errorf("resourceURIAndMime(%s): uri=%q, want %q", tt.item.Type, uri, tt.wantURI)
		}
		if mime != "text/plain" {
			t.Errorf("resourceURIAndMime(%s): mime=%q, want text/plain", tt.item.Type, mime)
		}
	}
}

func TestExtractNoteID(t *testing.T) {
	if got := extractNoteID("notes/abc123"); got != "abc123" {
		t.Errorf("expected abc123, got %s", got)
	}
	if got := extractNoteID("abc123"); got != "abc123" {
		t.Errorf("expected abc123, got %s", got)
	}
}

func TestFormatNoteContent(t *testing.T) {
	content := formatNoteContent(nil)
	if content != "" {
		t.Errorf("expected empty string for nil note, got %q", content)
	}
}

func TestHTTPEndpoint_MethodNotAllowed(t *testing.T) {
	srv := NewServer(nil, nil, "", testLogger())
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rr := httptest.NewRecorder()

	srv.handleMCP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHTTPEndpoint_EmptyBody(t *testing.T) {
	srv := NewServer(nil, nil, "", testLogger())
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(""))
	rr := httptest.NewRecorder()

	srv.handleMCP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHTTPEndpoint_Auth(t *testing.T) {
	srv := NewServer(nil, nil, "secret-key", testLogger())

	// No auth header
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	rr := httptest.NewRecorder()
	srv.handleMCP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rr.Code)
	}

	// Wrong key
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer wrong-key")
	rr = httptest.NewRecorder()
	srv.handleMCP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong key, got %d", rr.Code)
	}

	// Correct key
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer secret-key")
	rr = httptest.NewRecorder()
	srv.handleMCP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with correct key, got %d", rr.Code)
	}
}

func TestHTTPEndpoint_ValidRequest(t *testing.T) {
	srv := NewServer(nil, nil, "", testLogger())

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handleMCP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp Response
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error.Message)
	}
}

// --- Mock StatusManager ---

type mockStatusManager struct {
	statuses map[string]string
}

func newMockStatusManager() *mockStatusManager {
	return &mockStatusManager{statuses: make(map[string]string)}
}

func (m *mockStatusManager) GetStatus(id string) string {
	return m.statuses[id]
}

func (m *mockStatusManager) SetStatus(id, status string) error {
	allowed := map[string]bool{
		"Pending": true, "Execute": true, "Active": true,
		"Blocked": true, "Review": true, "Complete": true, "Error": true,
	}
	if !allowed[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	m.statuses[id] = status
	return nil
}

func (m *mockStatusManager) ListStatuses() map[string]string {
	result := make(map[string]string, len(m.statuses))
	for k, v := range m.statuses {
		result[k] = v
	}
	return result
}

func (m *mockStatusManager) AllowedStatuses() []string {
	return []string{"Pending", "Execute", "Active", "Blocked", "Review", "Complete", "Error"}
}

// --- Status Tool Tests ---

func TestToolGetStatus(t *testing.T) {
	sm := newMockStatusManager()
	sm.statuses["doc123"] = "Active"
	h := NewHandler(nil, sm, testLogger())

	resp := call(t, h, "tools/call", map[string]interface{}{
		"name":      "get_status",
		"arguments": map[string]interface{}{"id": "doc123"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(ToolResult)
	if !ok {
		t.Fatal("result is not a ToolResult")
	}

	text := result.Content[0].Text
	if !strings.Contains(text, `"status":"Active"`) {
		t.Errorf("expected status Active in response, got: %s", text)
	}
}

func TestToolGetStatus_DefaultPending(t *testing.T) {
	sm := newMockStatusManager()
	h := NewHandler(nil, sm, testLogger())

	resp := call(t, h, "tools/call", map[string]interface{}{
		"name":      "get_status",
		"arguments": map[string]interface{}{"id": "unknown-item"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result := resp.Result.(ToolResult)
	if !strings.Contains(result.Content[0].Text, `"status":"Pending"`) {
		t.Errorf("expected default Pending status, got: %s", result.Content[0].Text)
	}
}

func TestToolSetStatus(t *testing.T) {
	sm := newMockStatusManager()
	h := NewHandler(nil, sm, testLogger())

	resp := call(t, h, "tools/call", map[string]interface{}{
		"name":      "set_status",
		"arguments": map[string]interface{}{"id": "sheet456", "status": "Review"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result := resp.Result.(ToolResult)
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, `"ok":true`) {
		t.Errorf("expected ok:true in response, got: %s", result.Content[0].Text)
	}

	if sm.statuses["sheet456"] != "Review" {
		t.Errorf("expected status Review, got: %s", sm.statuses["sheet456"])
	}
}

func TestToolSetStatus_InvalidStatus(t *testing.T) {
	sm := newMockStatusManager()
	h := NewHandler(nil, sm, testLogger())

	resp := call(t, h, "tools/call", map[string]interface{}{
		"name":      "set_status",
		"arguments": map[string]interface{}{"id": "doc123", "status": "InvalidStatus"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result := resp.Result.(ToolResult)
	if !result.IsError {
		t.Error("expected tool to return an error for invalid status")
	}
}

func TestToolSetStatus_MissingParams(t *testing.T) {
	sm := newMockStatusManager()
	h := NewHandler(nil, sm, testLogger())

	// Missing id
	resp := call(t, h, "tools/call", map[string]interface{}{
		"name":      "set_status",
		"arguments": map[string]interface{}{"status": "Active"},
	})
	result := resp.Result.(ToolResult)
	if !result.IsError {
		t.Error("expected error when id is missing")
	}

	// Missing status
	resp = call(t, h, "tools/call", map[string]interface{}{
		"name":      "set_status",
		"arguments": map[string]interface{}{"id": "doc123"},
	})
	result = resp.Result.(ToolResult)
	if !result.IsError {
		t.Error("expected error when status is missing")
	}
}

func TestToolListStatuses(t *testing.T) {
	sm := newMockStatusManager()
	sm.statuses["notes/abc"] = "Execute"
	sm.statuses["doc123"] = "Complete"
	h := NewHandler(nil, sm, testLogger())

	resp := call(t, h, "tools/call", map[string]interface{}{
		"name": "list_statuses",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	result := resp.Result.(ToolResult)
	text := result.Content[0].Text
	if !strings.Contains(text, `"total":2`) {
		t.Errorf("expected total 2 in response, got: %s", text)
	}
	if !strings.Contains(text, "allowedStatuses") {
		t.Errorf("expected allowedStatuses in response, got: %s", text)
	}
}

func TestToolStatusWithNilManager(t *testing.T) {
	h := NewHandler(nil, nil, testLogger())

	resp := call(t, h, "tools/call", map[string]interface{}{
		"name":      "get_status",
		"arguments": map[string]interface{}{"id": "doc123"},
	})

	result := resp.Result.(ToolResult)
	if !result.IsError {
		t.Error("expected error when status manager is nil")
	}
}
