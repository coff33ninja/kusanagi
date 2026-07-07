package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type Agent struct {
	geminiKeys   []string
	geminiModel  string
	temperature  float64
	voiceModel   string
	voiceName    string
	topK         int
	live         LiveProvider
	mcp          MCPProvider
	systemPrompt string
	mu           sync.Mutex
	toolHistory  *toolHistory
}

// toolHistory tracks the last N tool calls for loop detection
type toolHistory struct {
	mu   sync.Mutex
	ring [50]toolCallEntry
	pos  int
	full bool
}

type toolCallEntry struct {
	name      string
	timestamp time.Time
}

func newToolHistory() *toolHistory {
	return &toolHistory{}
}

func (th *toolHistory) record(name string) {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.ring[th.pos] = toolCallEntry{name: name, timestamp: time.Now()}
	th.pos = (th.pos + 1) % len(th.ring)
	if th.pos == 0 {
		th.full = true
	}
}

// detectLoop checks for two patterns:
// 1. Same tool called 4+ times within 30 seconds
// 2. Alternating pair A→B→A→B
// Returns the detected pattern or empty string.
func (th *toolHistory) detectLoop() string {
	th.mu.Lock()
	defer th.mu.Unlock()

	n := th.pos
	if th.full {
		n = len(th.ring)
	}
	if n < 4 {
		return ""
	}

	now := time.Now()

	// Pattern 1: frequency — same tool 4+ times in 30s
	counts := make(map[string]int)
	for i := 0; i < n; i++ {
		entry := th.ring[i]
		if now.Sub(entry.timestamp) > 30*time.Second {
			continue
		}
		counts[entry.name]++
	}
	for name, count := range counts {
		if count >= 4 {
			return fmt.Sprintf("LOOP DETECTED: %s called %d times in 30s", name, count)
		}
	}

	// Pattern 2: alternating pair A→B→A→B
	if n >= 4 {
		last := n - 1
		if last >= 3 &&
			th.ring[last].name == th.ring[last-2].name &&
			th.ring[last-1].name == th.ring[last-3].name &&
			th.ring[last].name != th.ring[last-1].name {
			return fmt.Sprintf("LOOP DETECTED: alternating pattern %s ↔ %s",
				th.ring[last].name, th.ring[last-1].name)
		}
	}

	return ""
}

type MCPProvider interface {
	CallTool(name string, args map[string]any) (string, error)
}

type LiveProvider interface {
	Run(ctx context.Context) error
}

func New(geminiKeys []string, geminiModel string, temperature float64, voiceModel, voiceName string, topK int, mcp MCPProvider) *Agent {
	a := &Agent{
		geminiKeys:  geminiKeys,
		geminiModel: geminiModel,
		temperature: temperature,
		voiceModel:  voiceModel,
		voiceName:   voiceName,
		topK:        topK,
		mcp:         mcp,
	}
	a.systemPrompt = a.buildSystemPrompt()
	return a
}

func (a *Agent) SetLive(live LiveProvider)  { a.live = live }
func (a *Agent) SystemPrompt() string       { return a.systemPrompt }

// ToolCallHandler returns a function suitable for LiveConfig.OnToolCall.
// It records the tool call in history and logs loop detections.
func (a *Agent) ToolCallHandler() func(name string, success bool) {
	a.toolHistory = newToolHistory()
	return func(name string, success bool) {
		a.toolHistory.record(name)
		if loop := a.toolHistory.detectLoop(); loop != "" {
			slog.Warn(loop)
		}
	}
}

// AuditHandler returns a function suitable for LiveConfig.AuditFunc.
func (a *Agent) AuditHandler() func(key, value, tags string) {
	return func(key, value, tags string) {
		a.logAudit(key, value, tags)
	}
}

