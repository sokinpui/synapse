package model

import (
	"context"
	"fmt"

	"github.com/sokinpui/synapse.go/internal/config"
)

type LLM interface {
	Generate(ctx context.Context, prompt string, images [][]byte, config *Config) (string, error)
	GenerateStream(ctx context.Context, prompt string, images [][]byte, config *Config) (<-chan string, <-chan error)
	CountTokens(prompt string) (int, error)
}

type ModelProvider func(cfg *config.Config) (map[string]LLM, error)

var providers []ModelProvider

func RegisterProvider(provider ModelProvider) {
	providers = append(providers, provider)
}

type Registry struct {
	models map[string]LLM
}

func New(cfg *config.Config) (*Registry, error) {
	allModels := make(map[string]LLM)
	for _, provider := range providers {
		providerModels, err := provider(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize a model provider: %w", err)
		}
		for name, model := range providerModels {
			if _, exists := allModels[name]; exists {
				// Handle potential model name collisions
				fmt.Printf("Warning: Model '%s' is being overwritten by a new provider.\n", name)
			}
			allModels[name] = model
		}
	}

	return &Registry{models: allModels}, nil
}

func (r *Registry) GetModel(modelCode string) (LLM, error) {
	model, ok := r.models[modelCode]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrModelNotFound, modelCode)
	}
	return model, nil
}

func (r *Registry) ListModels() []string {
	keys := make([]string, 0, len(r.models))
	for k := range r.models {
		keys = append(keys, k)
	}
	return keys
}
