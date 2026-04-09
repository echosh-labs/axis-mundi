# Axis Mundi MCP Server

Model Context Protocol (MCP) server for exposing Google Keep notes to cloud code agents. Runs as an integrated endpoint within Axis Mundi.

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

Lists all Keep notes as MCP resources. Each note gets a `keep://notes/{id}` URI.

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
        "description": "- [ ] Deploy staging\n- [x] Review PR",
        "mimeType": "text/plain"
      }
    ]
  }
}
```

### resources/read

Retrieves full content of a specific Keep note.

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

Three tools are available via `tools/list` and `tools/call`.

### list_notes

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

### get_note

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

### search_notes

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
Cloud Agent → POST /mcp → MCP Server → Workspace Service → Google Keep API
                  ↑                          ↑
           Bearer auth              Domain-wide delegation
           (MCP_API_KEY)           (service account impersonation)
```

The MCP server reuses the same authenticated `workspace.Service` as the rest of Axis Mundi. No separate credentials or service accounts are needed.
