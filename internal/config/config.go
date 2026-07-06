package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ── Available Gemini Models (2026) ───────────────────────────────────────────
//
// Free tier (no billing):
//   gemini-3.1-flash-lite         ($0.25/1M in, $1.50/1M out — 15 RPM free)
//   gemini-3-flash-preview        ($0.50/1M in, $3.00/1M out — 10 RPM free, 1500 RPD)
//   gemini-2.5-flash              (stable, $0.15/1M in, $0.60/1M out)
//   gemini-2.5-flash-lite         (stable, $0.075/1M in, $0.30/1M out)
//   gemini-2.0-flash              (legacy, shut down June 1 2026)
//
// Paid (no free tier):
//   gemini-3.5-flash              (stable, latest gen)
//   gemini-3.1-pro-preview        ($2/1M in, $12/1M out)
//   gemini-2.5-pro                (stable, $1.25/1M in, $10/1M out)
//
// Live API (voice/video streaming):
//   gemini-3.1-flash-live-preview ← recommended (replaces gemini-live-2.5-flash-preview)
//   gemini-live-2.5-flash-native-audio (shuts down Dec 13 2026)
//
// TTS models (text-to-speech):
//   gemini-3.1-flash-tts-preview  ← recommended
//   gemini-2.5-flash-preview-tts
//   gemini-2.5-pro-preview-tts
//
// ── Available Live/TTS Voices (30 total) ─────────────────────────────────────
//
//   Zephyr     — Bright      | Puck       — Upbeat     | Charon      — Informative
//   Kore       — Firm        | Fenrir     — Excitable  | Leda        — Youthful
//   Orus       — Firm        | Aoede      — Breezy     | Callirrhoe  — Easy-going
//   Autonoe    — Bright      | Enceladus  — Breathy    | Iapetus     — Clear
//   Umbriel    — Easy-going  | Algieba    — Smooth     | Despina     — Smooth
//   Erinome    — Clear        | Algenib    — Gravelly   | Rasalgethi  — Informative
//   Laomedeia  — Upbeat      | Achernar   — Soft       | Alnilam     — Firm
//   Schedar    — Even        | Gacrux     — Mature     | Pulcherrima — Forward
//   Achird     — Friendly    | Zubenelgenubi — Casual   | Vindemiatrix — Gentle
//   Sadachbia  — Lively      | Sadaltager — Knowledgeable | Sulafat   — Warm

type Config struct {
	GeminiKeys  []string  `json:"gemini_keys"`
	GeminiModel string    `json:"gemini_model"`
	LiveModel   string    `json:"live_model"`
	Temperature float64   `json:"temperature"`
	VoiceModel  string    `json:"voice_model"`
	VoiceName   string    `json:"voice_name"`
	MCP         MCPConfig `json:"mcp"`
	RAG         RAGConfig `json:"rag"`
}

type MCPConfig struct {
	ServerScripts []ServerEntry `json:"server_scripts"`
}

type ServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type RAGConfig struct {
	TopK int `json:"top_k"`
}

func (s ServerEntry) ResolvedCommand(root string) string {
	cmd := s.Command
	if !filepath.IsAbs(cmd) {
		cmd = filepath.Join(root, cmd)
	}
	return cmd
}

func (s ServerEntry) ResolvedArgs(root string) []string {
	args := make([]string, len(s.Args))
	for i, arg := range s.Args {
		if len(arg) > 0 && arg[0] == '-' {
			args[i] = arg
		} else if !filepath.IsAbs(arg) {
			args[i] = filepath.Join(root, arg)
		} else {
			args[i] = arg
		}
	}
	return args
}

func Defaults() *Config {
	return &Config{
		GeminiModel: "gemini-3.1-flash-lite",
		LiveModel:   "gemini-3.1-flash-live-preview",
		Temperature: 0.7,
		VoiceModel:  "gemini-3.1-flash-tts-preview",
		VoiceName:   "Aoede",
		RAG:         RAGConfig{TopK: 5},
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if len(cfg.GeminiKeys) == 0 {
		return nil, fmt.Errorf("no gemini_keys configured — set at least one API key in config.json")
	}
	for i, key := range cfg.GeminiKeys {
		if len(key) < 10 {
			return nil, fmt.Errorf("gemini_keys[%d] looks invalid (too short): %q", i, key)
		}
	}

	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		return nil, fmt.Errorf("temperature must be between 0 and 2, got %f", cfg.Temperature)
	}

	if len(cfg.MCP.ServerScripts) == 0 {
		return nil, fmt.Errorf("no mcp.server_scripts configured")
	}

	root, _ := os.Getwd()
	for i, entry := range cfg.MCP.ServerScripts {
		cmd := entry.ResolvedCommand(root)
		if _, err := os.Stat(cmd); os.IsNotExist(err) {
			return nil, fmt.Errorf("mcp.server_scripts[%d] binary not found: %s", i, cmd)
		}
	}

	return cfg, nil
}
