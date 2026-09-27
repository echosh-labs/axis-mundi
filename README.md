# 🏛️ axis-mundi — Command-Center & Voice-to-Workspace Orchestration Hub
> A cybernetic terminal dashboard, Google Keep voice-directive ingestion bridge, and Model Context Protocol (MCP) server for autonomous agents.

---

## 🌟 The Vibe
`axis-mundi` is the nexus between voice directives and executive action. Designed to run as a microservice on **Port 8088**, it acts as a functional uplink between personal voice notes (captured on mobile devices via Google Gemini / Keep) and automated system orchestration. It features a retro-futuristic terminal UI (TUI), real-time Server-Sent Events (SSE) telemetry, and an MCP server allowing AI agents to search and inspect notes, documents, and calendars.

---

## 🚀 60-Second Quickstart

```bash
# 1. Clone the repository
git clone https://github.com/echosh-labs/axis-mundi.git
cd axis-mundi

# 2. Setup your local environment
cp .env.example .env

# 3. Boot the server & landing portal
go run ./cmd/axis/main.go
# Or preview the landing portal directly:
./serve-landing.sh
```

Once running:
- **Web Command Portal**: Visit [http://localhost:8088](http://localhost:8088)
- **Model Context Protocol (MCP)**: Streamable endpoint available at `http://localhost:8088/mcp`
- **Health Diagnostics**: `GET http://localhost:8088/health`

---

## 🖥️ Interactive Controls (TUI Mode)

When operating in terminal console mode, manage directives with keyboard shortcuts:

| Keybinding | Action | Description |
| :--- | :--- | :--- |
| `[PageUp]` / `[PageDown]` | **Cycle Status** | Transition directives: `Pending` ➔ `Execute` ➔ `Complete`. |
| `[A]` | **AUTO Mode** | Enables continuous autonomous background monitoring and triage. |
| `[M]` | **MANUAL Mode** | Locks system into interactive human operator inspection mode. |
| `[Arrow Keys]` | **Navigate** | Scroll through the directive registry stream. |
| `[Enter]` / `[Space]` | **Inspect** | Expand and inspect full note payload, metadata, and origin tags. |
| `[Delete]` | **Purge** | Archive or remove note from the active working registry. |

---

## 🤖 Model Context Protocol (MCP) Server

Axis Mundi includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server (`Port 8088/mcp`) that equips AI coding agents (Antigravity, Claude Desktop, VS Code) with direct tools to query your workspace.

### Available Agent Tools
- `list_workspace`: Unified discovery across Keep notes, Google Docs, Sheets, and Gmail.
- `read_keep_note`: Fetch full checklist items and markdown content from a specific Keep note.
- `search_notes`: Full-text keyword search across all notes.
- `read_doc`: Read the plain text body of Google Docs.
- `read_sheet`: Pull spreadsheet cell data with custom range filtering.
- `read_gmail`: Inspect email threads and metadata.
- `read_calendar`: Query upcoming schedule events.

### Client Configuration

#### VS Code / Copilot (`.vscode/mcp.json`)
```json
{
  "servers": {
    "axis-mundi": {
      "type": "http",
      "url": "http://localhost:8088/mcp"
    }
  }
}
```

#### Claude Desktop (`claude_desktop_config.json`)
```json
{
  "mcpServers": {
    "axis-mundi": {
      "type": "streamableHttp",
      "url": "http://localhost:8088/mcp"
    }
  }
}
```

---

## ⚙️ Configuration & Google Workspace Setup

Copy `.env.example` to `.env`:
```env
PORT=8088
MCP_API_KEY=your-optional-api-key

# Google Cloud Service Account (Domain-Wide Delegation)
# Required for live Keep/Drive/Docs synchronization:
GOOGLE_APPLICATION_CREDENTIALS="./.service-account.json"
SERVICE_ACCOUNT_EMAIL="service-account@your-project.iam.gserviceaccount.com"
ADMIN_EMAIL="admin@your-domain.com"
USER_EMAIL="user@your-domain.com"
```

> **Testing Offline / Without Google Cloud:**  
> The web landing interface and TUI can be previewed without Google Cloud credentials using `./serve-landing.sh`. For backend unit tests with mock endpoints, run `go test ./...`.

---

## 🏗️ Architecture

```
axis-mundi/
├── cmd/axis/main.go          # Microservice HTTP daemon & MCP server entrypoint
├── internal/
│   ├── server/               # HTTP router, middleware, and CORS configuration
│   ├── workspace/            # Google Keep, Drive, Docs, Sheets, and Gmail handlers
│   ├── mcp/                  # Streamable HTTP MCP JSON-RPC protocol implementation
│   └── database/             # SQLite / memory persistence for directive lifecycle
├── landing/                  # Web command-center dashboard & styling
├── serve-landing.sh          # Quick local web server for frontend inspection
└── .env.example              # Sample configuration template
```

---

## 📄 License
Dual-licensed under the AGPL-3.0 and commercial enterprise licensing from [echoSH labs](https://echosh-labs.com).