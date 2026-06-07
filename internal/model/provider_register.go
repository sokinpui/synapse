package model

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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

	for name, pCfg := range cfg.Models {
		apiKeys := parseAPIKeysFromEnv(pCfg.Env)
		balancer := NewKeyBalancer(apiKeys)

		for _, code := range pCfg.Codes {
			if _, exists := allModels[code]; exists {
				log.Printf("Warning: Model '%s' from provider '%s' is overwriting an existing model.", code, name)
			}
			allModels[code] = NewOpenAIModel(code, pCfg.BaseURL, balancer, cfg.Worker.MaxRetry)
		}
		log.Printf("Initialized provider '%s' with %d models and %d API keys", name, len(pCfg.Codes), len(apiKeys))
	}

	ordered := cfg.GetOrderedModelCodes()
	var finalOrder []string
	for _, code := range ordered {
		if _, ok := allModels[code]; ok {
			finalOrder = append(finalOrder, code)
		}
	}

	if len(allModels) == 0 {
		log.Println("Warning: No models were loaded. Check your config.yaml and environment variables.")
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

func parseAPIKeysFromEnv(envVar string) []string {
	if envVar == "" {
		return []string{""}
	}

	val := os.Getenv(envVar)
	if val == "" {
		return []string{""}
	}

	// Feature: handle comma or newline as separators, preserving empty segments as empty strings
	normalized := strings.ReplaceAll(val, ",", "\n")
	rawKeys := strings.Split(normalized, "\n")

	var keys []string
	for _, k := range rawKeys {
		keys = append(keys, strings.TrimSpace(k))
	}
	return keys
}
