package model

import (
	"context"
	"errors"
	"fmt"
	"log"
	"github.com/sokinpui/synapse.go/internal/config"
	"google.golang.org/genai"
	"google.golang.org/genai/tokenizer"
	"os"
	"strings"
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
	ctx := context.Background()
	balancer := NewKeyBalancer(apiKeys)

	for _, code := range cfg.Models.Gemini.Codes {
		model, err := NewGeminiModel(ctx, code, balancer)
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini model '%s': %w", code, err)
		}
		models[code] = model
	}

	return models, nil
}

type GeminiModel struct {
	model    string
	balancer *KeyBalancer
}

func NewGeminiModel(ctx context.Context, modelCode string, balancer *KeyBalancer) (*GeminiModel, error) {
	return &GeminiModel{
		model:    modelCode,
		balancer: balancer,
	}, nil
}

// Generate performs a non-streaming text generation.
func (m *GeminiModel) Generate(ctx context.Context, prompt string, images [][]byte, config *Config) (string, error) {
	if m.balancer.KeyCount() == 0 {
		return "", fmt.Errorf("%w: API key is required for generation", ErrConfiguration)
	}

	content, err := buildContent(prompt, images)
	if err != nil {
		return "", err
	}

	genConfig := getGenConfig(config)
	var lastErr error

	for i := 0; i < m.balancer.KeyCount(); i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		apiKey, keyIdx := m.balancer.PickKey()
		log.Printf("[%s] Attempting generation with API key #%d", m.model, keyIdx)

		client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
		if err != nil {
			lastErr = fmt.Errorf("failed to create genai client: %w", err)
			log.Printf("Gemini API key [#%d] failed for model %s, retrying... Error: %v", keyIdx, m.model, err)
			continue
		}

		resp, err := client.Models.GenerateContent(ctx, m.model, content, genConfig)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			lastErr = fmt.Errorf("%w: %v", ErrGeneration, err)
			log.Printf("Gemini API key [#%d] failed for model %s, retrying... Error: %v", keyIdx, m.model, err)
			continue
		}

		if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("%w: no content in response", ErrGeneration)
		}

		return resp.Text(), nil
	}

	return "", fmt.Errorf("all API keys failed: %w", lastErr)
}

// GenerateStream performs a streaming text generation.
func (m *GeminiModel) GenerateStream(ctx context.Context, prompt string, images [][]byte, config *Config) (<-chan string, <-chan error) {
	genConfig := getGenConfig(config)
	outCh := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if m.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required for generation", ErrConfiguration)
			return
		}

		content, err := buildContent(prompt, images)
		if err != nil {
			errCh <- err
			return
		}

		var lastErr error

		for i := 0; i < m.balancer.KeyCount(); i++ {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			apiKey, keyIdx := m.balancer.PickKey()
			log.Printf("[%s] Attempting stream generation with API key #%d", m.model, keyIdx)

			client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
			if err != nil {
				lastErr = fmt.Errorf("failed to create genai client: %w", err)
				log.Printf("Gemini API key [#%d] failed for model %s (stream), retrying... Error: %v", keyIdx, m.model, err)
				continue
			}

			streamErr := func() error {
				iter := client.Models.GenerateContentStream(ctx, m.model, content, genConfig)
				for resp, err := range iter {
					if err != nil {
						return err
					}
					if resp != nil && len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
						outCh <- resp.Text()
					}
				}
				return nil
			}()

			if streamErr != nil {
				if errors.Is(streamErr, context.Canceled) {
					errCh <- streamErr
					return
				}
				lastErr = fmt.Errorf("%w: %v", ErrGeneration, streamErr)
				log.Printf("Gemini API key [#%d] failed for model %s (stream), retrying... Error: %v", keyIdx, m.model, streamErr)
				continue
			}
			return // Success
		}

		errCh <- fmt.Errorf("all API keys failed: %w", lastErr)
	}()
	return outCh, errCh
}

func buildContent(prompt string, images [][]byte) ([]*genai.Content, error) {
	parts := []*genai.Part{genai.NewPartFromText(prompt)}

	for _, imgBytes := range images {
		parts = append(parts, genai.NewPartFromBytes(imgBytes, "image/jpeg"))
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	return contents, nil
}

// CountTokens counts the number of tokens in a prompt.
func (m *GeminiModel) CountTokens(prompt string) (int, error) {
	tok, err := tokenizer.NewLocalTokenizer("gemini-2.5-flash")
	if err != nil {
		return 0, fmt.Errorf("token counting failed: %w", err)
	}

	ntoks, err := tok.CountTokens(genai.Text(prompt), nil)
	if err != nil {
		return 0, fmt.Errorf("token counting failed: %w", err)
	}

	return int(ntoks.TotalTokens), nil
}

func getGenConfig(config *Config) *genai.GenerateContentConfig {
	if config == nil {
		return &genai.GenerateContentConfig{}
	}

	// var tools = []*genai.Tool{
	// 	{
	// 		GoogleSearch: &genai.GoogleSearch{},
	// 		URLContext:   &genai.URLContext{},
	// 	},
	// }

	// disable tools if code is gemini-3-flash-preview
	// if m.model == "gemini-3-flash-preview" {
	// 	tools = nil
	// }

	return &genai.GenerateContentConfig{
		Temperature:     config.Temperature,
		TopP:            config.TopP,
		TopK:            config.TopK,
		MaxOutputTokens: config.OutputLength,
		// Tools:           tools,
	}
}
