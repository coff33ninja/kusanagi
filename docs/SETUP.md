# Kusanagi Setup

## Requirements

- **Windows 11** (WASAPI audio via miniaudio/malgo, Win32 UI Automation)
- **Go 1.26+** for building from source
- **Gemini API key(s)**: set in `config.json`
- **Zig** (`zig cc`) for CGo compilation — install via `winget install zig` or `scoop install zig`
- **Storage**: ~50MB for binary + MCP server

## Quick Start (fresh machine)

```powershell
# 1. Clone
git clone https://github.com/coff33ninja/kusanagi
cd kusanagi

# 2. Copy config and add API keys
copy config.example.json config.json

# 3. Download MCP server binary
.\scripts\download-servers.ps1

# 4. Build (requires zig for CGo)
$env:CC = "zig cc"
$env:CGO_ENABLED = "1"
go build -o kusanagi.exe .\cmd\kusanagi\

# 5. Run
.\kusanagi.exe -config config.json
```

Or use the launcher:
```powershell
.\scripts\go-run.ps1
```

## Configuration

Copy `config.example.json` to `config.json` and fill in your Gemini API keys:

| Key | Default | Description |
|---|---|---|
| `gemini_keys` | — | Array of Gemini API keys (usage-weighted selection) |
| `gemini_model` | `gemini-3.1-flash-lite` | Fallback model reference |
| `live_model` | `gemini-3.1-flash-live-preview` | Live WebSocket model (STT+LLM+TTS in one) |
| `temperature` | `0.7` | Response randomness |
| `voice_model` | `gemini-3.1-flash-tts-preview` | Config reference (not used in Live mode) |
| `voice_name` | `Aoede` | TTS voice preset |
| `rag.top_k` | `5` | Number of MCP memory results to retrieve (FTS5 search) |
| `mcp.server_scripts` | — | Array of MCP stdio server commands |

## Voice Mode

Kusanagi uses the Gemini Live API (single WebSocket for STT+LLM+TTS) by default:
- Mic audio captured via malgo (miniaudio WASAPI) → streamed over Live WebSocket → TTS played back via malgo
- One stream handles everything — no separate STT/LLM/TTS calls

## VRAM Budget

All inference is offloaded to Gemini API. The Go binary itself uses negligible RAM (~15MB binary, ~50MB runtime).

## Adding MCP Tools

To add an MCP stdio tool server, add to `config.json`:

```json
"mcp": {
  "server_scripts": [
    { "command": "node", "args": ["path/to/mcp-server.js"] }
  ]
}
```

## Project Structure

```
kusanagi/
├── config.example.json            # Config template (safe for repo)
├── scripts/
│   ├── download-servers.ps1       # Download go-mcp-computer-use binary
│   ├── go-run.ps1                 # Go launcher with pre-flight validation
│   └── backup.ps1                 # Create timestamped project backup
├── cmd/
│   └── kusanagi/
│       └── main.go                # Entry point, flag parsing, MCP + Gemini init
├── internal/
│   ├── agent/
│   │   └── agent.go               # Agent: validation, loop detection, audit trail,
│   │                              #   behavioral rules, startup probes, system prompt
│   ├── config/
│   │   ├── config.go              # Config loader with model/voice reference
│   │   └── config_test.go         # Config tests
│   ├── gemini/
│   │   ├── live.go                # Gemini Live WebSocket client (BidiGenerateContent)
│   │   └── keyring.go             # Usage-weighted key rotation with per-key stats
│   ├── mcp/
│   │   └── client.go              # Raw JSON-RPC 2.0 MCP client over stdio
│       └── audio/
│       ├── winmm.go               # (package declaration only, zig cc CGo linkage)
│       ├── stream.go              # malgo continuous microphone streaming
│       └── playback.go            # malgo audio playback
└── docs/
    ├── SPEC.md                    # Architecture spec
    └── SETUP.md                   # This file
```

## Building from Source

```powershell
$env:CC = "zig cc"
$env:CGO_ENABLED = "1"
go build -o kusanagi.exe .\cmd\kusanagi\
```

Or use the build script:
```powershell
.\scripts\build.ps1
```

The binary is self-contained with miniaudio statically linked. Copy it and `config.json` anywhere — no runtime needed.
