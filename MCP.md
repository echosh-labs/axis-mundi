# Axis Mundi MCP Server

Model Context Protocol (MCP) server for exposing Google Workspace items (Keep, Docs, Sheets, Gmail, Calendar) and their workflow statuses to cloud code agents. Runs as an integrated endpoint within Axis Mundi.

## Setup

### 1. Environment Variable

Add to your `.env` file:

```env
# Optional. If set, all MCP requests must include this as a Bearer token.
# If omitted, the MCP endpoint is open (rely on network-level controls).
MCP_API_KEY=your-secret-key-here
```

### 2. Start the Server

The MCP endpoint starts automatically with Axis Mundi. No additional flags or configuration needed.

```bash
go run ./cmd/axis
```

The MCP endpoint is available at:

```
POST http://localhost:8080/mcp
```

## Protocol

The server implements the [MCP specification (2025-03-26)](https://modelcontextprotocol.io/) using the **Streamable HTTP** transport. All communication uses **JSON-RPC 2.0** over `POST /mcp`.

## Authentication

If `MCP_API_KEY` is set, include it as a Bearer token:

```
Authorization: Bearer your-secret-key-here
```

Requests without a valid token receive HTTP 401.

## MCP Methods

### initialize

Handshake that returns server info and capabilities.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "clientInfo": { "name": "my-agent", "version": "1.0.0" }
    }
  }'
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-03-26",
    "serverInfo": { "name": "axis-mundi", "version": "0.1.0" },
    "capabilities": {
      "resources": { "listChanged": false },
      "tools": { "listChanged": false }
    }
  }
}
```

### ping

Health check.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"ping"}'
```

### resources/list

Lists all workspace items as MCP resources. Each item gets a typed URI:

- Keep notes: `keep://notes/{id}`
- Google Docs: `docs://documents/{id}`
- Google Sheets: `sheets://spreadsheets/{id}`
- Gmail threads: `gmail://threads/{id}`
- Calendar events: `calendar://events/{id}`

Resource descriptions include the current workflow status (e.g. `[Active] snippet text`).

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{"jsonrpc":"2.0","id":1,"method":"resources/list"}'
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "resources": [
      {
        "uri": "keep://notes/abc123",
        "name": "Project Tasks",
        "description": "[Pending] - [ ] Deploy staging\n- [x] Review PR",
        "mimeType": "text/plain"
      },
      {
        "uri": "docs://documents/doc456",
        "name": "Architecture Doc",
        "description": "[Active] System design overview...",
        "mimeType": "text/plain"
      }
    ]
  }
}
```

### resources/read

Retrieves full content of a specific resource by URI.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "resources/read",
    "params": { "uri": "keep://notes/abc123" }
  }'
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "contents": [
      {
        "uri": "keep://notes/abc123",
        "mimeType": "text/plain",
        "text": "# Project Tasks\n\nCreated: 2026-04-01 10:30:00\nUpdated: 2026-04-09 14:15:00\n\n- [ ] Deploy staging\n- [x] Review PR"
      }
    ]
  }
}
```

## MCP Tools

Eleven tools are available via `tools/list` and `tools/call`.

### Keep Tools

#### list_notes

Lists all notes with titles and snippets.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "list_notes", "arguments": {} }
  }'
```

#### get_note

Retrieves full content of a note by ID.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "get_note", "arguments": { "noteId": "abc123" } }
  }'
```

The `noteId` accepts either the bare ID (`abc123`) or the full resource name (`notes/abc123`).

#### search_notes

Searches notes by keyword across titles and content.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "search_notes", "arguments": { "query": "deploy" } }
  }'
```

Response (wrapped in tool result content block):

```json
{
  "matches": [
    { "noteId": "abc123", "title": "Project Tasks", "snippet": "- [ ] Deploy staging..." }
  ],
  "total": 1
}
```

### Docs, Sheets, Gmail & Calendar Tools

#### get_doc

Retrieves the plain text content of a Google Doc.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "get_doc", "arguments": { "documentId": "doc456" } }
  }'
```