func (a *Agent) buildSystemPrompt() string {
	return `You are Kusanagi, an AI assistant with computer-use abilities on Windows 11.

## RULE: ONLY CALL TOOLS WHEN THE USER ASKS FOR A COMPUTER ACTION
For casual conversation, greetings, questions — just respond naturally with text. Do NOT call any tools.
Only use tools when the user explicitly asks you to: click, type, screenshot, open/close windows, read/write files, control volume/brightness, or perform any computer action.

## THE PERCEIVE-REASON-ACT LOOP
When you DO need to use tools:
1. PERCEIVE — screenshot + OCR to understand current screen state
2. REASON — decide what action achieves the goal
3. ACT — call one tool (click, type, key_press, etc.)
4. OBSERVE — screenshot + OCR to verify the result
5. REPEAT — until the goal is reached
Start every task with a screenshot. Never assume screen state.

## CHAIN TOOL — MULTI-STEP ORCHESTRATION (PREFERRED)
Use the chain tool to execute multi-step sequences server-side in a single call instead of calling tools one at a time. This eliminates round-trip latency.
Step types: tool (call any tool), wait (delay N ms), poll (poll OCR until text appears), if/else (branch based on OCR text presence), loop (repeat N times), verify (OCR before/after to confirm change).
Use {{variable}} to pass captured output between steps. Set capture on a step to store its result, then reference it in later steps.
Always prefer chain for any sequence of 2+ steps: "open notepad, type text, save, close", "find the search box, type query, press enter, wait for results".

## BACKGROUND ONNX WATCHER — PASSIVE SCREEN MONITORING
The ONNX watcher is running in the background (started at Kusanagi startup, polling every 5 seconds). It continuously runs YOLO UI element detection on the screen and caches results.
Use onnx_watch_cache to retrieve the most recent detections at any time — this gives you a snapshot of what UI elements are currently on screen without taking a new screenshot. Detected elements include their class labels, confidence scores, and bounding boxes.
Use onnx_watch_status to check if the watcher is running. Use onnx_watch_stop to disable it. This is useful for understanding screen layout passively over time.

## KEYLOGGER — RECORD USER ACTIONS
Use keylogger_start to begin recording keyboard and mouse input. Use keylogger_stop to get the recorded sequence as chain-compatible steps that can be replayed.
Call keylogger_status to check if recording is active and see event count.
When the user demonstrates a multi-step workflow, start the keylogger, let them perform the actions, then stop it and replay the returned steps. This lets you learn and repeat complex procedures without being told each step.

## TASK INTROSPECTION
Wrap multi-step tasks with task_begin (before starting) and task_end (after finishing). After task_end, call introspection_analyze to get mined insights: slowest tools, most-failed tools, OCR stats, repeated patterns, and improvement suggestions.

## ADAPTIVE ENGINE
agent_analyze returns per-tool timing stats, success rates, and learned OCR→command associations. agent_suggest(ocr_text) predicts the best next command from past patterns with confidence scores. agent_train rebuilds the prediction model from recent data.

## BEHAVIORAL RULES
- NEVER INVENT DATA — hard rule. Never make up coordinates, window handles, positions, file contents, or any data not received from a tool. If a tool errors or returns 'ok' without data, say so and ask the user. Do not guess.
- REUSE PREVIOUS RESULTS — Window handles, coordinates, file paths stay in history once discovered. Do NOT call list_windows or get_system_info again for data you already have.

- ANNOUNCE YOUR ACTIONS — Before calling any tool, EXPLAIN what you're about to do. After it executes, state the result briefly.
- ERROR RECOVERY — If an action fails: (1) screenshot, (2) if click missed, OCR to find correct coordinates and retry, (3) if text went to wrong window, focus the right one first, (4) if UIPI blocked, tell user server needs admin mode.
- STARTUP STATE IS IN HISTORY — System info, displays, disk, battery, uptime, volume, open windows were probed at startup. Read from history instead of re-probing.

## TOOL RESULT SEMANTICS
Tools fall into two categories with different return behavior:
1. QUERY tools return structured JSON with real data: screenshot (base64 PNG), ocr (text + bounding boxes), list_windows (window array), get_system_info (hostname, OS, RAM), memory_search (fact array), datalog_query (row array), get_battery (level %), get_disk_usage (free/total), find_image (coordinates + score), list_displays, get_volume, find_text_and_click (clicked coords), uia_find (element rect).
2. ACTION tools return the string "ok" which ONLY means the Windows API call did not crash — NOT that the action succeeded or changed anything: click, type, key_press, key_down, key_up, scroll, drag, set_volume, set_brightness, set_mute, move_window, focus_window, minimize_window, maximize_window, kill_process, set_clipboard, memory_set, memory_forget, template_store, template_forget, training_mark_used, set_config.
NEVER try to parse "ok" as JSON. ALWAYS verify action results with screenshot + OCR.

## OCR: FULL SCREEN VS REGION
Full-screen OCR (ocr with no position args) is slow — it captures and OCRs the entire display.
Region OCR (ocr with x, y, w, h) is 10x faster and more accurate. Use it when you know where the target text appears: OCR a specific button area, a menu region, a text field, a toolbar.
Strategy: start with one full-screen OCR to understand layout, then use region OCR for follow-up reads.

## SCREENSHOT OUTPUT IS REUSABLE
screenshot returns a base64-encoded PNG string. Pass this same PNG data directly to:
- find_image(template_b64, screen_b64) — locate a template in the screenshot
- find_all_images(template_b64, screen_b64) — find all occurrences
- onnx_detect(image_b64) — run YOLO UI element detection
This avoids taking a second screenshot when analyzing the current screen.

## UI AUTOMATION (UIA) — PREFER OVER OCR
For Windows standard controls, UIA tools are more reliable than OCR+click:
- uia_find(name, automation_id, control_type) — locate elements programmatically by name, ID, or type
- uia_get_text(name) — read text directly from an element (no OCR needed)
- uia_invoke(name) — click/invoke an element by name without pixel coordinates
Use UIA first for known window elements (buttons, text boxes, menus, list items). Fall back to OCR+click for anything UIA cannot find (custom controls, web content, images).

## TEMPLATE MATCHING — PERSISTENT VISUAL LANDMARKS
Use template_store(element_key, center_x, center_y) to save a visual crop of any UI element. Later use template_find(element_key) to relocate it even after the window moves or resizes. template_list shows stored templates, template_forget removes them.
Useful for buttons, icons, or landmarks that you need to find reliably across sessions.

## DATALOG — YOUR OWN HISTORY
datalog_query(table, source, tool, success, limit, offset) lets you query your own past actions:
- table="commands" — all tool calls with timing, success/failure
- table="chains" — multi-step chain executions
- table="ocr" — recent OCR snapshots with full text and timestamps
- table="pairs" — OCR-before/command/OCR-after training pairs
This is more powerful than the audit trail — it includes exact timestamps, success rates per tool, and OCR context surrounding each command. Use it to diagnose patterns of failure or to recall what happened in a prior session.

## TRAINING PIPELINE
The server learns from screenshots and commands to improve UI detection over time:
- training_save_sample(category, task_prompt) — intentionally capture a training sample with a description of the task (categories: click, type, navigate, general)
- training_stats — see total samples, unused samples, breakdown by category, disk usage
- training_list_samples(unused_only) — browse captured samples
- training_cleanup_noise(max_age_hours, dry_run) — remove low-quality samples
- training_mark_used(id) — mark a sample as trained
The ONNX model (YOLO UI element detection) retrains from these samples during idle time. Good samples make detection more accurate.

## RUNTIME CONFIGURATION
set_config can change server behavior at runtime without restart:
- training_enabled (true/false) — start/stop background screenshot saving
- watcher_enabled (true/false) — background ONNX detection watcher
- watcher_interval_seconds — how often the watcher polls
- log_level (debug/info/warn/error)
Use set_config to adjust behavior mid-session.

## MEMORY
The MCP server has built-in memory tools backed by SQLite with FTS5 full-text search:
- memory_set(key, value, scope, tags, ttl) — store or update a fact (upsert by key+scope)
- memory_get(key, scope) — retrieve a specific fact
- memory_search(query, scope) — FTS5 full-text search across key, value, scope, and tags. Supports SQLite FTS5 syntax (phrase search, prefix, etc.)
- memory_list(scope, tags, limit) — browse facts filtered by scope and/or tags
- memory_forget(key, scope, tags) — delete facts (requires at least one filter)
Facts are persistent across sessions and shared with the training/introspection systems.
- Store user preferences with scope='user_profile'
- Store important tool results with scope='kusanagi'
- Every tool call is automatically logged to scope='audit' — search it to learn from past patterns
- At start of each task, search memory for 'behavioral rules' and check scope='user_profile'
- Build a user profile over time: name, preferred browser, common tasks, window layouts
- Use tags to categorize facts (comma-separated). Set ttl in seconds for auto-expiry.

## STARTUP STATE
System was probed at startup — system_info, screen_size, displays, disk_usage, uptime, battery, volume, open windows. Results are in conversation history.`
}



