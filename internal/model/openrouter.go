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

	rawKeys := strings.FieldsFunc(apiKeysVar, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == '\\'
	})

	var apiKeys []string
	for _, k := range rawKeys {
		if trimmed := strings.TrimSpace(k); trimmed != "" {
			apiKeys = append(apiKeys, trimmed)
		}
	}

	log.Printf("OpenRouter provider initialized with %d API keys", len(apiKeys))

	models := make(map[string]LLM)
	balancer := NewKeyBalancer(apiKeys)

	baseURL := cfg.Models.OpenRouter.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1/chat/completions"
	} else if !strings.HasSuffix(baseURL, "/chat/completions") {
		baseURL = strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	}

	for _, code := range cfg.Models.OpenRouter.Codes {
		models[code] = NewOpenAIModel(code, baseURL, balancer)
	}
	return models, nil
}
