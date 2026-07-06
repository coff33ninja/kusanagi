package main

import (
	"flag"
	"fmt"
	"log"
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
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("")

	configPath := flag.String("config", "config.json", "path to config file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Kusanagi (Go) v%s\n", Version)
		fmt.Println("Voice-driven AI agent with computer-use abilities on Windows 11")
		return
	}

	root, _ := os.Getwd()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Config: %v", err)
	}

	// Start MCP server
	client := mcp.NewClient()

	for _, entry := range cfg.MCP.ServerScripts {
		cmd := entry.ResolvedCommand(root)
		args := entry.ResolvedArgs(root)
		log.Printf("Starting MCP server: %s", cmd)
		if err := client.Start(cmd, args); err != nil {
			log.Fatalf("MCP start: %v", err)
		}
	}

	if err := client.Initialize(); err != nil {
		log.Fatalf("MCP init: %v", err)
	}

	tools, err := client.ListTools()
	if err != nil {
		log.Fatalf("MCP list tools: %v", err)
	}
	log.Printf("MCP connected: %d tools", len(tools))

	log.Println("Memory: using MCP built-in memory tools (no ChromaDB required)")

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

	os.MkdirAll(filepath.Join(root, "memories"), 0755)

	// Signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down gracefully...")
		if err := client.Close(); err != nil {
			log.Printf("MCP close error: %v", err)
		}
		os.Exit(0)
	}()

	log.Println("Kusanagi ready — voice-driven AI agent with computer-use")
	if err := ag.Run(); err != nil {
		log.Fatalf("Agent: %v", err)
	}

	if err := client.Close(); err != nil {
		log.Printf("MCP close error: %v", err)
	}

	fmt.Println("\nGoodbye.")
}


