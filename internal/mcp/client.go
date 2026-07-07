package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

type Client struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Scanner
	stderrBuf bytes.Buffer
	mu        sync.Mutex
	msgID     int
	pending   map[int]chan<- Response
	tools     []ToolDefinition
	closed    bool
}

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	ID     int              `json:"id"`
	Result *json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func NewClient() *Client {
	return &Client{
		pending: make(map[int]chan<- Response),
	}
}

func (c *Client) Start(command string, args []string) error {
	c.cmd = exec.Command(command, args...)

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdin = stdin

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	c.stdout = bufio.NewScanner(stdout)
	c.stdout.Buffer(make([]byte, 0, 512*1024), 10*1024*1024)

	// Capture stderr for diagnostics
	c.cmd.Stderr = &c.stderrBuf

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}

	go c.readLoop()
	return nil
}

func (c *Client) readLoop() {
	for c.stdout.Scan() {
		line := c.stdout.Text()

		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			slog.Warn("mcp: ignoring non-JSON stderr line", "line", line)
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.mu.Unlock()
		if ok {
			ch <- resp
			close(ch)
		}
	}
}

func (c *Client) sendRequest(method string, params any) (Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Response{}, fmt.Errorf("client closed")
	}
	c.msgID++
	id := c.msgID
	ch := make(chan Response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal: %w", err)
	}

	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	c.mu.Unlock()
	if err != nil {
		return Response{}, fmt.Errorf("write: %w", err)
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(60 * time.Second):
		slog.Error("mcp: request timed out", "method", method, "id", id)
		return Response{}, fmt.Errorf("MCP request timed out after 60s (method: %s, id: %d)", method, id)
	}
}

func (c *Client) Initialize() error {
	resp, err := c.sendRequest("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]string{
			"name":    "kusanagi",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize rejected: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	_, err = c.sendRequest("notifications/initialized", nil)
	if err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	return nil
}

func (c *Client) ListTools() ([]ToolDefinition, error) {
	resp, err := c.sendRequest("tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("list tools error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	if resp.Result == nil {
		return nil, fmt.Errorf("list tools: empty result")
	}
	var result struct {
		Tools []ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(*resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}
	if len(result.Tools) == 0 {
		return nil, fmt.Errorf("MCP server returned 0 tools")
	}
	c.tools = result.Tools
	return result.Tools, nil
}

func (c *Client) CallTool(name string, args map[string]any) (string, error) {
	resp, err := c.sendRequest("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", fmt.Errorf("call %s: %w", name, err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("tool %s error: code=%d msg=%s", name, resp.Error.Code, resp.Error.Message)
	}
	if resp.Result == nil {
		return "ok", nil
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(*resp.Result, &result); err != nil {
		return string(*resp.Result), nil
	}

	if result.IsError && len(result.Content) > 0 {
		return "", fmt.Errorf("MCP error: %s", result.Content[0].Text)
	}

	var texts []string
	for _, c := range result.Content {
		if c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	if len(texts) > 0 {
		return texts[0], nil
	}
	return "ok", nil
}

func (c *Client) Tools() []ToolDefinition {
	return c.tools
}

func (c *Client) ToGeminiDeclarations(exclude ...string) []map[string]any {
	excluded := make(map[string]bool)
	for _, name := range exclude {
		excluded[name] = true
	}
	var fds []map[string]any
	for _, t := range c.tools {
		if excluded[t.Name] {
			continue
		}
		fds = append(fds, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  sanitizeSchema(t.InputSchema),
		})
	}
	return []map[string]any{{
		"functionDeclarations": fds,
	}}
}

func sanitizeSchema(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok {
		return schema
	}
	result := make(map[string]any)
	for k, v := range m {
		switch k {
		case "type":
			if arr, ok := v.([]any); ok {
				for _, t := range arr {
					if t != "null" {
						v = t
						break
					}
				}
			}
			result[k] = v
		case "properties":
			props := make(map[string]any)
			if pm, ok := v.(map[string]any); ok {
				for pname, pval := range pm {
					props[pname] = sanitizeSchema(pval)
				}
			}
			result[k] = props
		case "items":
			if arr, ok := v.([]any); ok {
				sanitized := make([]any, len(arr))
				for i, item := range arr {
					sanitized[i] = sanitizeSchema(item)
				}
				result[k] = sanitized
			} else {
				result[k] = sanitizeSchema(v)
			}
		case "required", "description", "enum", "nullable", "default":
			result[k] = v
		}
	}
	if _, hasType := result["type"]; !hasType {
		result["type"] = "object"
	}
	return result
}

func (c *Client) StderrLog() string {
	return c.stderrBuf.String()
}

func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		// Try graceful shutdown via shutdown request
		if resp, err := c.sendRequest("shutdown", nil); err == nil && resp.Error == nil {
			c.sendRequest("notifications/exit", nil)
		}
		return c.cmd.Process.Kill()
	}
	return nil
}