func (a *Agent) logAudit(key, value, tags string) {
	go func() {
		_, err := a.mcp.CallTool("memory_set", map[string]any{
			"key":   key,
			"value": value,
			"scope": "audit",
			"tags":  tags,
		})
		if err != nil {
			slog.Error("audit log failed", "error", err)
		}
	}()
}

func (a *Agent) initRules() {
	rules := []struct{ key, text string }{
		{"rule_anti_hallucination",
			"NEVER INVENT DATA — hard rule. Never make up coordinates, window handles, positions, file contents, or any data not received from a tool. If a tool errors, times out, or returns 'ok' without data, say so and ask the user. Do not guess, fabricate, or assume values. Re-read tool results from history."},
		{"rule_reuse_results",
			"REUSE PREVIOUS RESULTS — Window handles, coordinates, file paths, and window titles stay in the conversation history once discovered. Do NOT call list_windows, get_system_info, or other discovery tools again for data you already have. Only re-probe if state may have changed."},

		{"rule_error_recovery",
			"ERROR RECOVERY — If an action fails: (1) screenshot to see what happened, (2) if click missed, OCR to find correct coordinates and retry, (3) if text went to wrong window, focus the right one first, (4) if UIPI blocked, tell user server needs admin mode, (5) use agent_analyze to check per-tool success rates."},
		{"rule_tool_best_practices",
			"TOOL BEST PRACTICES — Start every task with a screenshot. Prefer find_text_and_click over hardcoded coordinates. Use wait_for_text instead of fixed delays. Chain multi-step sequences with the chain tool. Window handles beat pixel coordinates. Use get_window_state to check position before moving windows. OCR with region is faster than full screen."},
		{"rule_task_introspection",
			"TASK INTROSPECTION — Wrap major tasks with task_begin and task_end. Call introspection_analyze afterward for mined insights about slow tools, failures, and improvement suggestions."},
	}
	for _, r := range rules {
		_, err := a.mcp.CallTool("memory_set", map[string]any{
			"key":   r.key,
			"value": r.text,
			"scope": "kusanagi",
			"tags":  "rule,guideline",
		})
		if err != nil {
			slog.Error("rule init failed", "rule_key", r.key, "error", err)
		}
	}
}

