package translate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOllamaTranslatePullsMissingModel(t *testing.T) {
	var tagsCalls int32
	var pullCalls int32
	var chatCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			atomic.AddInt32(&tagsCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/pull":
			atomic.AddInt32(&pullCalls, 1)

			var req ollamaPullRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode pull request: %v", err)
			}
			if req.Model != "qwen2.5:7b" {
				t.Fatalf("pull model = %q, want %q", req.Model, "qwen2.5:7b")
			}
			if req.Stream {
				t.Fatal("pull request should disable streaming")
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case "/api/chat":
			atomic.AddInt32(&chatCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"hello"},"done":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(OllamaConfig{
		BaseURL: server.URL,
		Model:   "qwen2.5:7b",
	})

	resp, err := client.Translate(context.Background(), &Request{
		Text:       "こんにちは",
		ToLanguage: LanguageEnglish,
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("translated text = %q, want %q", resp.Text, "hello")
	}
	if got := atomic.LoadInt32(&tagsCalls); got != 1 {
		t.Fatalf("tags calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&pullCalls); got != 1 {
		t.Fatalf("pull calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&chatCalls); got != 1 {
		t.Fatalf("chat calls = %d, want 1", got)
	}
}

func TestOllamaTranslateSkipsPullWhenModelExists(t *testing.T) {
	var pullCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5:7b","model":"qwen2.5:7b"}]}`))
		case "/api/pull":
			atomic.AddInt32(&pullCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case "/api/chat":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"hello"},"done":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(OllamaConfig{
		BaseURL: server.URL,
		Model:   "qwen2.5:7b",
	})

	_, err := client.Translate(context.Background(), &Request{
		Text:       "こんにちは",
		ToLanguage: LanguageEnglish,
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if got := atomic.LoadInt32(&pullCalls); got != 0 {
		t.Fatalf("pull calls = %d, want 0", got)
	}
}
