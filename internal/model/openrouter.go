package model

import (
	"log"
	"os"
	"strings"

	"github.com/sokinpui/synapse.go/internal/config"
)

func init() {
	RegisterProvider(newOpenRouterProvider)
}

func newOpenRouterProvider(cfg *config.Config) (map[string]LLM, error) {
	apiKeysVar := os.Getenv("OPENROUTER_API_KEYS")
	if apiKeysVar == "" {
		apiKeysVar = os.Getenv("OPENROUTER_API_KEY")
	}

	var apiKeys []string
	normalized := strings.ReplaceAll(apiKeysVar, ",", "\n")
	rawKeys := strings.Split(normalized, "\n")

	for _, k := range rawKeys {
		apiKeys = append(apiKeys, strings.TrimSpace(k))
	}
	log.Printf("OpenRouter provider initialized with %d API keys", len(apiKeys))

	models := make(map[string]LLM)
	balancer := NewKeyBalancer(apiKeys)

	baseURL := cfg.Models.OpenRouter.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	endpointURL := buildChatEndpoint(baseURL)

	for _, code := range cfg.Models.OpenRouter.Codes {
		models[code] = NewOpenAIModel(code, endpointURL, balancer)
	}
	return models, nil
}
