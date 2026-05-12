package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/sokinpui/synapse.go/internal/config"
)

type LLM interface {
	Generate(ctx context.Context, req *Request) (*Result, error)
	GenerateStream(ctx context.Context, req *Request) (<-chan *Result, <-chan error)
	CountTokens(prompt string) (int, error)
}

type ModelProvider func(cfg *config.Config) (map[string]LLM, error)

var providers []ModelProvider

func RegisterProvider(provider ModelProvider) {
	providers = append(providers, provider)
}

type Registry struct {
	models     map[string]LLM
	modelCodes []string
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

	ordered := cfg.GetOrderedModelCodes()
	var finalOrder []string
	for _, code := range ordered {
		if _, ok := allModels[code]; ok {
			finalOrder = append(finalOrder, code)
		}
	}

	return &Registry{models: allModels, modelCodes: finalOrder}, nil
}

func (r *Registry) GetModel(modelCode string) (LLM, error) {
	model, ok := r.models[modelCode]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrModelNotFound, modelCode)
	}
	return model, nil
}

func (r *Registry) ListModels() []string {
	return r.modelCodes
}

func buildChatEndpoint(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/chat/completions"
}