func (a *Agent) runSystemProbes() (map[string]string, string) {
	type probeResult struct {
		name string
		data string
	}
	probes := []string{
		"get_system_info", "get_screen_size", "list_displays",
		"get_disk_usage", "get_uptime", "get_battery",
		"get_volume", "list_windows",
	}
	resultCh := make(chan probeResult, len(probes))
	for _, name := range probes {
		go func(n string) {
			result, err := a.mcp.CallTool(n, map[string]any{})
			if err != nil {
				resultCh <- probeResult{n, ""}
				return
			}
			resultCh <- probeResult{n, result}
		}(name)
	}
	state := make(map[string]string)
	for range probes {
		pr := <-resultCh
		if pr.data != "" {
			state[pr.name] = pr.data
		}
	}
	stateJSON, jErr := json.Marshal(state)
	if jErr != nil {
		slog.Error("state marshal failed", "error", jErr)
	}
	return state, string(stateJSON)
}

type validationItem struct {
	name   string
	status string // PASS, FAIL, WARN
	detail string
}

func (a *Agent) validate() []validationItem {
	var results []validationItem

	// 1. Config validity (static fields already checked by config.Load)
	results = append(results, validationItem{
		name: "config", status: "PASS", detail: fmt.Sprintf("model=%s voice=%s", a.geminiModel, a.voiceName),
	})

	// 2. MCP connection — probe with a no-arg tool
	if _, err := a.mcp.CallTool("get_system_info", map[string]any{}); err != nil {
		results = append(results, validationItem{name: "mcp_server", status: "FAIL", detail: err.Error()})
	} else {
		results = append(results, validationItem{name: "mcp_server", status: "PASS", detail: "responding"})
	}

	// 3. Memory system
	if _, err := a.mcp.CallTool("memory_set", map[string]any{
		"key": "healthcheck", "value": "ok", "scope": "health", "ttl": 60,
	}); err != nil {
		results = append(results, validationItem{name: "memory_store", status: "FAIL", detail: err.Error()})
	} else {
		if got, err := a.mcp.CallTool("memory_get", map[string]any{"key": "healthcheck", "scope": "health"}); err != nil || got == "" {
			results = append(results, validationItem{name: "memory_store", status: "FAIL", detail: "write/read round-trip failed"})
		} else {
			results = append(results, validationItem{name: "memory_store", status: "PASS", detail: "SQLite+FTS5 read/write ok"})
		}
	}

	// 4. System probes via probe map
	state, _ := a.runSystemProbes()
	sysInfo := state["get_system_info"]
	if sysInfo == "" {
		results = append(results, validationItem{name: "system_probe", status: "FAIL", detail: "no system info returned"})
	} else {
		results = append(results, validationItem{name: "system_probe", status: "PASS", detail: truncate(sysInfo, 120)})
	}

	// 5. Screen capture (screenshot + OCR)
	screen, err := a.mcp.CallTool("get_screen_size", map[string]any{})
	if err != nil || screen == "" {
		results = append(results, validationItem{name: "display", status: "FAIL", detail: fmt.Sprintf("screen size: %v", err)})
	} else {
		results = append(results, validationItem{name: "display", status: "PASS", detail: truncate(screen, 80)})
	}

	ss, err := a.mcp.CallTool("screenshot", map[string]any{})
	if err != nil || ss == "" || ss == "ok" {
		results = append(results, validationItem{name: "screenshot", status: "FAIL", detail: fmt.Sprintf("screenshot: %v", err)})
	} else {
		results = append(results, validationItem{name: "screenshot", status: "PASS", detail: fmt.Sprintf("%d bytes", len(ss))})
	}

	// 6. Audio — microphone check
	mics, err := a.mcp.CallTool("list_audio_devices", map[string]any{})
	if err != nil || mics == "" {
		results = append(results, validationItem{name: "audio_devices", status: "WARN", detail: fmt.Sprintf("list failed: %v", err)})
	} else {
		results = append(results, validationItem{name: "audio_devices", status: "PASS", detail: truncate(mics, 100)})
	}

	return results
}

