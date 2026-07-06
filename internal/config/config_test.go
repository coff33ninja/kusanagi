package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.GeminiModel != "gemini-3.1-flash-lite" {
		t.Errorf("expected default model, got %s", cfg.GeminiModel)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected 0.7, got %f", cfg.Temperature)
	}
	if cfg.LiveModel != "gemini-3.1-flash-live-preview" {
		t.Errorf("expected live model gemini-3.1-flash-live-preview, got %s", cfg.LiveModel)
	}
	if cfg.VoiceName != "Aoede" {
		t.Errorf("expected Aoede, got %s", cfg.VoiceName)
	}
	if cfg.RAG.TopK != 5 {
		t.Errorf("expected TopK 5, got %d", cfg.RAG.TopK)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("nonexistent.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadEmptyKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"gemini_keys":[],"mcp":{"server_scripts":[]}}`), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty keys")
	}
}

func TestLoadShortKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"gemini_keys":["short"],"mcp":{"server_scripts":[]}}`), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestLoadBadTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"gemini_keys":["abcdefghijklmnop"],"temperature":3.0,"mcp":{"server_scripts":[]}}`), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for bad temperature")
	}
}

func TestLoadNoServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"gemini_keys":["abcdefghijklmnop"],"mcp":{"server_scripts":[]}}`), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for no server scripts")
	}
}

func TestLoadMissingBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"gemini_keys":["abcdefghijklmnop"],"mcp":{"server_scripts":[{"command":"nonexistent.exe","args":[]}]}}`), 0644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestResolvedCommand(t *testing.T) {
	entry := ServerEntry{Command: "foo.exe", Args: []string{"--flag", "config.json"}}
	root := "C:\\test"
	cmd := entry.ResolvedCommand(root)
	args := entry.ResolvedArgs(root)
	if cmd != "C:\\test\\foo.exe" && cmd != "C:/test/foo.exe" {
		t.Errorf("expected resolved path, got %s", cmd)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "--flag" {
		t.Errorf("expected --flag unchanged, got %s", args[0])
	}
	expected := "C:\\test\\config.json"
	if args[1] != expected {
		t.Errorf("expected %s, got %s", expected, args[1])
	}
}

func TestResolvedCommandAbsolute(t *testing.T) {
	entry := ServerEntry{Command: "C:\\tools\\server.exe", Args: []string{"C:\\data\\cfg.json"}}
	root := "C:\\test"
	cmd := entry.ResolvedCommand(root)
	args := entry.ResolvedArgs(root)
	if cmd != "C:\\tools\\server.exe" {
		t.Errorf("expected absolute path unchanged, got %s", cmd)
	}
	if len(args) != 1 || args[0] != "C:\\data\\cfg.json" {
		t.Errorf("expected absolute arg unchanged, got %v", args)
	}
}
