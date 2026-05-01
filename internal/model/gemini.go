package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/sokinpui/synapse.go/internal/config"
)

func init() {
	RegisterProvider(newGeminiProvider)
}

func newGeminiProvider(cfg *config.Config) (map[string]LLM, error) {
	apiKeysVar := os.Getenv("GENAI_API_KEYS")
	rawKeys := strings.FieldsFunc(apiKeysVar, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == '\\'
	})

	var apiKeys []string
	for _, k := range rawKeys {
		if trimmed := strings.TrimSpace(k); trimmed != "" {
			apiKeys = append(apiKeys, trimmed)
		}
	}

	log.Printf("Gemini provider initialized with %d API keys", len(apiKeys))

	models := make(map[string]LLM)
	balancer := NewKeyBalancer(apiKeys)
	client := &http.Client{}

	baseURL := cfg.Models.Gemini.BaseURL
	for _, code := range cfg.Models.Gemini.Codes {
		models[code] = &GeminiModel{
			model:    code,
			balancer: balancer,
			client:   client,
			baseURL:  baseURL,
		}
	}

	return models, nil
}

type GeminiModel struct {
	model    string
	baseURL  string
	balancer *KeyBalancer
	client   *http.Client
}

// Generate performs a non-streaming text generation.
func (m *GeminiModel) Generate(ctx context.Context, req *Request) (string, error) {
	if m.balancer.KeyCount() == 0 {
		return "", fmt.Errorf("%w: API key is required for generation", ErrConfiguration)
	}

	var lastErr error
	bodyBytes, _ := json.Marshal(m.buildOpenAIRequest(req, false))

	for i := 0; i < m.balancer.KeyCount(); i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		apiKey, keyIdx := m.balancer.PickKey()
		log.Printf("[%s] Attempting generation with API key #%d", m.model, keyIdx)

		url := m.baseURL
		if url == "" {
			url = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)

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
		json.NewDecoder(resp.Body).Decode(&oaiResp)
		if len(oaiResp.Choices) > 0 {
			return oaiResp.Choices[0].Message.Content, nil
		}
	}
	return "", fmt.Errorf("all API keys failed: %w", lastErr)
}

// GenerateStream performs a streaming text generation.
func (m *GeminiModel) GenerateStream(ctx context.Context, req *Request) (<-chan string, <-chan error) {
	outCh := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if m.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required for generation", ErrConfiguration)
			return
		}

		var lastErr error
		bodyBytes, _ := json.Marshal(m.buildOpenAIRequest(req, true))

		for i := 0; i < m.balancer.KeyCount(); i++ {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			apiKey, keyIdx := m.balancer.PickKey()
			log.Printf("[%s] Attempting stream generation with API key #%d", m.model, keyIdx)

			url := m.baseURL
			if url == "" {
				url = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
			}
			httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
			if err != nil {
				errCh <- err
				return
			}
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)

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

// CountTokens counts the number of tokens in a prompt.
func (m *GeminiModel) CountTokens(prompt string) (int, error) {
	return len(prompt) / 4, nil
}

func (m *GeminiModel) buildOpenAIRequest(req *Request, stream bool) map[string]any {
	payload := map[string]any{
		"model":    m.model,
		"messages": req.Messages,
		"stream":   stream,
	}

	if req.Config != nil {
		if req.Config.Temperature != nil {
			payload["temperature"] = *req.Config.Temperature
		}
		if req.Config.TopP != nil {
			payload["top_p"] = *req.Config.TopP
		}
		if req.Config.OutputLength > 0 {
			payload["max_tokens"] = req.Config.OutputLength
		}
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
