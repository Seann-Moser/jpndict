package translate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type OllamaConfig struct {
	BaseURL    string
	Model      string
	Models     []string
	HTTPClient *http.Client

	Managed     bool
	OllamaPath  string
	Host        string
	Port        string
	StartupWait time.Duration

	Temperature *float64
	MaxTokens   int
}

type OllamaClient struct {
	baseURL    string
	model      string
	models     []string
	httpClient *http.Client

	managed bool
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	mu      sync.Mutex

	ollamaPath  string
	host        string
	port        string
	startupWait time.Duration

	temperature *float64
	maxTokens   int

	modelMu      sync.Mutex
	modelEnsured bool
}

func NewOllamaClient(cfg OllamaConfig) *OllamaClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: time.Duration(defaultHTTPTimeoutSeconds()) * time.Second,
		}
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	model := cfg.Model
	if model == "" {
		model = "qwen2.5:7b"
	}

	return &OllamaClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		models:      append([]string(nil), cfg.Models...),
		httpClient:  httpClient,
		ollamaPath:  defaultString(cfg.OllamaPath, "ollama"),
		host:        defaultString(cfg.Host, "127.0.0.1"),
		port:        defaultString(cfg.Port, "11434"),
		startupWait: defaultDuration(cfg.StartupWait, 20*time.Second),
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
	}
}

func NewManagedOllamaClient(cfg OllamaConfig) *OllamaClient {
	cfg.Managed = true

	host := defaultString(cfg.Host, "127.0.0.1")
	port := defaultString(cfg.Port, "11434")

	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://" + net.JoinHostPort(host, port)
	}

	c := NewOllamaClient(cfg)
	c.managed = true
	c.host = host
	c.port = port
	return c
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`

	Error string `json:"error,omitempty"`
	Done  bool   `json:"done"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

type ollamaPullRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

type ollamaPullResponse struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (c *OllamaClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.managed {
		return nil
	}

	if c.cmd != nil && c.cmd.Process != nil {
		return nil
	}

	if err := c.ping(ctx); err == nil {
		return c.ensureModel(ctx)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	cmd := exec.CommandContext(runCtx, c.ollamaPath, "serve")
	cmd.Env = append(cmd.Environ(),
		"OLLAMA_HOST="+net.JoinHostPort(c.host, c.port),
	)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start ollama: %w", err)
	}

	c.cmd = cmd

	go drainPipe("ollama stdout", stdout)
	go drainPipe("ollama stderr", stderr)

	deadline := time.Now().Add(c.startupWait)
	for time.Now().Before(deadline) {
		if err := c.ping(ctx); err == nil {
			return c.ensureModel(ctx)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	return fmt.Errorf("ollama did not become ready at %s", c.baseURL)
}

func (c *OllamaClient) Translate(ctx context.Context, r *Request) (*Response, error) {
	if err := validateRequest(r); err != nil {
		return nil, err
	}
	if len(r.Image) > 0 {
		return nil, ErrImageUnsupported
	}
	if !c.IsLanguageSupported(LanguageJapanese, r.ToLanguage) {
		return nil, ErrUnsupportedLanguage
	}

	if c.managed {
		if err := c.Start(ctx); err != nil {
			return nil, err
		}
	}
	if err := c.ensureModel(ctx); err != nil {
		return nil, err
	}

	options := map[string]any{}
	if c.temperature != nil {
		options["temperature"] = *c.temperature
	}
	if c.maxTokens > 0 {
		options["num_predict"] = c.maxTokens
	}
	if len(options) == 0 {
		options = nil
	}

	payload := ollamaChatRequest{
		Model: c.model,
		Messages: []ollamaMessage{
			{
				Role:    "system",
				Content: BuildTranslationSystemPrompt(r),
			},
			{
				Role:    "user",
				Content: BuildTranslationUserPrompt(r),
			},
		},
		Stream:  false,
		Options: options,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := c.baseURL + "/api/chat"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded ollamaChatResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("ollama decode response: %w: %s", err, string(respBody))
	}

	if decoded.Error != "" {
		return nil, fmt.Errorf("ollama api error: %s", decoded.Error)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	text := cleanText(decoded.Message.Content)
	if text == "" {
		return nil, ErrNotFound
	}

	return &Response{
		Request: r,
		Text:    text,
	}, nil
}

func (c *OllamaClient) Search(ctx context.Context, r *Request) (*Response, error) {
	return nil, ErrNotFound
}

func (c *OllamaClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}

	c.cmd = nil
}

func (c *OllamaClient) SupportedLanguage() []Language {
	return []Language{LanguageJapanese, LanguageEnglish}
}

func (c *OllamaClient) IsLanguageSupported(from Language, to Language) bool {
	return to == LanguageEnglish || to == LanguageJapanese
}

func (c *OllamaClient) SupportedModels() []string {
	if len(c.models) > 0 {
		return append([]string(nil), c.models...)
	}
	if c.model == "" {
		return nil
	}
	return []string{c.model}
}

func (c *OllamaClient) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama ping status %d", resp.StatusCode)
	}

	return nil
}

func (c *OllamaClient) ensureModel(ctx context.Context) error {
	c.modelMu.Lock()
	defer c.modelMu.Unlock()

	if c.modelEnsured {
		return nil
	}

	exists, err := c.hasModel(ctx)
	if err != nil {
		return err
	}
	if !exists {
		if err := c.pullModel(ctx); err != nil {
			return err
		}
	}

	c.modelEnsured = true
	return nil
}

func (c *OllamaClient) hasModel(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("ollama tags status %d: %s", resp.StatusCode, string(respBody))
	}

	var decoded ollamaTagsResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return false, fmt.Errorf("ollama decode tags response: %w: %s", err, string(respBody))
	}

	for _, model := range decoded.Models {
		if model.Name == c.model || model.Model == c.model {
			return true, nil
		}
	}

	return false, nil
}

func (c *OllamaClient) pullModel(ctx context.Context) error {
	payload := ollamaPullRequest{
		Model:  c.model,
		Stream: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var decoded ollamaPullResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &decoded); err != nil {
			return fmt.Errorf("ollama decode pull response: %w: %s", err, string(respBody))
		}
	}

	if decoded.Error != "" {
		return fmt.Errorf("ollama pull error: %s", decoded.Error)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama pull status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func drainPipe(name string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		_ = scanner.Text()
	}
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func defaultDuration(v time.Duration, fallback time.Duration) time.Duration {
	if v == 0 {
		return fallback
	}
	return v
}
