package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type LiveToolExecutor interface {
	CallTool(name string, args map[string]any) (string, error)
}

type LiveAudioSource interface {
	Start(sampleRate int, chunkDuration time.Duration) (<-chan []byte, func() error, error)
}

type LiveAudioSink interface {
	Play(audioData []byte) error
}

func DefaultLiveModel() string { return "gemini-3.1-flash-live-preview" }

type LiveConfig struct {
	APIKey       string
	Model        string
	SystemPrompt string
	VoiceName    string
	Temperature  float64
	Tools        []map[string]any
	InitialPrompt string // sent after setupComplete to trigger first response
	AuditFunc    func(key, value, tags string) // called after tool execution
	OnToolCall   func(name string, success bool) // called after each tool execution (loop detection etc)
}

type LiveClient struct {
	cfg       LiveConfig
	executor  LiveToolExecutor
	audioSrc  LiveAudioSource
	audioSink LiveAudioSink
}

func NewLiveClient(cfg LiveConfig, executor LiveToolExecutor, audioSrc LiveAudioSource, audioSink LiveAudioSink) *LiveClient {
	return &LiveClient{cfg: cfg, executor: executor, audioSrc: audioSrc, audioSink: audioSink}
}

func (lc *LiveClient) SetInitialPrompt(prompt string) { lc.cfg.InitialPrompt = prompt }

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (lc *LiveClient) Run(ctx context.Context) error {
	modelName := lc.cfg.Model
	if modelName == "" {
		modelName = DefaultLiveModel()
	}
	endpoint := fmt.Sprintf(
		"wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s",
		url.QueryEscape(lc.cfg.APIKey),
	)

	conn, _, err := websocket.Dial(ctx, endpoint, nil)
	if err != nil {
		return fmt.Errorf("live ws dial: %w", err)
	}
	conn.SetReadLimit(50 * 1024 * 1024) // 50MB limit for large tool responses
	defer conn.Close(websocket.StatusNormalClosure, "session done")

	setup := map[string]any{
		"setup": map[string]any{
			"model": "models/" + modelName,
			"generationConfig": map[string]any{
				"temperature":       lc.cfg.Temperature,
				"responseModalities": []string{"AUDIO"},
				"speechConfig": map[string]any{
					"voiceConfig": map[string]any{
						"prebuiltVoiceConfig": map[string]any{
							"voiceName": lc.cfg.VoiceName,
						},
					},
				},
			},
			"systemInstruction": map[string]any{
				"parts": []map[string]any{{"text": lc.cfg.SystemPrompt}},
			},
			"realtimeInputConfig": map[string]any{
				"automaticActivityDetection": map[string]any{
					"disabled":                false,
					"startOfSpeechSensitivity": "START_SENSITIVITY_LOW",
					"endOfSpeechSensitivity":   "END_SENSITIVITY_LOW",
					"prefixPaddingMs":          20,
					"silenceDurationMs":        500,
				},
			},
		},
	}

	if len(lc.cfg.Tools) > 0 {
		setupMap := setup["setup"].(map[string]any)
		setupMap["tools"] = lc.cfg.Tools
	}

	setupJSON, _ := json.Marshal(setup)
	if err := conn.Write(ctx, websocket.MessageText, setupJSON); err != nil {
		return fmt.Errorf("live write setup: %w", err)
	}
	log.Println("Live: setup sent, waiting for setupComplete...")

	audioCh, stopCapture, err := lc.audioSrc.Start(24000, 200*time.Millisecond)
	if err != nil {
		return fmt.Errorf("live audio start: %w", err)
	}
	defer stopCapture()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		lc.sendAudioLoop(ctx, conn, audioCh)
	}()

	recvErr := lc.receiveLoop(ctx, conn, cancel)
	wg.Wait()

	return recvErr
}

func (lc *LiveClient) sendAudioLoop(ctx context.Context, conn *websocket.Conn, audioCh <-chan []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Live: audio loop panicked: %v", r)
		}
	}()
	var chunkCount int
	lastLog := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-audioCh:
			if !ok {
				return
			}
			chunkCount++
			b64 := base64.StdEncoding.EncodeToString(chunk)
		msg := map[string]any{
			"realtimeInput": map[string]any{
				"audio": map[string]any{
					"mimeType": "audio/pcm;rate=24000",
					"data":     b64,
				},
			},
		}
			data, _ := json.Marshal(msg)
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				log.Printf("Live: audio send error (stopping): %v", err)
				return
			}
			if chunkCount == 1 {
				log.Println("Live: first audio chunk sent — mic is streaming")
			}
			if time.Since(lastLog) > 5*time.Second {
				log.Printf("Live: sent %d audio chunks so far (%d bytes each)", chunkCount, len(chunk))
				lastLog = time.Now()
			}
		}
	}
}

