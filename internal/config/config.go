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
	Models map[string]ProviderConfig `yaml:"models"`
}

type ProviderConfig struct {
	BaseURL string   `yaml:"base_url"`
	Env     string   `yaml:"env"`
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
	for _, provider := range c.Models {
		codes = append(codes, provider.Codes...)
	}
	return codes
}
