package translate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GeminiConfig struct {
	APIKey     string
	Model      string
	Models     []string
	BaseURL    string
	HTTPClient *http.Client

	Temperature *float64
	MaxTokens   int
}

type GeminiClient struct {
	apiKey      string
	model       string
	models      []string
	baseURL     string
	httpClient  *http.Client
	temperature *float64
	maxTokens   int
}

func NewGeminiClient(cfg GeminiConfig) *GeminiClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: time.Duration(defaultHTTPTimeoutSeconds()) * time.Second,
		}
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}

	model := cfg.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	return &GeminiClient{
		apiKey:      cfg.APIKey,
		model:       model,
		models:      append([]string(nil), cfg.Models...),
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
	}
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`

	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (c *GeminiClient) Translate(ctx context.Context, r *Request) (*Response, error) {
	if err := validateRequest(r); err != nil {
		return nil, err
	}
	if !c.IsLanguageSupported(LanguageJapanese, r.ToLanguage) {
		return nil, ErrUnsupportedLanguage
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini api key is required")
	}

	parts := []geminiPart{
		{Text: BuildTranslationUserPrompt(r)},
	}

	if len(r.Image) > 0 {
		parts = append(parts, geminiPart{
			InlineData: &geminiInlineData{
				MimeType: "image/png",
				Data:     base64.StdEncoding.EncodeToString(r.Image),
			},
		})
	}

	payload := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{
				{Text: BuildTranslationSystemPrompt(r)},
			},
		},
		Contents: []geminiContent{
			{
				Role:  "user",
				Parts: parts,
			},
		},
	}

	if c.temperature != nil || c.maxTokens > 0 {
		payload.GenerationConfig = &geminiGenerationConfig{
			Temperature:     c.temperature,
			MaxOutputTokens: c.maxTokens,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

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

	var decoded geminiResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("gemini decode response: %w: %s", err, string(respBody))
	}

	if decoded.Error != nil {
		return nil, fmt.Errorf("gemini api error: %s", decoded.Error.Message)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if len(decoded.Candidates) == 0 {
		return nil, ErrNotFound
	}

	var out strings.Builder
	for _, part := range decoded.Candidates[0].Content.Parts {
		out.WriteString(part.Text)
	}

	text := cleanText(out.String())
	if text == "" {
		return nil, ErrNotFound
	}

	return &Response{
		Request: r,
		Text:    text,
	}, nil
}

func (c *GeminiClient) Search(ctx context.Context, r *Request) (*Response, error) {
	return nil, ErrNotFound
}

func (c *GeminiClient) Close() {}

func (c *GeminiClient) SupportedLanguage() []Language {
	return []Language{LanguageJapanese, LanguageEnglish}
}

func (c *GeminiClient) IsLanguageSupported(from Language, to Language) bool {
	return to == LanguageEnglish || to == LanguageJapanese
}

func (c *GeminiClient) SupportedModels() []string {
	if len(c.models) > 0 {
		return append([]string(nil), c.models...)
	}
	if c.model == "" {
		return nil
	}
	return []string{c.model}
}
