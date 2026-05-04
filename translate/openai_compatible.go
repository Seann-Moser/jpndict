package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	Name       string
	BaseURL    string
	APIKey     string
	Model      string
	Models     []string
	HTTPClient *http.Client

	Temperature *float64
	MaxTokens   int
}

type OpenAICompatibleClient struct {
	name       string
	baseURL    string
	apiKey     string
	model      string
	models     []string
	httpClient *http.Client

	temperature *float64
	maxTokens   int
}

func NewOpenAICompatibleClient(cfg OpenAICompatibleConfig) *OpenAICompatibleClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: time.Duration(defaultHTTPTimeoutSeconds()) * time.Second,
		}
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = "openai-compatible"
	}

	return &OpenAICompatibleClient{
		name:        name,
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		models:      append([]string(nil), cfg.Models...),
		httpClient:  httpClient,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
	}
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`

	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompatibleClient) Translate(ctx context.Context, r *Request) (*Response, error) {
	if err := validateRequest(r); err != nil {
		return nil, err
	}
	if len(r.Image) > 0 {
		return nil, ErrImageUnsupported
	}
	if !c.IsLanguageSupported(LanguageJapanese, r.ToLanguage) {
		return nil, ErrUnsupportedLanguage
	}
	if c.baseURL == "" {
		return nil, fmt.Errorf("%s base URL is required", c.name)
	}
	if c.model == "" {
		return nil, fmt.Errorf("%s model is required", c.name)
	}

	payload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: BuildTranslationSystemPrompt(r),
			},
			{
				Role:    "user",
				Content: BuildTranslationUserPrompt(r),
			},
		},
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := c.baseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("%s decode response: %w: %s", c.name, err, string(respBody))
	}

	if decoded.Error != nil {
		return nil, fmt.Errorf("%s api error: %s", c.name, decoded.Error.Message)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned status %d: %s", c.name, resp.StatusCode, string(respBody))
	}

	if len(decoded.Choices) == 0 {
		return nil, ErrNotFound
	}

	text := cleanText(decoded.Choices[0].Message.Content)
	if text == "" {
		return nil, ErrNotFound
	}

	return &Response{
		Request: r,
		Text:    text,
	}, nil
}

func (c *OpenAICompatibleClient) Search(ctx context.Context, r *Request) (*Response, error) {
	return nil, ErrNotFound
}

func (c *OpenAICompatibleClient) Close() {}

func (c *OpenAICompatibleClient) SupportedLanguage() []Language {
	return []Language{LanguageJapanese, LanguageEnglish}
}

func (c *OpenAICompatibleClient) IsLanguageSupported(from Language, to Language) bool {
	return to == LanguageEnglish || to == LanguageJapanese
}

func (c *OpenAICompatibleClient) SupportedModels() []string {
	if len(c.models) > 0 {
		return append([]string(nil), c.models...)
	}
	if c.model == "" {
		return nil
	}
	return []string{c.model}
}
