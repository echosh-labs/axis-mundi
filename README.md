# axis-mundi

A high-performance command-center and connectivity bridge between Google Gemini voice interfaces and Google Workspace orchestration. Axis Mundi serves as the ingestion and triage layer for voice-generated data.

## Core Connectivity

This project is dual-licensed. Use of this software is governed by the AGPL-3.0 unless a commercial license has been acquired from echoSH labs.

* **Gemini Voice Bridge**: Acts as a functional uplink between personal mobile devices (via Google Gemini) and professional Workspace accounts.

* **Voice-to-Keep Ingestion**: Captures spoken directives through personal Gemini interfaces, automatically generating Google Keep notes for system ingestion.

* **Orchestration Interface**: Provides a high-speed TUI for the management, inspection, and execution of voice-driven tasks.

## Features

* **Hybrid TUI**: Keyboard-centric React terminal for rapid object management.

* **Real-Time Uplink**: Server-Sent Events (SSE) for zero-latency registry updates.

* **State Persistence**: Server-side tracking of task lifecycles and operational modes.

* **Service Account Impersonation**: Secure delegation using domain-wide credentials.

* **MCP Server**: Built-in Model Context Protocol endpoint that lets AI agents read and search your entire Google Workspace directly.

## Interaction Schema

### Task Lifecycle Status

* **Pending**: Initial state of ingested voice notes awaiting triage.

* **Execute**: Directive approved for automation or manual processing.

* **Complete**: Task finalized and archived within the Workspace environment.

### Controls

* `[PageUp/PageDown]`: Cycle status (Pending → Execute → Complete).

* `[A]`: Enable AUTO Mode (Background Monitoring).

* `[M]`: Enable MANUAL Mode (Interactive Control).

* `[Arrows]`: Navigate registry list.

* `[Enter/Space]`: Inspect note payload.

* `[Delete]`: Purge note from registry.

## Setup

### Prerequisites

* Go 1.24+

* Node.js 18+

* GCP Service Account with Domain-Wide Delegation for Keep, Admin Directory, Docs, Sheets, Drive, and Calendar.

## Environment

Configure `.env` in the root directory with the appropriate administrative and service account credentials:

## MCP Server

Axis Mundi includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server that gives AI coding agents and assistants direct access to your Google Workspace. Any MCP-compatible client — VS Code with Copilot, Claude Desktop, or your own tools — can connect and work with your data without any extra setup.

### What Your Agent Can Do

Once connected, an agent can:

* **Browse everything** — List all your Keep notes, Google Docs, Sheets, Gmail threads, and Calendar events in one unified view using the `list_workspace` tool.
* **Read Keep notes** — Pull the full content of any note, including checklists with checked/unchecked state.
* **Search notes** — Find Keep notes by keyword across titles and content.
* **Read Google Docs** — Retrieve the plain text body of any document.
* **Read Google Sheets** — Pull cell data from any spreadsheet, with support for custom ranges.
* **Read Gmail threads** — Get complete email conversations including all messages, headers, and attachments.
* **Read Calendar events** — Fetch full event details including descriptions and timestamps from Google Calendar.

### Connecting

The MCP endpoint runs automatically at `POST /mcp` when you start the server. No additional processes or configurations needed.

To secure the endpoint, set an API key in your `.env`:

```env
MCP_API_KEY=your-secret-key-here
```

Then point your agent to the server:

**VS Code / Copilot** — add to `.vscode/mcp.json`:

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

**Claude Desktop** — add to `claude_desktop_config.json`:

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

For full protocol details, tool schemas, and curl examples, see [MCP.md](MCP.md).