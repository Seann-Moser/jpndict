package translate

import "net/http"

type DeepSeekConfig struct {
	APIKey     string
	Model      string
	Models     []string
	BaseURL    string
	HTTPClient *http.Client

	Temperature *float64
	MaxTokens   int
}

func NewDeepSeekClient(cfg DeepSeekConfig) *OpenAICompatibleClient {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}

	model := cfg.Model
	if model == "" {
		model = "deepseek-v4-flash"
	}

	return NewOpenAICompatibleClient(OpenAICompatibleConfig{
		Name:        "deepseek",
		BaseURL:     baseURL,
		APIKey:      cfg.APIKey,
		Model:       model,
		Models:      cfg.Models,
		HTTPClient:  cfg.HTTPClient,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
}
