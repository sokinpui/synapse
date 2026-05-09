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

	var apiKeys []string
	normalized := strings.ReplaceAll(apiKeysVar, ",", "\n")
	rawKeys := strings.Split(normalized, "\n")

	for _, k := range rawKeys {
		apiKeys = append(apiKeys, strings.TrimSpace(k))
	}
	log.Printf("Gemini provider initialized with %d API keys", len(apiKeys))

	models := make(map[string]LLM)
	balancer := NewKeyBalancer(apiKeys)

	baseURL := cfg.Models.Gemini.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}

	endpointURL := buildChatEndpoint(baseURL)

	for _, code := range cfg.Models.Gemini.Codes {
		models[code] = NewOpenAIModel(code, endpointURL, balancer)
	}

	return models, nil
}
