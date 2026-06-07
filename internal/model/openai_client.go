package model

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

	"github.com/sokinpui/synapse/internal/color"
)

type ModelListJSON struct {
	Object string      `json:"object"`
	Data   []ModelJSON `json:"data"`
}

type ModelJSON struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIModel is a generic implementation of the LLM interface for OpenAI-compatible APIs.
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

func (m *OpenAIModel) Generate(ctx context.Context, req *Request) (*Result, error) {
	if m.balancer.KeyCount() == 0 {
		return nil, fmt.Errorf("%w: API key is required", ErrConfiguration)
	}

	targetURL := m.buildURL(req.Endpoint)

	maxAttempts := m.maxRetry + 1
	var lastErr error

	for i := 0; i < maxAttempts && i < m.balancer.KeyCount(); i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		apiKey, keyIdx := m.balancer.PickKey()
		log.Printf("-> %s: %s [%s] -> %s, try API key #%d", 
			color.BlueString("Processing request"), 
			color.YellowString(req.TaskID), 
			m.modelCode, req.Endpoint, keyIdx)

		httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(req.Payload))
		if err != nil {
			return nil, err
		}

		m.setHeaders(httpReq, apiKey)
		resp, err := m.client.Do(httpReq)
		if err != nil {
			lastErr = err
			log.Printf("!! %s: %s [key #%d] network error: %v", color.YellowString("Attempt failed"), req.TaskID, keyIdx, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("status code: %d, body: %s", resp.StatusCode, string(body))
			log.Printf("!! %s: %s [key #%d] provider error: %v", color.YellowString("Attempt failed"), req.TaskID, keyIdx, lastErr)
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

func (m *OpenAIModel) GenerateStream(ctx context.Context, req *Request) (<-chan *Result, <-chan error) {
	outCh := make(chan *Result)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if m.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required", ErrConfiguration)
			return
		}

		targetURL := m.buildURL(req.Endpoint)

		maxAttempts := m.maxRetry + 1
		var lastErr error

		for i := 0; i < maxAttempts && i < m.balancer.KeyCount(); i++ {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			apiKey, keyIdx := m.balancer.PickKey()
			log.Printf("-> %s: %s [%s] -> %s, try API key #%d", 
				color.BlueString("Processing request"), 
				color.YellowString(req.TaskID), 
				m.modelCode, req.Endpoint, keyIdx)

			httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(req.Payload))
			if err != nil {
				errCh <- err
				return
			}

			m.setHeaders(httpReq, apiKey)
			resp, err := m.client.Do(httpReq)
			if err != nil {
				lastErr = err
				log.Printf("!! %s: %s [key #%d] network error: %v", color.YellowString("Attempt failed"), req.TaskID, keyIdx, err)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				lastErr = fmt.Errorf("status code: %d, body: %s", resp.StatusCode, string(body))
				log.Printf("!! %s: %s [key #%d] provider error: %v", color.YellowString("Attempt failed"), req.TaskID, keyIdx, lastErr)
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
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
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