#### get_sheet

Retrieves cell values from a Google Sheet. Reads A1:Z100 by default; pass `range` to customize.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "get_sheet", "arguments": { "spreadsheetId": "sheet789", "range": "A1:D20" } }
  }'
```

#### get_gmail_thread

Retrieves the full conversation content of a Gmail thread.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "get_gmail_thread", "arguments": { "threadId": "thread123" } }
  }'
```

#### get_calendar_event

Retrieves the details and description of a Google Calendar event.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "get_calendar_event", "arguments": { "eventId": "event123" } }
  }'
```

### Registry Tool

#### list_workspace

Lists all items across Keep, Docs, Sheets, Gmail, and Calendar with their current workflow status.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "list_workspace", "arguments": {} }
  }'
```

Response:

```json
{
  "items": [
    { "id": "notes/abc123", "type": "keep", "title": "Project Tasks", "snippet": "...", "status": "Active", "uri": "keep://notes/abc123" },
    { "id": "doc456", "type": "doc", "title": "Architecture Doc", "snippet": "...", "status": "Pending", "uri": "docs://documents/doc456" }
  ],
  "total": 2
}
```

### Status Tools

Workflow statuses track the lifecycle of workspace items. Valid status values:

| Status | Description |
|--------|-------------|
| Pending | Default state, not yet acted upon |
| Execute | Queued for execution |
| Active | Currently being worked on |
| Blocked | Waiting on external dependency |
| Review | Ready for review |
| Complete | Done |
| Error | Failed or needs attention |

#### get_status

Get the current workflow status of an item.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "get_status", "arguments": { "id": "notes/abc123" } }
  }'
```

Response:

```json
{ "id": "notes/abc123", "status": "Active" }
```

#### set_status

Set the workflow status of an item. Broadcasts the change via SSE to all connected TUI clients.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "set_status", "arguments": { "id": "doc456", "status": "Review" } }
  }'
```

Response:

```json
{ "id": "doc456", "status": "Review", "ok": true }
```

#### list_statuses

List the current status of all tracked items and the allowed status values.

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-secret-key-here" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": { "name": "list_statuses", "arguments": {} }
  }'
```

Response:

```json
{
  "statuses": { "notes/abc123": "Active", "doc456": "Pending" },
  "total": 2,
  "allowedStatuses": ["Pending", "Execute", "Active", "Blocked", "Review", "Complete", "Error"]
}
```

## Agent Configuration

### VS Code / Copilot

Add to your MCP settings (`.vscode/mcp.json` or user settings):

```json
{
  "servers": {
    "axis-mundi": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-secret-key-here"
      }
    }
  }
}
```

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "axis-mundi": {
      "type": "streamableHttp",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-secret-key-here"
      }
    }
  }
}
```

### Generic / Programmatic

Any MCP-compatible client can connect via:

- **Transport**: Streamable HTTP
- **URL**: `http://localhost:8080/mcp`
- **Method**: `POST` with `Content-Type: application/json`
- **Auth**: `Authorization: Bearer <MCP_API_KEY>` (if configured)

## Error Codes

| Code | Meaning |
|------|---------|
| -32700 | Parse error (malformed JSON) |
| -32600 | Invalid request (bad JSON-RPC version) |
| -32601 | Method not found |
| -32602 | Invalid parameters |
| -32603 | Internal error (Keep API failure) |

## Architecture

```
Cloud Agent → POST /mcp → MCP Server → Workspace Service → Google APIs (Keep, Docs, Sheets, Gmail, Calendar)
                  ↑            ↓                ↑
           Bearer auth   StatusManager   Domain-wide delegation
           (MCP_API_KEY)  (get/set/list)  (service account impersonation)
                               ↓
                           SQLite + SSE broadcast → TUI clients
```

The MCP server reuses the same authenticated `workspace.Service` as the rest of Axis Mundi. Status changes made by agents are persisted to SQLite and broadcast via SSE to all connected TUI clients in real time.
