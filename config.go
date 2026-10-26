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

type ModelSpec struct {
	Code       string   `json:"code"`
	InputType  []string `json:"input_type"`
	OutputType []string `json:"output_type"`
}

type ProviderConfig struct {
	Name    string
	BaseURL string
	Adapter adapter.ProviderAdapter
	APIKeys []string
	Models  []ModelSpec
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
				Models: []ModelSpec{
					{Code: "gemini-flash-latest-search", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-flash-lite-latest-search", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-pro-latest-search", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-flash-latest", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-flash-lite-latest", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-pro-latest", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-3.8-flash", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-3.7-flash", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-3.5-flash-lite", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-3.1-flash-lite", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-3.1-pro-preview", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemma-4-26b-a4b-it", InputType: []string{"text"}, OutputType: []string{"text"}},
					{Code: "gemma-4-31b-it", InputType: []string{"text"}, OutputType: []string{"text"}},
					{Code: "gemini-robotics-er-2-preview", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-robotics-er-2-streaming-preview", InputType: []string{"text", "image"}, OutputType: []string{"text"}},
					{Code: "gemini-3.5-transcribe", InputType: []string{"audio"}, OutputType: []string{"text"}},
					{Code: "gemini-3.5-transcribe-live", InputType: []string{"audio"}, OutputType: []string{"text"}},
					{Code: "gemini-3.5-live-translate-preview", InputType: []string{"text", "audio"}, OutputType: []string{"text"}},
					{Code: "gemini-3.1-flash-live-preview", InputType: []string{"text", "image", "audio"}, OutputType: []string{"text", "audio"}},
					{Code: "gemini-3.1-flash-tts-preview", InputType: []string{"text"}, OutputType: []string{"audio"}},
					{Code: "gemini-2.5-flash-preview-tts", InputType: []string{"text"}, OutputType: []string{"audio"}},
					{Code: "gemini-2.5-pro-preview-tts", InputType: []string{"text"}, OutputType: []string{"audio"}},
				},
			},
			{
				Name:    "glm",
				BaseURL: "https://open.bigmodel.cn/api/paas/v4",
				APIKeys: ParseAPIKeys(os.Getenv("GLM_API_KEY")),
				Adapter: adapter.GLMAdapter(),
				Models: []ModelSpec{
					{Code: "glm-4-flash", InputType: []string{"text"}, OutputType: []string{"text"}},
					{Code: "glm-4-flash-250414", InputType: []string{"text"}, OutputType: []string{"text"}},
					{Code: "glm-4.7-flash", InputType: []string{"text"}, OutputType: []string{"text"}},
					{Code: "glm-z1-flash", InputType: []string{"text"}, OutputType: []string{"text"}},
				},
			},
			{
				Name:    "aisrp-image",
				BaseURL: "http://localhost:9003/v1",
				Adapter: adapter.AISRPImageAdapter(),
				APIKeys: ParseAPIKeys(os.Getenv("AISRP_API_KEYS")),
				Models: []ModelSpec{
					{Code: "gemini-3.1-flash-lite-image", InputType: []string{"text"}, OutputType: []string{"image"}},
				},
			},
		},
	}
}

func (c *Config) GetOrderedModelCodes() []string {
	var codes []string
	for _, provider := range c.Providers {
		for _, spec := range provider.Models {
			codes = append(codes, provider.Name+"/"+spec.Code)
		}
	}
	return codes
}

func (c *Config) GetOrderedModelSpecs() []ModelSpec {
	var specs []ModelSpec
	for _, provider := range c.Providers {
		for _, spec := range provider.Models {
			inType := spec.InputType
			if len(inType) == 0 {
				inType = []string{"text"}
			}
			outType := spec.OutputType
			if len(outType) == 0 {
				outType = []string{"text"}
			}
			specs = append(specs, ModelSpec{
				Code:       provider.Name + "/" + spec.Code,
				InputType:  inType,
				OutputType: outType,
			})
		}
	}
	return specs
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
