package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type LLM interface {
	Generate(ctx context.Context, task *GenerationTask) (*Result, error)
	GenerateStream(ctx context.Context, task *GenerationTask) (<-chan *Result, <-chan error)
	CountTokens(prompt string) (int, error)
}

type OpenAIModel struct {
	modelCode string
	baseURL   string
	balancer  *KeyBalancer
	maxRetry  int
	client    *http.Client
}

func NewOpenAIModel(modelCode, baseURL string, balancer *KeyBalancer, maxRetry int) *OpenAIModel {
	return &OpenAIModel{
		modelCode: modelCode,
		baseURL:   baseURL,
		balancer:  balancer,
		maxRetry:  maxRetry,
		client:    &http.Client{},
	}
}

func (m *OpenAIModel) Generate(ctx context.Context, task *GenerationTask) (*Result, error) {
	if m.balancer.KeyCount() == 0 {
		return nil, fmt.Errorf("%w: API key is required", ErrConfiguration)
	}

	targetURL := m.buildURL(task.Endpoint)
	maxAttempts := m.maxRetry + 1
	var lastErr error

	for i := 0; i < maxAttempts && i < m.balancer.KeyCount(); i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		apiKey, keyIdx := m.balancer.PickKey()
		log.Printf("-> %s: %s [%s] -> %s, try API key #%d",
			blueString("Processing request"),
			yellowString(task.TaskID),
			m.modelCode, task.Endpoint, keyIdx)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(task.Payload))
		if err != nil {
			return nil, err
		}

		m.setHeaders(httpReq, apiKey)
		resp, err := m.client.Do(httpReq)
		if err != nil {
			lastErr = err
			log.Printf("!! %s: %s [key #%d] network error: %v", yellowString("Attempt failed"), task.TaskID, keyIdx, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("status code: %d, body: %s", resp.StatusCode, string(body))
			log.Printf("!! %s: %s [key #%d] provider error: %v", yellowString("Attempt failed"), task.TaskID, keyIdx, lastErr)
			continue
		}

		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}
		return &Result{Raw: raw}, nil
	}

	return nil, fmt.Errorf("all API keys failed: %w", lastErr)
}

func (m *OpenAIModel) GenerateStream(ctx context.Context, task *GenerationTask) (<-chan *Result, <-chan error) {
	outCh := make(chan *Result)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if m.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required", ErrConfiguration)
			return
		}

		targetURL := m.buildURL(task.Endpoint)
		maxAttempts := m.maxRetry + 1
		var lastErr error

		for i := 0; i < maxAttempts && i < m.balancer.KeyCount(); i++ {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			apiKey, keyIdx := m.balancer.PickKey()
			log.Printf("-> %s: %s [%s] -> %s, try API key #%d",
				blueString("Processing request"),
				yellowString(task.TaskID),
				m.modelCode, task.Endpoint, keyIdx)

			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(task.Payload))
			if err != nil {
				errCh <- err
				return
			}

			m.setHeaders(httpReq, apiKey)
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

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}
				if line == "data: [DONE]" {
					outCh <- &Result{IsDone: true}
					break
				}
				if after, ok := strings.CutPrefix(line, "data: "); ok {
					data := after
					outCh <- &Result{Raw: json.RawMessage(data)}
				}
			}
			resp.Body.Close()
			return
		}
		errCh <- fmt.Errorf("all API keys failed: %w", lastErr)
	}()

	return outCh, errCh
}

func (m *OpenAIModel) CountTokens(prompt string) (int, error) {
	return len(prompt) / 4, nil
}

func (m *OpenAIModel) setHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func (m *OpenAIModel) buildURL(endpoint string) string {
	return strings.TrimSuffix(m.baseURL, "/") + "/" + strings.TrimPrefix(endpoint, "/")
}

type Registry struct {
	models     map[string]LLM
	modelCodes []string
}

func NewRegistry(cfg *Config) (*Registry, error) {
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
