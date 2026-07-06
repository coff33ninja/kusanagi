package gemini

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type KeyStats struct {
	Key          string    `json:"key"`
	Requests     int64     `json:"requests"`
	PromptTokens int64     `json:"prompt_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	Errors       int64     `json:"errors"`
	RateLimited  int64     `json:"rate_limited"`
	LastSuccess  time.Time `json:"last_success"`
	LastError    time.Time `json:"last_error"`

	LimitRequests     int    `json:"limit_requests"`
	RemainingRequests int    `json:"remaining_requests"`
	LimitTokens       int    `json:"limit_tokens"`
	RemainingTokens   int    `json:"remaining_tokens"`
	ResetRequests     string `json:"reset_requests"`
}

type KeyRing struct {
	mu       sync.Mutex
	keys     []string
	stats    map[string]*KeyStats
}

func NewKeyRing(keys []string) *KeyRing {
	kr := &KeyRing{
		keys:  keys,
		stats: make(map[string]*KeyStats, len(keys)),
	}
	for _, k := range keys {
		kr.stats[k] = &KeyStats{Key: k}
	}
	return kr
}

func (kr *KeyRing) Pick() string {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	if len(kr.keys) == 0 {
		return ""
	}
	if len(kr.keys) == 1 {
		return kr.keys[0]
	}

	var bestKey string
	bestScore := math.MaxFloat64

	for _, k := range kr.keys {
		s := kr.stats[k]
		score := float64(s.Requests + s.Errors*10)

		if time.Since(s.LastError) < 30*time.Second {
			score += 50
		}
		if time.Since(s.LastError) < 5*time.Second {
			score += 200
		}

		// Prefer keys with remaining quota headroom
		if s.RemainingRequests > 0 {
			score -= float64(s.RemainingRequests) * 0.5
		}
		if s.RemainingTokens > 0 {
			score -= float64(s.RemainingTokens) / 100000 * 0.5
		}

		if score < bestScore {
			bestScore = score
			bestKey = k
		}
	}

	if bestKey == "" {
		bestKey = kr.keys[0]
	}
	return bestKey
}

func (kr *KeyRing) RecordResponse(key string, statusCode int, header http.Header, promptTokens, outputTokens int64) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	s, ok := kr.stats[key]
	if !ok {
		return
	}

	s.Requests++

	if header != nil {
		s.LimitRequests = getHeaderInt(header, "x-ratelimit-limit-requests", s.LimitRequests)
		s.RemainingRequests = getHeaderInt(header, "x-ratelimit-remaining-requests", s.RemainingRequests)
		s.LimitTokens = getHeaderInt(header, "x-ratelimit-limit-tokens", s.LimitTokens)
		s.RemainingTokens = getHeaderInt(header, "x-ratelimit-remaining-tokens", s.RemainingTokens)
		if reset := header.Get("x-ratelimit-reset-requests"); reset != "" {
			s.ResetRequests = reset
		}
	}

	switch {
	case statusCode == 429:
		s.RateLimited++
		s.LastError = time.Now()
	case statusCode >= 400:
		s.Errors++
		s.LastError = time.Now()
	default:
		s.PromptTokens += promptTokens
		s.OutputTokens += outputTokens
		s.LastSuccess = time.Now()
	}
}

func getHeaderInt(h http.Header, key string, fallback int) int {
	v := h.Get(key)
	if v == "" {
		return fallback
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return fallback
}

func (kr *KeyRing) EstimatedCost(key string) float64 {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	s, ok := kr.stats[key]
	if !ok {
		return 0
	}
	return float64(s.PromptTokens)/1_000_000*0.30 + float64(s.OutputTokens)/1_000_000*2.50
}

func (kr *KeyRing) Stats() []KeyStats {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	out := make([]KeyStats, 0, len(kr.stats))
	for _, s := range kr.stats {
		out = append(out, *s)
	}
	return out
}

func (kr *KeyRing) StatsJSON() string {
	stats := kr.Stats()
	b, _ := json.MarshalIndent(stats, "", "  ")
	return string(b)
}

func (kr *KeyRing) FormatStats() string {
	stats := kr.Stats()
	var out string
	for _, s := range stats {
		cost := float64(s.PromptTokens)/1_000_000*0.30 + float64(s.OutputTokens)/1_000_000*2.50
		masked := maskKey(s.Key)
		out += fmt.Sprintf("  %s: %d req | %d in + %d out tok | %d err | %d rl | $%.4f",
			masked, s.Requests, s.PromptTokens, s.OutputTokens, s.Errors, s.RateLimited, cost)

		if s.RemainingRequests > 0 || s.RemainingTokens > 0 {
			out += fmt.Sprintf(" | quota: %d/%d req %d/%d tok",
				s.RemainingRequests, s.LimitRequests,
				s.RemainingTokens, s.LimitTokens)
		}
		if !s.LastSuccess.IsZero() {
			out += fmt.Sprintf(" | last ok: %s", s.LastSuccess.Format("15:04:05"))
		}
		out += "\n"
	}
	return out
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func extractUsage(data map[string]any) (promptTokens, outputTokens int64) {
	usage, _ := data["usageMetadata"].(map[string]any)
	if usage == nil {
		return 0, 0
	}
	if pt, ok := usage["promptTokenCount"].(float64); ok {
		promptTokens = int64(pt)
	}
	if ot, ok := usage["candidatesTokenCount"].(float64); ok {
		outputTokens = int64(ot)
	}
	return
}

func extractUsageFromSSE(dataStr string) (promptTokens, outputTokens int64) {
	var data map[string]any
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return 0, 0
	}
	return extractUsage(data)
}