func (a *Agent) printValidationReport(items []validationItem) {
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════╗")
	fmt.Println("  ║         KUSANAGI SYSTEM VALIDATION           ║")
	fmt.Println("  ╚══════════════════════════════════════════════╝")
	pass, fail, warn := 0, 0, 0
	for _, item := range items {
		symbol := "✓"
		switch item.status {
		case "PASS":
			symbol = "✓"
			pass++
		case "FAIL":
			symbol = "✗"
			fail++
		case "WARN":
			symbol = "!"
			warn++
		}
		fmt.Printf("  %s %s", symbol, item.name)
		padding := 25 - len(item.name)
		if padding > 0 {
			fmt.Print(strings.Repeat(" ", padding))
		}
		fmt.Printf(" %s\n", item.detail)
	}
	fmt.Printf("\n  %d passed, %d failed, %d warnings\n", pass, fail, warn)
	if fail > 0 {
		fmt.Println("  ⚠ Some checks failed — agent may have limited functionality")
	}
	fmt.Println()
}

func (a *Agent) Run() error {
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════╗")
	fmt.Println("  ║              KUSANAGI AGENT                  ║")
	fmt.Println("  ║     Voice-driven AI with computer-use        ║")
	fmt.Println("  ╚══════════════════════════════════════════════╝")
	fmt.Println()

	slog.Info("starting up")

	// Phase 1: Validation
	report := a.validate()
	a.printValidationReport(report)

	// Phase 2: System probe for greeting + history
	state, stateJSON := a.runSystemProbes()

	greeting := a.buildGreeting(state)
	slog.Info("greeting", "text", greeting)

	a.logAudit(fmt.Sprintf("startup_%d", time.Now().UnixNano()),
		fmt.Sprintf("Startup state: %s", truncate(stateJSON, 1000)),
		"startup_probe")

	a.initRules()

	// Start background ONNX watcher so the AI can detect UI elements passively
	if result, err := a.mcp.CallTool("onnx_watch_start", map[string]any{
		"interval_seconds": 5,
	}); err != nil {
		slog.Error("ONNX watcher start failed", "error", err)
	} else {
		slog.Info("ONNX watcher started", "result", result)
	}

	if a.live != nil {
		slog.Info("entering Gemini Live mode")
		fmt.Println("\n  Microphone active — start speaking when ready")

		// Build a contextual initial prompt so Gemini speaks first
		initPrompt := fmt.Sprintf(
			"[Startup state: %s] Say a brief, natural greeting acknowledging the current system state, then say you're ready.",
			truncate(stateJSON, 500),
		)
		if lp, ok := a.live.(interface{ SetInitialPrompt(string) }); ok {
			lp.SetInitialPrompt(initPrompt)
		}

		return a.live.Run(context.Background())
	}

	slog.Warn("no Live provider configured")
	return nil
}

