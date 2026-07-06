# Kusanagi — AI Agent Spec

## Goal
A voice-driven AI agent with computer-use abilities on Windows 11, powered by Google Gemini 2.5 Flash via the Live API (single WebSocket for STT+LLM+TTS). Uses go-mcp-computer-use MCP server (133 tools) for screen OCR, mouse, keyboard, files, browser automation, UI Automation, ONNX detection, and more. Single ~13MB Go binary with CGo-bound miniaudio via malgo.

## Architecture
```
User mic → Gemini Live API (single WebSocket: STT + LLM + TTS) → Speaker
              ↕
        MCP tools (go-mcp-computer-use, 133 tools)
              ↕
 SQLite+FTS5 memory | ONNX watcher | Datalog
```

## Pipeline
- **Single Live WebSocket**: `gemini-3.1-flash-live-preview` via `BidiGenerateContent` WSS — bidirectional audio streaming with native tool calling. No separate STT/TTS calls — everything in one stream at 1x token cost vs 3x.
- **Audio capture**: malgo (miniaudio CGo bindings) via WASAPI, Data callback pushes PCM frames to channel
- **Audio playback**: malgo with persistent device, Data callback reads from shared ring buffer
- **MCP**: go-mcp-computer-use v0.2.36 (stdio), 133 tools, all exposed to Gemini (no exclusions)
- **Memory**: SQLite+FTS5 via MCP `memory_set/get/search` — no separate vector DB
- **Audio**: malgo — Go bindings for miniaudio C library (WASAPI on Windows). Built via `zig cc` for fast CGo compilation.

## Startup Sequence
1. Load config, validate fields
2. Connect to MCP server, initialize (JSON-RPC 2.0)
3. List tools (all exposed to Gemini, no exclusions)
4. Run 7 validation checks: config, MCP ping, memory round-trip, system probe (hostname, OS, RAM, displays, disk, battery, uptime, volume, open windows), display info, screenshot, audio devices
5. Start ONNX watcher (background YOLO UI detection, 5s interval)
6. Load 5 behavioral rules into MCP memory (searchable by AI via `memory_search`)
7. Print validation table with PASS/FAIL/WARN counts
8. Speak startup report over TTS, enter Live chat loop

## Key Design Decisions
- **Live-only architecture**: Single Gemini Live WebSocket handles STT → LLM → TTS → tool calling in one stream. No separate SSE calls, no 3x token overhead from the old pipeline.
- **Minimal dependencies**: Stdlib except malgo (miniaudio CGo, statically linked via zig cc). MCP client implements JSON-RPC 2.0 from scratch in ~316 lines.
- **malgo audio**: miniaudio C library via CGo bindings — handles capture and playback through WASAPI with automatic format conversion.
- **System prompt as platform docs**: The prompt tells the LLM about MCP tool semantics, chain orchestration, OCR regions, UIA priority, datalog queries, template matching, ONNX watcher, keylogger, training pipeline, and adaptive engine.
- **Startup validation**: Explicit 7-check health report in conversation history — the LLM always knows its own health.
- **Auto-audit trail**: Every tool call and result logged to MCP memory automatically — no LLM effort required.
- **Pattern-based loop detection**: Two algorithms (frequency threshold + alternating pair detection) prevent infinite tool cycles.
- **Tool retry**: Failed tool calls retry up to 3 times with exponential backoff (500ms → 1s) before reporting error.
- **Server-side chain orchestration**: Multi-step sequences execute in a single MCP call via `chain` — tool calls, waits, OCR polling, branching, loops, {{variable}} capture.
- **Tool result semantics**: LLM taught to distinguish query tools (return JSON) from action tools (return `"ok"`). Prevents hallucinating data from `"ok"`.
- **MCP-first memory**: Uses MCP server's SQLite+FTS5 for all persistent storage — no separate vector database.
- **Key rotation**: Usage-weighted key selection across all configured API keys with error/rate-limit penalties.

## Acceptance Criteria
- [x] `.\kusanagi.exe -config config.json` starts live interaction
- [x] malgo captures mic audio via WASAPI (24kHz 16-bit PCM)
- [x] Gemini Live WebSocket streams audio ↔ bidirectional tool calling
- [x] Gemini TTS speaks responses (Aoede voice, 24kHz PCM, streaming via Live)
- [x] Tool call round-trip: functionCall → MCP execute → functionResponse → follow-up
- [x] Tool retry: 3 attempts with exponential backoff (500ms, 1s)
- [x] Startup validation: 7 checks with PASS/FAIL/WARN table
- [x] ONNX watcher auto-started at startup (background YOLO detection, 5s interval)
- [x] Audit trail: every tool call logged via `memory_set` with `scope="audit"`
- [x] Behavioral rules pre-loaded (5 rules, searchable by AI)
- [x] Pattern-based loop detection (frequency: 4+ in 30s, alternating pairs)
- [x] Pre-tool announcement: AI says plan before calling, reports result after (prompt instruction)
- [x] Tool usage guard: tools only called for computer actions, chat gets verbal response (prompt instruction)
- [x] Memories persist across sessions via MCP SQLite+FTS5
- [x] Chain tool available for server-side multi-step orchestration
- [x] All 133 MCP tools exposed to Gemini as function declarations
- [x] Usage-weighted key rotation with error/rate-limit penalties
