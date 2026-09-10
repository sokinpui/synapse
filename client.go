package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
)

type LLM interface {
	Generate(ctx context.Context, task *GenerationTask) (*Result, error)
	GenerateStream(ctx context.Context, task *GenerationTask) (<-chan *Result, <-chan error)
	CountTokens(prompt string) (int, error)
}

type ProviderModel struct {
	provider  string
	modelCode string
	baseURL   string
	adapter   ProviderAdapter
	balancer  *KeyBalancer
	maxRetry  int
	client    *http.Client
}

func NewProviderModel(provider, modelCode, baseURL string, adapter ProviderAdapter, balancer *KeyBalancer, maxRetry int) *ProviderModel {
	return &ProviderModel{
		provider:  provider,
		modelCode: modelCode,
		baseURL:   baseURL,
		adapter:   adapter,
		balancer:  balancer,
		maxRetry:  maxRetry,
		client:    &http.Client{},
	}
}

func (m *ProviderModel) Generate(ctx context.Context, task *GenerationTask) (*Result, error) {
	if m.balancer.KeyCount() == 0 {
		return nil, fmt.Errorf("%w: API key is required", ErrConfiguration)
	}

	reqCtx := &RequestContext{
		BaseURL:   m.baseURL,
		Endpoint:  task.Endpoint,
		ModelCode: m.modelCode,
		Stream:    false,
		Payload:   task.Payload,
	}
	maxAttempts := m.maxRetry + 1
	var lastErr error

	for i := 0; i < maxAttempts && i < m.balancer.KeyCount(); i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		apiKey, keyIdx := m.balancer.PickKey()
		reqCtx.APIKey = apiKey

		log.Printf("-> %s: %s [%s] -> %s, try API key #%d",
			blueString("Processing request"),
			yellowString(task.TaskID),
			m.provider+"/"+m.modelCode, task.Endpoint, keyIdx)

		httpReq, err := m.adapter.BuildRequest(ctx, reqCtx)
		if err != nil {
			return nil, err
		}

		resp, err := m.client.Do(httpReq)
		if err != nil {
			lastErr = err
			log.Printf("!! %s: %s [key #%d] network error: %v", yellowString("Attempt failed"), task.TaskID, keyIdx, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("status code: %d, body: %s", resp.StatusCode, string(body))
			log.Printf("!! %s: %s [key #%d] provider error: %v", yellowString("Attempt failed"), task.TaskID, keyIdx, lastErr)
			continue
		}

		raw, err := m.adapter.TransformResponse(resp)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return &Result{Raw: raw}, nil
	}

	return nil, fmt.Errorf("all API keys failed: %w", lastErr)
}

func (m *ProviderModel) GenerateStream(ctx context.Context, task *GenerationTask) (<-chan *Result, <-chan error) {
	outCh := make(chan *Result)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if m.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required", ErrConfiguration)
			return
		}

		reqCtx := &RequestContext{
			BaseURL:   m.baseURL,
			Endpoint:  task.Endpoint,
			ModelCode: m.modelCode,
			Stream:    true,
			Payload:   task.Payload,
		}
		maxAttempts := m.maxRetry + 1
		var lastErr error

		for i := 0; i < maxAttempts && i < m.balancer.KeyCount(); i++ {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			apiKey, keyIdx := m.balancer.PickKey()
			reqCtx.APIKey = apiKey

			log.Printf("-> %s: %s [%s] -> %s, try API key #%d",
				blueString("Processing request"),
				yellowString(task.TaskID),
				m.provider+"/"+m.modelCode, task.Endpoint, keyIdx)

			httpReq, err := m.adapter.BuildRequest(ctx, reqCtx)
			if err != nil {
				errCh <- err
				return
			}

			resp, err := m.client.Do(httpReq)
			if err != nil {
				lastErr = err
				log.Printf("!! %s: %s [key #%d] network error: %v", yellowString("Attempt failed"), task.TaskID, keyIdx, err)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				lastErr = fmt.Errorf("status code: %d, body: %s", resp.StatusCode, string(body))
				log.Printf("!! %s: %s [key #%d] provider error: %v", yellowString("Attempt failed"), task.TaskID, keyIdx, lastErr)
				continue
			}

			err = m.adapter.TransformStream(ctx, resp, outCh)
			if err != nil {
				errCh <- err
			}
			resp.Body.Close()
			return
		}
		errCh <- fmt.Errorf("all API keys failed: %w", lastErr)
	}()

	return outCh, errCh
}

func (m *ProviderModel) CountTokens(prompt string) (int, error) {
	return len(prompt) / 4, nil
}

type Registry struct {
	models     map[string]LLM
	modelCodes []string
}

func NewRegistry(cfg *Config) (*Registry, error) {
	allModels := make(map[string]LLM)

	for _, pCfg := range cfg.Providers {
		adapter := pCfg.Adapter
		if adapter == nil {
			adapter = TransparentAdapter()
		}

		balancer := NewKeyBalancer(pCfg.APIKeys)

		for _, code := range pCfg.Codes {
			fullKey := fmt.Sprintf("%s/%s", pCfg.Name, code)
			if _, exists := allModels[fullKey]; exists {
				log.Printf("Warning: Duplicate model entry '%s' found in provider '%s'.", code, pCfg.Name)
			}
			allModels[fullKey] = NewProviderModel(pCfg.Name, code, pCfg.BaseURL, adapter, balancer, cfg.Worker.MaxRetry)
		}
		log.Printf("Initialized provider '%s' (adapter: %s) with %d models and %d API keys", pCfg.Name, adapter.Name(), len(pCfg.Codes), len(pCfg.APIKeys))
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