func (a *Agent) buildGreeting(state map[string]string) string {
	parts := []string{"Kusanagi online."}

	if data := state["get_system_info"]; data != "" {
		var si map[string]any
		if json.Unmarshal([]byte(data), &si) == nil {
			if host, ok := si["hostname"].(string); ok {
				parts = append(parts, fmt.Sprintf("Running on %s", host))
			}
			if osName, ok := si["os"].(string); ok {
				parts = append(parts, osName)
			}
		}
	}

	if data := state["list_displays"]; data != "" {
		var displays []map[string]any
		if json.Unmarshal([]byte(data), &displays) == nil {
			parts = append(parts, fmt.Sprintf("%d display(s)", len(displays)))
		}
	}

	if data := state["get_disk_usage"]; data != "" {
		var du map[string]any
		if json.Unmarshal([]byte(data), &du) == nil {
			if free, ok := du["free"].(string); ok {
				parts = append(parts, fmt.Sprintf("%s free", free))
			}
		}
	}

	if data := state["get_battery"]; data != "" {
		var bat map[string]any
		if json.Unmarshal([]byte(data), &bat) == nil {
			if level, ok := bat["level"].(float64); ok {
				parts = append(parts, fmt.Sprintf("Battery %.0f%%", level))
			}
		}
	}

	if data := state["get_uptime"]; data != "" {
		var up map[string]any
		if json.Unmarshal([]byte(data), &up) == nil {
			if secs, ok := up["uptime_seconds"].(float64); ok {
				h := int(secs) / 3600
				m := (int(secs) % 3600) / 60
				if h > 0 {
					parts = append(parts, fmt.Sprintf("Up %dh %dm", h, m))
				} else {
					parts = append(parts, fmt.Sprintf("Up %dm", m))
				}
			}
		}
	}

	return strings.Join(parts, ". ") + "."
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
