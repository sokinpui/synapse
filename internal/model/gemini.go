package model

import (
	"log"
	"os"
	"strings"

	"github.com/sokinpui/synapse.go/internal/config"
)

func init() {
	RegisterProvider(newGeminiProvider)
}

func newGeminiProvider(cfg *config.Config) (map[string]LLM, error) {
	apiKeysVar := os.Getenv("GENAI_API_KEYS")
	rawKeys := strings.FieldsFunc(apiKeysVar, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == '\\'
	})

	var apiKeys []string
	for _, k := range rawKeys {
		if trimmed := strings.TrimSpace(k); trimmed != "" {
			apiKeys = append(apiKeys, trimmed)
		}
	}

	log.Printf("Gemini provider initialized with %d API keys", len(apiKeys))

	models := make(map[string]LLM)
	balancer := NewKeyBalancer(apiKeys)

	baseURL := cfg.Models.Gemini.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
	}

	for _, code := range cfg.Models.Gemini.Codes {
		models[code] = NewOpenAIModel(code, baseURL, balancer)
	}

	return models, nil
}
