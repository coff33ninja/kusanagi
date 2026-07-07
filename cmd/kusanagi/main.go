package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/coff33ninja/kusanagi/internal/agent"
	"github.com/coff33ninja/kusanagi/internal/audio"
	"github.com/coff33ninja/kusanagi/internal/config"
	"github.com/coff33ninja/kusanagi/internal/gemini"
	"github.com/coff33ninja/kusanagi/internal/mcp"
)

var Version = "dev"

type liveAudioSink struct {
	mu     sync.Mutex
	player *audio.Player
}

func (s *liveAudioSink) Play(audioData []byte) error {
	s.mu.Lock()
	if s.player == nil {
		var err error
		s.player, err = audio.NewPlayer(24000)
		if err != nil {
			s.mu.Unlock()
			return err
		}
	}
	p := s.player
	s.mu.Unlock()
	return p.Play(audioData)
}

type liveAudioSource struct{}

func (liveAudioSource) Start(sampleRate int, chunkDuration time.Duration) (<-chan []byte, func() error, error) {
	return audio.StartStream(sampleRate, chunkDuration)
}

func main() {
	logFile, err := os.OpenFile("kusanagi.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	slog.Info("Kusanagi starting", "version", Version)

	configPath := flag.String("config", "config.json", "path to config file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Kusanagi (Go) v%s\n", Version)
		fmt.Println("Voice-driven AI agent with computer-use abilities on Windows 11")
		return
	}

	root, err := os.Getwd()
	if err != nil {
		slog.Error("getwd failed", "error", err)
		root = "."
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config load failed", "error", err)
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// Start MCP server
	client := mcp.NewClient()

	for _, entry := range cfg.MCP.ServerScripts {
		cmd := entry.ResolvedCommand(root)
		args := entry.ResolvedArgs(root)
		slog.Info("starting MCP server", "cmd", cmd)
		if err := client.Start(cmd, args); err != nil {
			slog.Error("MCP start failed", "error", err)
			os.Exit(1)
		}
	}

	if err := client.Initialize(); err != nil {
		slog.Error("MCP init failed", "error", err)
		os.Exit(1)
	}

	tools, err := client.ListTools()
	if err != nil {
		slog.Error("MCP list tools failed", "error", err)
		os.Exit(1)
	}
	slog.Info("MCP connected", "tool_count", len(tools))

	toolDecls := client.ToGeminiDeclarations()

	ag := agent.New(cfg.GeminiKeys, cfg.GeminiModel, cfg.Temperature, cfg.VoiceModel, cfg.VoiceName, cfg.RAG.TopK, client)

	// Create Live client for voice-only mode
	pickedKey := cfg.GeminiKeys[0]
	if len(cfg.GeminiKeys) > 1 {
		kr := gemini.NewKeyRing(cfg.GeminiKeys)
		pickedKey = kr.Pick()
	}
	liveCfg := gemini.LiveConfig{
		APIKey:       pickedKey,
		Model:        cfg.LiveModel,
		SystemPrompt: ag.SystemPrompt(),
		VoiceName:    cfg.VoiceName,
		Temperature:  cfg.Temperature,
		Tools:        toolDecls,
		AuditFunc:    ag.AuditHandler(),
		OnToolCall:   ag.ToolCallHandler(),
	}
	liveClient := gemini.NewLiveClient(liveCfg, client, liveAudioSource{}, &liveAudioSink{})
	ag.SetLive(liveClient)

	if err := os.MkdirAll(filepath.Join(root, "memories"), 0755); err != nil {
		slog.Warn("memories dir creation failed", "error", err)
	}

	// Signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down gracefully...")
		if err := client.Close(); err != nil {
			slog.Error("MCP close error", "error", err)
		}
		os.Exit(0)
	}()

	slog.Info("Kusanagi ready")
	if err := ag.Run(); err != nil {
		slog.Error("agent run failed", "error", err)
		os.Exit(1)
	}

	if err := client.Close(); err != nil {
		slog.Error("MCP close error", "error", err)
	}

	fmt.Println("\nGoodbye.")
}


