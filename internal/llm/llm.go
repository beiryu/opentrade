// Package llm is the thin client for any OpenAI-compatible chat endpoint.
// OpenTrade calls the Vercel AI Gateway by default, but DeepSeek, OpenAI,
// Ollama or LM Studio work by changing the base URL. No provider SDK.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Result is one completion.
type Result struct {
	Text             string
	Model            string
	LatencyMs        int64
	PromptTokens     int
	CompletionTokens int
}

// Client produces a completion for a system + user prompt.
type Client interface {
	Complete(ctx context.Context, system, user string) (Result, error)
	// Name identifies the client in records ("deepseek/…", "fake").
	Name() string
}

// Options for the HTTP client.
type Options struct {
	BaseURL string
	APIKey  string
	Model   string
	// JSONMode asks the server for a JSON object response (response_format).
	JSONMode    bool
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
	// Retries on transient failures (5xx, 429, network), with a short backoff.
	Retries int
	// HTTPClient to use; http.DefaultClient when nil.
	HTTPClient *http.Client
}

// HTTPClient talks to /chat/completions.
type HTTPClient struct {
	opt Options
}

// New returns an HTTPClient. The API key is required unless the base URL is
// a local server (Ollama-style) that ignores it.
func New(opt Options) (*HTTPClient, error) {
	if opt.BaseURL == "" {
		return nil, errors.New("llm: base URL is required")
	}
	if opt.Model == "" {
		return nil, errors.New("llm: model is required")
	}
	if opt.Timeout == 0 {
		opt.Timeout = 60 * time.Second
	}
	if opt.MaxTokens == 0 {
		opt.MaxTokens = 2000
	}
	if opt.HTTPClient == nil {
		opt.HTTPClient = &http.Client{Timeout: opt.Timeout}
	}
	opt.BaseURL = strings.TrimRight(opt.BaseURL, "/")
	return &HTTPClient{opt: opt}, nil
}

// Name implements Client.
func (c *HTTPClient) Name() string { return c.opt.Model }

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Complete implements Client.
func (c *HTTPClient) Complete(ctx context.Context, system, user string) (Result, error) {
	body := chatRequest{
		Model: c.opt.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: c.opt.Temperature,
		MaxTokens:   c.opt.MaxTokens,
	}
	if c.opt.JSONMode {
		body.ResponseFormat = &respFormat{Type: "json_object"}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	var lastErr error
	for attempt := 0; attempt <= c.opt.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		res, retry, err := c.once(ctx, payload)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return Result{}, lastErr
}

func (c *HTTPClient) once(ctx context.Context, payload []byte) (Result, bool, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opt.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.opt.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.opt.APIKey)
	}
	resp, err := c.opt.HTTPClient.Do(req)
	if err != nil {
		return Result{}, true, fmt.Errorf("llm: request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Result{}, true, fmt.Errorf("llm: read: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return Result{}, true, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, trim(raw))
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, false, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, trim(raw))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return Result{}, false, fmt.Errorf("llm: bad JSON from server: %w", err)
	}
	if cr.Error != nil {
		return Result{}, false, fmt.Errorf("llm: %s: %s", cr.Error.Type, cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return Result{}, false, errors.New("llm: no choices in response")
	}
	model := cr.Model
	if model == "" {
		model = c.opt.Model
	}
	return Result{
		Text:             cr.Choices[0].Message.Content,
		Model:            model,
		LatencyMs:        time.Since(start).Milliseconds(),
		PromptTokens:     cr.Usage.PromptTokens,
		CompletionTokens: cr.Usage.CompletionTokens,
	}, false, nil
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// Fake is a Client for tests and offline runs: it returns whatever Reply
// produces for the user prompt.
type Fake struct {
	Reply func(system, user string) (string, error)
	Calls int
}

// Name implements Client.
func (f *Fake) Name() string { return "fake" }

// Complete implements Client.
func (f *Fake) Complete(_ context.Context, system, user string) (Result, error) {
	f.Calls++
	if f.Reply == nil {
		return Result{Text: `{"decisions":[]}`, Model: "fake"}, nil
	}
	text, err := f.Reply(system, user)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Model: "fake"}, nil
}

// ExtractJSON finds the first JSON object in a model reply, tolerating
// ```json fences and prose around it. Returns the raw text unchanged when
// no object is found so the parser can report the real problem.
func ExtractJSON(text string) string {
	s := strings.TrimSpace(text)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		if j := strings.Index(rest, "\n"); j >= 0 && len(strings.TrimSpace(rest[:j])) <= 8 {
			rest = rest[j+1:]
		}
		if k := strings.Index(rest, "```"); k >= 0 {
			rest = rest[:k]
		}
		s = strings.TrimSpace(rest)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return text
}
