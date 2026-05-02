package translate

import "net/http"

type OpenAIConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client

	Temperature *float64
	MaxTokens   int
}

func NewOpenAIClient(cfg OpenAIConfig) *OpenAICompatibleClient {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4.1-mini"
	}

	return NewOpenAICompatibleClient(OpenAICompatibleConfig{
		Name:        "openai",
		BaseURL:     baseURL,
		APIKey:      cfg.APIKey,
		Model:       model,
		HTTPClient:  cfg.HTTPClient,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
}
