package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/sokinpui/synapse.go/internal/color"
)

// OpenAIModel is a generic implementation of the LLM interface for OpenAI-compatible APIs.
type OpenAIModel struct {
	modelCode string
	baseURL   string
	balancer  *KeyBalancer
	client    *http.Client
}

func NewOpenAIModel(modelCode, baseURL string, balancer *KeyBalancer) *OpenAIModel {
	return &OpenAIModel{
		modelCode: modelCode,
		baseURL:   baseURL,
		balancer:  balancer,
		client:    &http.Client{},
	}
}

func (m *OpenAIModel) Generate(ctx context.Context, req *Request) (string, error) {
	if m.balancer.KeyCount() == 0 {
		return "", fmt.Errorf("%w: API key is required", ErrConfiguration)
	}

	var lastErr error
	payload := m.buildPayload(req, false)
	bodyBytes, _ := json.Marshal(payload)

	for i := 0; i < m.balancer.KeyCount(); i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		apiKey, keyIdx := m.balancer.PickKey()
		log.Printf("-> %s: %s [%s], try API key #%d", color.YellowString("Processing request"), req.TaskID, m.modelCode, keyIdx)

		httpReq, err := http.NewRequestWithContext(ctx, "POST", m.baseURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", err
		}

		m.setHeaders(httpReq, apiKey)
		resp, err := m.client.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status code: %d", resp.StatusCode)
			continue
		}

		var oaiResp openAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
			lastErr = err
			continue
		}

		if len(oaiResp.Choices) > 0 {
			return oaiResp.Choices[0].Message.Content, nil
		}
	}

	return "", fmt.Errorf("all API keys failed: %w", lastErr)
}

func (m *OpenAIModel) GenerateStream(ctx context.Context, req *Request) (<-chan string, <-chan error) {
	outCh := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if m.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required", ErrConfiguration)
			return
		}

		var lastErr error
		payload := m.buildPayload(req, true)
		bodyBytes, _ := json.Marshal(payload)

		for i := 0; i < m.balancer.KeyCount(); i++ {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			apiKey, keyIdx := m.balancer.PickKey()
			log.Printf("-> %s: %s [%s], try API key #%d", color.YellowString("Processing request"), req.TaskID, m.modelCode, keyIdx)

			httpReq, err := http.NewRequestWithContext(ctx, "POST", m.baseURL, bytes.NewReader(bodyBytes))
			if err != nil {
				errCh <- err
				return
			}

			m.setHeaders(httpReq, apiKey)
			resp, err := m.client.Do(httpReq)
			if err != nil {
				lastErr = err
				continue
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				lastErr = fmt.Errorf("status code: %d", resp.StatusCode)
				continue
			}

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					break
				}
				var chunk openAIStreamChunk
				if err := json.Unmarshal([]byte(data), &chunk); err == nil && len(chunk.Choices) > 0 {
					outCh <- chunk.Choices[0].Delta.Content
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

func (m *OpenAIModel) buildPayload(req *Request, stream bool) map[string]any {
	payload := map[string]any{
		"model":    m.modelCode,
		"messages": req.Messages,
		"stream":   stream,
	}

	if req.Config == nil {
		return payload
	}

	if req.Config.Temperature != nil {
		payload["temperature"] = *req.Config.Temperature
	}
	if req.Config.TopP != nil {
		payload["top_p"] = *req.Config.TopP
	}
	if req.Config.OutputLength > 0 {
		payload["max_tokens"] = req.Config.OutputLength
	}

	return payload
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}
