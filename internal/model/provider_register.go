package model

import (
	"context"
	"fmt"
	"log"

	"github.com/sokinpui/synapse/internal/config"
)

type LLM interface {
	Generate(ctx context.Context, req *Request) (*Result, error)
	GenerateStream(ctx context.Context, req *Request) (<-chan *Result, <-chan error)
	CountTokens(prompt string) (int, error)
}

type Registry struct {
	models     map[string]LLM
	modelCodes []string
}

func New(cfg *config.Config) (*Registry, error) {
	allModels := make(map[string]LLM)

	for _, pCfg := range cfg.Providers {
		balancer := NewKeyBalancer(pCfg.APIKeys)

		for _, code := range pCfg.Codes {
			fullKey := fmt.Sprintf("%s/%s", pCfg.Name, code)
			if _, exists := allModels[fullKey]; exists {
				log.Printf("Warning: Duplicate model entry '%s' found in provider '%s'.", code, pCfg.Name)
			}
			allModels[fullKey] = NewOpenAIModel(fullKey, pCfg.BaseURL, balancer, cfg.Worker.MaxRetry)
		}
		log.Printf("Initialized provider '%s' with %d models and %d API keys", pCfg.Name, len(pCfg.Codes), len(pCfg.APIKeys))
	}

	ordered := cfg.GetOrderedModelCodes()
	var finalOrder []string
	for _, code := range ordered {
		if _, ok := allModels[code]; ok {
			finalOrder = append(finalOrder, code)
		}
	}

	if len(allModels) == 0 {
		log.Println("Warning: No models were loaded. Check your config and environment variables.")
	} else {
		log.Printf("Registry initialized with %d total models", len(allModels))
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
