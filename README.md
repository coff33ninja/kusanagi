# Kusanagi

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Gemini](https://img.shields.io/badge/Gemini-2.5_Flash-8E75B2?logo=googlebard)](https://deepmind.google/technologies/gemini/flash/)
[![Windows](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows11)](https://www.microsoft.com/windows)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

</div>

Voice-driven AI agent with computer-use abilities on Windows 11. Powered entirely by Google Gemini 2.5 Flash via the **Live API** — a single WebSocket handles STT, LLM, and TTS in one stream (1x token cost vs 3x). Single ~13MB binary with miniaudio audio via CGo.

Built on [go-mcp-computer-use](https://github.com/coff33ninja/go-mcp-computer-use) for screen OCR, mouse, keyboard, filesystem, browser, and clipboard control across 133 MCP tools.

## Architecture

```
┌─ Startup ──────────────────────────────────────────────┐
│  System Validation (7 checks: config, MCP, memory,     │
│    display, screenshot, audio, system probe)           │
│  ONNX Watcher started (background UI detection, 5s)    │
│  Audit trail auto-initialized (per-tool-call logging)  │
│  Behavioral rules loaded (5 rules)                     │
└────────────────────────────────────────────────────────┘
         ↓
┌─ Live Loop (single Gemini WSS — STT+LLM+TTS) ────────────┐
│  Mic → malgo capture → Live WebSocket → malgo playback   │
│                               ↕                          │
│              MCP server (133 tools via stdio)             │
│        retry 3x + loop detection + audit trail           │
│                               ↕                          │
│   SQLite+FTS5 memory  |  ONNX watcher  |  Datalog        │
└──────────────────────────────────────────────────────────┘
```

## Features

### Startup Validation
On launch, runs 7 automated checks: config validity, MCP server ping, memory store round-trip (SQLite+FTS5), system probe (hostname, OS, RAM, displays, disk, battery, uptime, volume, open windows), screen capture, screenshot, and audio devices. Results printed as a formatted table with PASS/FAIL/WARN counts and stored in LLM conversation history.

### Server-Side Chain Orchestration
Multi-step sequences execute server-side in a single MCP call via the `chain` tool — no round-trip latency between steps. Supports tool calls, waits, OCR polling (wait for text), if/else branching, loops, and before/after OCR verification. Use `{{variable}}` to pass captured output between steps.

### Background ONNX Watcher
YOLO UI element detection runs passively in the background (5s interval). Detected elements cached — the AI reads them via `onnx_watch_cache` without taking a new screenshot. Class labels, confidence scores, and bounding boxes available instantly.

### Keylogger — Record & Replay
Record keyboard and mouse input via `keylogger_start`, stop with `keylogger_stop` which returns the sequence as chain-compatible steps. Learn multi-step workflows from user demonstrations and replay them.

### Adaptive Engine
Learns OCR→command associations over time. `agent_analyze` returns per-tool timing stats and success rates. `agent_suggest(ocr_text)` predicts the best next command with confidence scores. `agent_train` rebuilds the prediction model from recent datalog entries.

### Task Introspection
Wrap tasks with `task_begin`/`task_end`. After `task_end`, call `introspection_analyze` to get mined insights: slowest tools, most-failed tools, repeated patterns, and improvement suggestions.

### Automatic Audit Trail
Every tool call, result, error, and user input is automatically logged via MCP `memory_set` with `scope="audit"`. The AI searches this trail across sessions via `memory_search` to learn from past patterns.

### Behavioral Rules Pre-Loaded
6 behavioral rules (anti-hallucination, reuse results, fix STT mistakes, error recovery, tool best practices, task introspection) stored in MCP memory at startup. Rules survive restarts and are searchable by the AI.

### Tool Loop Detection
Tracks the last 50 tool calls in a ring buffer. Detects loops two ways: same tool called 4+ times within 30 seconds, or alternating tool pairs (A→B→A→B) pattern. Warnings logged to terminal.

### MCP Tool Result Semantics
LLM is explicitly told the difference between query tools (return structured JSON) and action tools (return `"ok"` — API didn't crash, not that anything happened). Prevents hallucinating data from `"ok"` responses.

### Live WebSocket Streaming (Single Connection)
- **All-in-one**: Gemini Live API handles STT → LLM → TTS → tool calling in a single WebSocket stream. No separate SSE calls — 1x token cost vs the old 3x pipeline.
- **Audio**: PCM 24kHz audio chunks played incrementally through malgo (miniaudio WASAPI) as they arrive from the Live stream.
- **Pre-tool announcements**: AI says what it's about to do before calling tools, reports results after (prompt-level instruction).

## Quick Start

```powershell
# 1. Clone
git clone https://github.com/coff33ninja/kusanagi
cd kusanagi

# 2. Copy config template and fill in API keys
copy config.example.json config.json

# 3. Download MCP server
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

Pre-built binaries are available on the [Releases page](https://github.com/coff33ninja/kusanagi/releases). Download `kusanagi.exe` and run from any directory — no Go toolchain needed.

> **Future installer**: Kusanagi lives in `%ProgramFiles%\Kusanagi\`. The installer creates `%AppData%\Kusanagi\config.json` with a template and opens it for you to fill in your API keys — no need to browse to AppData yourself. If the config is deleted, the exe regenerates it on next launch and prompts you from the terminal. The release binary currently downloads to a temp directory and is cleaned up — for now, grab it manually from the release page.

## Configuration

`config.json` at the project root holds everything:

| Key | Default | Description |
|---|---|---|
| `gemini_keys` | — | Array of API keys (usage-weighted selection, skip on 429/503) |
| `gemini_model` | `gemini-3.1-flash-lite` | Fallback model reference |
| `live_model` | `gemini-3.1-flash-live-preview` | Live WebSocket model (STT+LLM+TTS in one) |
| `temperature` | `0.7` | Response randomness |
| `voice_model` | `gemini-3.1-flash-tts-preview` | Config reference (not used in Live mode) |
| `voice_name` | `Aoede` | TTS voice |
| `rag.top_k` | `5` | Memory results to retrieve |
| `mcp.server_scripts` | — | MCP stdio server commands |

## Logging

Kusanagi writes structured JSON logs to `kusanagi.log` in the working directory (co-located with the binary). The log file is auto-created on startup with level `DEBUG` — all entries are written for downstream filtering or ingestion.

Log entries include:
- `time` — ISO 8601 timestamp
- `level` — `DEBUG`, `INFO`, `WARN`, or `ERROR`
- `msg` — human-readable message
- `error` — error detail (when applicable)
- Additional key-value pairs per entry (e.g. `tool`, `version`, `attempt`, `tool_count`)

Example:
```json
{"time":"2026-07-07T12:34:56.789Z","level":"INFO","msg":"Kusanagi starting","version":"0.1.1"}
{"time":"2026-07-07T12:34:57.012Z","level":"INFO","msg":"starting MCP server","cmd":"E:\\...\\mcp-server.exe"}
{"time":"2026-07-07T12:34:58.123Z","level":"ERROR","msg":"MCP request timed out","method":"tools/list","id":1}
```

Errors that were previously silently discarded (`json.Marshal` failures, type assertion misses, `Getwd` errors, MCP non-JSON stderr lines) are now captured at `WARN`/`ERROR` level.

## Voice Controls

Say any of these to exit: **shutdown**, **goodbye**, **quit**, **stop**, **exit**

Kusanagi validates its subsystems then speaks a startup report before listening. Tools are only called when the user asks for a computer action — greetings and chat get verbal responses only.

Terminal shows every tool call with status:
```
  ✓ screenshot({}) → ok
  ✓ ocr({region: ...}) → ok
  ✓ find_text_and_click(start) → ok
```

Failed tools retry up to 3 times with exponential backoff. Tool calls and results are logged to the audit trail for cross-session learning.

## MCP Server Tools (133 Total — All Passed to Gemini)

Kusanagi connects to go-mcp-computer-use, which provides tools for:

- **Screen**: screenshot, OCR (full or region), resolution, DPI, pixel color
- **Mouse**: click, right-click, double-click, move, drag, scroll, get position, find text and click
- **Keyboard**: type text, key combos, key hold/release, select all and type
- **Window**: list, focus, move, resize, minimize, maximize, close, wait for window
- **Clipboard**: get/set text
- **Filesystem**: read, write, edit, delete, copy, move, find, list directory, file info
- **Process**: list processes, kill, launch app, launch and wait
- **Browser**: open URL, new tab, search, focus URL bar
- **Audio**: list devices, volume, mute, set default device
- **System**: info, uptime, battery, disk, displays, notifications, lock, shutdown, restart, sleep, hibernate
- **Network**: ping, hostname, IP, DNS
- **UI Automation**: UIA find, get text, invoke (reliable element access without OCR)
- **Template Matching**: store, find, list, forget visual element templates
- **ONNX**: YOLO detection, watcher, download models
- **Memory**: set, get, search (FTS5), list, forget — SQLite-backed
- **Chaining**: chain (multi-step orchestration with polling, branching, loops)
- **Training**: save samples, stats, list, clean noise, mark used, export YOLO dataset
- **Datalog**: query command/chain/OCR history, export training pairs
- **Introspection**: task begin/end, analyze
- **Adaptive**: agent analyze, suggest, train
- **Keylogger**: start/stop recording, status
- **Power**: brightness, display modes, idle time

All 133 tools are passed to Gemini as function declarations (no exclusions).

## Project Structure

```
kusanagi/
├── VERSION                        # Tracked version number (consumed by build.ps1)
├── config.example.json            # Config template (tracked)
├── config.json                    # Local config (gitignored)
├── scripts/
│   ├── build.ps1                  # Build with VERSION → ldflags injection
│   ├── download-servers.ps1       # Download MCP server binary
│   ├── go-run.ps1                 # Launcher with pre-flight validation + auto-build
│   └── backup.ps1                 # Project backup script
├── kusanagi-go.ps1                # Double-click launcher (pwsh)
├── servers/                       # MCP server binaries (gitignored)
├── cmd/
│   └── kusanagi/
│       └── main.go                # Entry point, flag parsing, MCP + Gemini init
├── internal/
│   ├── agent/
│   │   └── agent.go               # Agent: validation, loop detection, audit trail,
│   │                              #   behavioral rules, startup probes, system prompt
│   ├── config/
│   │   ├── config.go              # JSON config loader with model/voice reference
│   │   └── config_test.go         # Config tests
│   ├── gemini/
│   │   ├── live.go                # Gemini Live WebSocket client (BidiGenerateContent)
│   │   └── keyring.go             # Usage-weighted key rotation with per-key stats
│   ├── mcp/
│   │   └── client.go              # Raw JSON-RPC 2.0 MCP client over stdio
│   └── audio/
│       ├── winmm.go               # (package declaration only, zig cc CGo linkage)
│       ├── stream.go              # malgo continuous microphone streaming
│       └── playback.go            # malgo audio playback
├── docs/
│   ├── meta/
│   │   └── CHANGELOG.md           # Keep a Changelog format
│   ├── SPEC.md                    # Architecture specification
│   └── SETUP.md                   # Setup guide
├── go.mod / go.sum                # Go module definition
└── README.md                      # This file
```

## Key Design Decisions

- **Live-only architecture**: Single Gemini Live WebSocket handles STT+LLM+TTS+tool calling in one stream. 1x token cost vs 3x from separate STT/LLM/TTS calls.
- **Minimal dependencies**: Stdlib + malgo (miniaudio CGo, statically linked via zig cc). MCP client implements JSON-RPC 2.0 from scratch in ~316 lines.
- **malgo audio**: Miniaudio C library via CGo bindings — handles capture and playback through WASAPI with automatic format conversion.
- **System prompt as platform docs**: The prompt is the primary documentation for the LLM about the MCP server's 133 tools — tool semantics, chain orchestration, OCR regions, UIA priority, datalog queries, template matching, passive watcher, keylogger, training pipeline, and adaptive engine.
- **Startup validation**: Explicit health check of every subsystem (MCP ping, memory R/W, screenshot, system probe, audio devices) with PASS/FAIL/WARN report in conversation history.
- **Auto-audit trail**: Every tool call and result logged to MCP memory via background goroutine — no LLM effort required, searchable across sessions.
- **Pattern-based loop detection**: Two detection algorithms (frequency: 4+ calls in 30s, alternating pairs: A→B→A→B) prevent infinite tool cycles.
- **Tool retry with exponential backoff**: Failed tool calls retry up to 3 times (500ms, 1s) before reporting error to Gemini.

## Dependencies

**Zero.** Go 1.26+ standard library only. Audio uses `gen2brain/malgo` (miniaudio CGo bindings, statically linked via zig cc).

## Roadmap

Milestone | Status | Description
---|---|---
**API Key Usage Tracking** | 🔴 **MUST** | Track per-key usage metrics (requests, tokens, errors, rate-limit hits). Surface via dashboard or `memory_set` for cost attribution and quota management. Implement key rotation with usage-weighted selection instead of round-robin.
**System Tray + Auto-Start** | ❌ | Register to auto-start with Windows. System tray icon showing status (listening/processing/idle). Configurable via script.
**Wake Word Activation** | ❌ | Keyword spotting (e.g. "Hey Kusanagi") to activate listening. Prevents processing background noise.
**Conversation Cooldown** | ❌ | Configurable silence period after AI speaks (default 2-3s) before accepting new voice input. Prevents self-triggering.
**Audio Ducking** | ❌ | Lower system volume during TTS playback. Restore after speaking. Uses `set_volume`/`get_volume` from MCP tools.
**Noise Suppression** | ❌ | Background noise filtering on mic input before STT. Reduces false transcriptions from fan noise, keyboard clatter.
**Session History Persistence** | ❌ | Persist conversation history across restarts via SQLite. Currently only MCP memories survive sessions.
**Tool Confirmation Mode** | ❌ | Before destructive actions (write_file, delete_file, kill_process, shutdown), prompt for voice confirmation. Toggleable via config.
**Context Window Management** | ❌ | Auto-summarize or trim history near Gemini's context limit. Use `memory_search` to save/restore compressed summaries.
**structuredContent Support** | ❌ | Parse MCP `structuredContent` extension for rich tool results (screenshots in tool output).
**OCR "ok" Fallback** | ❌ | When OCR returns only `"ok"`, retry via `datalog_query` to retrieve actual OCR text from the server's history.
**Multi-Profile Support** | ❌ | Multiple activation phrases mapped to different profiles with restricted tool access per profile.
**Push-to-Talk Fallback** | ❌ | Hold a hotkey (e.g. Scroll Lock) to talk instead of always-listening. Useful in noisy environments.
**Time-Aware Personality** | ❌ | Startup probe includes current time → greeting adapts (good morning/afternoon/evening). Optional weather lookup.
**Configurable Agent Name** | ❌ | Agent name detected from hostname or configured in config.json without editing source.

## Built With

- **go-mcp-computer-use** ([coff33ninja/go-mcp-computer-use](https://github.com/coff33ninja/go-mcp-computer-use)) — Windows computer-use MCP server (133 tools). All tools passed to Gemini with no exclusions, including `chain` for multi-step orchestration. Build patterns (VERSION file, ldflags injection, release workflow) adapted from this project.
- **ai-skills** ([coff33ninja/ai-skills](https://github.com/coff33ninja/ai-skills)) — Cross-tool AI agent skill definitions used to guide development. Skills like `os-awareness`, `anti-global-install`, `follow-existing-patterns`, `anti-phantom-symbols`, and `anti-library-hallucination` shaped every phase of this project.

## License

MIT
