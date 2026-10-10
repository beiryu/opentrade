package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHTTPClientCompleteAndRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("missing bearer")
		}
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
			t.Errorf("JSON mode not requested")
		}
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "deepseek/x",
			"choices": []map[string]any{{"message": map[string]string{"content": `{"decisions":[]}`}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 3},
		})
	}))
	defer srv.Close()
	c, err := New(Options{BaseURL: srv.URL, APIKey: "k", Model: "m", JSONMode: true, Retries: 1})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != `{"decisions":[]}` || res.Model != "deepseek/x" || res.PromptTokens != 10 {
		t.Fatalf("result = %+v", res)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
}

func TestHTTPClientNoRetryOn4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key","type":"auth"}}`))
	}))
	defer srv.Close()
	c, _ := New(Options{BaseURL: srv.URL, Model: "m", Retries: 3})
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("4xx must not retry, got %d calls", calls)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                               `{"a":1}`,
		"Here you go:\n```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":{\"b\":2}}\n```\nthanks":   `{"a":{"b":2}}`,
		"no json here":                          "no json here",
	}
	for in, want := range cases {
		if got := ExtractJSON(in); got != want {
			t.Errorf("ExtractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFake(t *testing.T) {
	f := &Fake{}
	res, err := f.Complete(context.Background(), "s", "u")
	if err != nil || res.Text != `{"decisions":[]}` || f.Calls != 1 {
		t.Fatalf("fake default = %+v %v", res, err)
	}
}
