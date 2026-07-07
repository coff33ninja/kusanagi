# Changelog

## [0.1.1] — 2026-07-07

### Added
- Structured logging via `log/slog` with JSON output
- Log file `kusanagi.log` (co-located with the binary, auto-created on startup)
- Discarded error paths now logged: `json.Marshal` failures, type assertion misses, `os.Getwd` errors, MCP timeout and non-JSON stderr lines, malgo context messages

### Changed
- All `log.Print`/`log.Printf`/`log.Println` calls replaced with `slog.Info`/`slog.Warn`/`slog.Debug`/`slog.Error`
- Log level: `DEBUG` (all entries written, filterable downstream)

## [0.1.0] — 2026-07-07

### Overview
Voice-driven AI agent with computer-use abilities on Windows 11. Powered by Google Gemini Live API — a single WebSocket handles STT, LLM, and TTS in one stream. Uses go-mcp-computer-use MCP server for screen OCR, mouse, keyboard, filesystem, browser, and clipboard control across 132 MCP tools.

### Core Architecture
- **Live-only pipeline**: Single Gemini Live WebSocket (`gemini-3.1-flash-live-preview`) via `BidiGenerateContent` — bidirectional audio streaming with native tool calling. 1x token cost vs 3x from separate STT/LLM/TTS.
- **malgo audio**: miniaudio C library via CGo bindings (zig cc). Capture and playback through Windows WASAPI with automatic format conversion (32-bit float mic → 16-bit PCM, stereo → mono, 24kHz).
- **MCP client**: Raw JSON-RPC 2.0 MCP client over stdio (~316 lines, no external dependencies). Connects to go-mcp-computer-use v0.2.36 with 132 tools.
- **Memory**: SQLite+FTS5 via MCP `memory_set/get/search/list/forget` — no separate vector database.

### Audio
- Capture via malgo `InitDevice(Capture)` with Data callback pushing PCM frames to channel
- Playback via malgo persistent device with ring buffer — `Play()` appends, Data callback consumes
- VAD configured via `realtimeInputConfig.automaticActivityDetection` (start sensitivity LOW, end sensitivity LOW, 500ms silence duration)

### Agent Features
- **Startup validation**: 7 automated checks (config, MCP ping, memory round-trip, system probe, display info, screenshot, audio) with PASS/FAIL/WARN table
- **Pattern-based loop detection**: Ring buffer of last 50 tool calls; frequency threshold (4+ in 30s) and alternating pair (A→B→A→B) detection
- **Tool retry**: Failed calls retry up to 3 times with exponential backoff (500ms → 1s)
- **Auto-audit trail**: Every tool call logged to MCP `memory_set` with `scope="audit"`
- **Behavioral rules**: 6 rules pre-loaded into MCP memory at startup
- **Key rotation**: Usage-weighted key selection across all configured Gemini API keys with error/rate-limit penalties

### MCP Integration
- All 132 go-mcp-computer-use tools exposed to Gemini (no exclusions)
- Server-side chain orchestration via `chain` tool (multi-step with polling, branching, loops, variable capture)
- ONNX watcher auto-started at startup (background YOLO UI detection, 5s interval)
- UI Automation integration (reliable element access without OCR)
- Template matching: store, find, list, forget visual element templates
- Training pipeline: save samples, stats, export YOLO dataset
- Datalog: query command/chain/OCR history, export training pairs
- Adaptive engine: analyze, suggest, train OCR→command sequences
- Keylogger: start/stop recording, replay as chain steps
- Introspection: task begin/end with mined insights

### Tools
- `scripts/build.ps1` — build with VERSION → ldflags injection
- `scripts/go-run.ps1` — launcher with pre-flight validation + auto-build
- `scripts/download-servers.ps1` — download MCP server binary
- `scripts/backup.ps1` — timestamped project backup
- `kusanagi-go.ps1` — double-click launcher (pwsh)
- `kusanagi.exe` — ~13MB single binary, miniaudio statically linked

### Built With
- go-mcp-computer-use v0.2.36 — Windows computer-use MCP server (132 tools)
- gen2brain/malgo v0.11.25 — miniaudio CGo bindings
- coder/websocket v1.8.15 — WebSocket client
- ai-skills — cross-tool AI agent skill definitions