type liveServerMsg struct {
	SetupComplete  *json.RawMessage      `json:"setupComplete"`
	ServerContent  *liveServerContent    `json:"serverContent"`
	ToolCall       *liveToolCall         `json:"toolCall"`
	Interrupted    *json.RawMessage      `json:"interrupted"`
}

type liveServerContent struct {
	ModelTurn    *liveModelTurn `json:"modelTurn"`
	TurnComplete bool           `json:"turnComplete"`
	Interrupted  bool           `json:"interrupted"`
}

type liveModelTurn struct {
	Parts []livePart `json:"parts"`
}

type livePart struct {
	Text       string          `json:"text"`
	InlineData *liveInlineData `json:"inlineData"`
}

type liveInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type liveToolCall struct {
	FunctionCalls []liveFunctionCall `json:"functionCalls"`
}

type liveFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
	ID   string         `json:"id"`
}

func (lc *LiveClient) receiveLoop(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc) error {
	defer cancel()

	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("live read: %w", err)
		}

		var server liveServerMsg
		if err := json.Unmarshal(msg, &server); err != nil {
			continue
		}

		if server.SetupComplete != nil {
			log.Println("Live: setupComplete received")
			// Send initial prompt now that the connection is confirmed ready
			if lc.cfg.InitialPrompt != "" {
				initMsg := map[string]any{
					"clientContent": map[string]any{
						"turns": []map[string]any{
							{
								"parts": []map[string]any{{"text": lc.cfg.InitialPrompt}},
								"role":  "user",
							},
						},
						"turnComplete": true,
					},
				}
				initJSON, _ := json.Marshal(initMsg)
				if wErr := conn.Write(ctx, websocket.MessageText, initJSON); wErr != nil {
					return fmt.Errorf("live write initial prompt: %w", wErr)
				}
				log.Println("Live: initial prompt sent")
			}
			continue
		}

		if server.Interrupted != nil {
			log.Println("Live: interrupted")
			continue
		}

		if server.ToolCall != nil {
			for _, fc := range server.ToolCall.FunctionCalls {
				log.Printf("Live: tool call: %s", fc.Name)

				// Retry tool calls up to 3 times with exponential backoff
				var resultStr string
				success := false
				for attempt := 0; attempt < 3; attempt++ {
					result, err := lc.executor.CallTool(fc.Name, fc.Args)
					if err == nil {
						resultStr = result
						success = true
						break
					}
					resultStr = fmt.Sprintf("error: %v", err)
					if attempt < 2 {
						delay := time.Duration(500*(1<<attempt)) * time.Millisecond
						log.Printf("Live: retry %s in %v (attempt %d/3): %v", fc.Name, delay, attempt+1, err)
						time.Sleep(delay)
					}
				}

				if !success {
					log.Printf("Live: tool %s failed after 3 attempts", fc.Name)
				}

				if lc.cfg.OnToolCall != nil {
					lc.cfg.OnToolCall(fc.Name, success)
				}

				// Audit log
				if lc.cfg.AuditFunc != nil {
					tags := "tool_call"
					if !success {
						tags = "tool_call,failed"
					}
					lc.cfg.AuditFunc(fc.Name+"_"+time.Now().Format("150405.000000"),
						fmt.Sprintf("tool=%s success=%t result=%s", fc.Name, success, truncateStr(resultStr, 500)),
						tags)
				}

				toolResp := map[string]any{
					"toolResponse": map[string]any{
						"functionResponses": []map[string]any{
							{
								"name":     fc.Name,
								"response": map[string]string{"result": resultStr},
								"id":       fc.ID,
							},
						},
					},
				}
				respJSON, _ := json.Marshal(toolResp)
				if wErr := conn.Write(ctx, websocket.MessageText, respJSON); wErr != nil {
					return fmt.Errorf("live write tool response: %w", wErr)
				}
			}
			continue
		}

		if server.ServerContent != nil {
			sc := server.ServerContent

			if sc.ModelTurn != nil {
				for _, part := range sc.ModelTurn.Parts {
					if part.InlineData != nil && part.InlineData.MimeType == "audio/pcm;rate=24000" {
						audioRaw, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
						if err == nil && len(audioRaw) > 0 {
							if playErr := lc.audioSink.Play(audioRaw); playErr != nil {
								log.Printf("Live: audio play error (continuing): %v", playErr)
							}
						}
					}
					if part.Text != "" {
						log.Printf("Live: %s", part.Text)
					}
				}
			}

			if sc.TurnComplete {
				log.Println("Live: turn complete")
				fmt.Print("\r  Listening... \r")
			}
		}
	}
}
