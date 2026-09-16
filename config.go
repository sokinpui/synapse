package main

import (
	"os"
	"strings"

	"github.com/sokinpui/synapse/adapter"
)

type ServerConfig struct {
	HTTPPort int
}

type WorkerConfig struct {
	ConcurrencyMultiplier int
	MaxRetry              int
}

type ProviderConfig struct {
	Name    string
	BaseURL string
	Adapter adapter.ProviderAdapter
	APIKeys []string
	Codes   []string
}

type Config struct {
	Server    ServerConfig
	Worker    WorkerConfig
	Providers []ProviderConfig
}

func LoadConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort: 9001,
		},
		Worker: WorkerConfig{
			ConcurrencyMultiplier: 4,
			MaxRetry:              29,
		},
		Providers: []ProviderConfig{
			{
				Name:    "aisrp",
				BaseURL: "http://localhost:9003/v1",
				Adapter: adapter.TransparentAdapter(),
				APIKeys: ParseAPIKeys(os.Getenv("AISRP_API_KEYS")),
				Codes: []string{
					"gemini-flash-latest",
					"gemini-flash-lite-latest",
					"gemini-pro-latest",
					"gemini-3.8-flash",
					"gemini-3.7-flash",
					"gemini-3.5-flash-lite",
					"gemini-3.1-flash-lite",
					"gemini-3.1-pro-preview",
					"gemma-4-26b-a4b-it",
					"gemma-4-31b-it",
					"gemini-robotics-er-2-preview",
					"gemini-robotics-er-2-streaming-preview",
					"gemini-3.5-transcribe",
					"gemini-3.5-transcribe-live",
					"gemini-3.5-live-translate-preview",
					"gemini-3.1-flash-live-preview",
					"gemini-3.1-flash-tts-preview",
					"gemini-2.5-flash-preview-tts",
					"gemini-2.5-pro-preview-tts",
				},
			},
			{
				Name:    "glm",
				BaseURL: "https://open.bigmodel.cn/api/paas/v4",
				APIKeys: ParseAPIKeys(os.Getenv("GLM_API_KEY")),
				Adapter: adapter.GLMAdapter(),
				Codes: []string{
					"glm-4-flash",
					"glm-4-flash-250414",
					"glm-4.7-flash",
					"glm-z1-flash",
				},
			},
			{
				Name:    "aisrp-image",
				BaseURL: "http://localhost:9003/v1",
				Adapter: adapter.AISRPImageAdapter(),
				APIKeys: ParseAPIKeys(os.Getenv("AISRP_API_KEYS")),
				Codes: []string{
					"gemini-3.1-flash-lite-image",
				},
			},
		},
	}
}

func (c *Config) GetOrderedModelCodes() []string {
	var codes []string
	for _, provider := range c.Providers {
		for _, code := range provider.Codes {
			codes = append(codes, provider.Name+"/"+code)
		}
	}
	return codes
}

func ParseAPIKeys(val string) []string {
	if val == "" {
		return []string{""}
	}

	normalized := strings.ReplaceAll(val, ",", "\n")
	rawKeys := strings.Split(normalized, "\n")

	var keys []string
	for _, k := range rawKeys {
		trimmed := strings.TrimSpace(k)
		if trimmed != "" {
			keys = append(keys, trimmed)
		}
	}

	if len(keys) == 0 {
		return []string{""}
	}
	return keys
}

func getEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
