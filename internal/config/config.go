package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		HTTPPort int `yaml:"http_port"`
	} `yaml:"server"`
	Worker struct {
		ConcurrencyMultiplier int `yaml:"concurrency_multiplier"`
	} `yaml:"worker"`
	Models struct {
		Gemini     ProviderConfig `yaml:"gemini"`
		OpenRouter ProviderConfig `yaml:"openrouter"`
	} `yaml:"models"`
}

type ProviderConfig struct {
	BaseURL string   `yaml:"base_url"`
	Codes []string `yaml:"codes"`
}

// Load reads configuration from the YAML file.
func Load(path string) *Config {
	if path == "" {
		path = "config.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read config file at %s: %v", path, err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatalf("failed to unmarshal config: %v", err)
	}

	return &cfg
}

func (c *Config) GetOrderedModelCodes() []string {
	var codes []string
	codes = append(codes, c.Models.Gemini.Codes...)
	codes = append(codes, c.Models.OpenRouter.Codes...)
	return codes
}
