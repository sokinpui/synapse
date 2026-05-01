package model

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	openrouter "github.com/revrost/go-openrouter"
	"github.com/sokinpui/synapse.go/internal/config"
)

func init() {
	RegisterProvider(newOpenRouterProvider)
}

func newOpenRouterProvider(cfg *config.Config) (map[string]LLM, error) {
	apiKeysVar := os.Getenv("OPENROUTER_API_KEYS")
	if apiKeysVar == "" {
		apiKeysVar = os.Getenv("OPENROUTER_API_KEY")
	}

	rawKeys := strings.FieldsFunc(apiKeysVar, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == '\\'
	})

	var apiKeys []string
	for _, k := range rawKeys {
		if trimmed := strings.TrimSpace(k); trimmed != "" {
			apiKeys = append(apiKeys, trimmed)
		}
	}

	log.Printf("OpenRouter provider initialized with %d API keys", len(apiKeys))

	models := make(map[string]LLM)
	ctx := context.Background()
	balancer := NewKeyBalancer(apiKeys)

	for _, code := range cfg.Models.OpenRouter.Codes {
		model, err := NewOpenRouterModel(ctx, code, balancer)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenRouter model '%s': %w", code, err)
		}
		models[code] = model
	}
	return models, nil
}

type OpenRouterModel struct {
	model    string
	balancer *KeyBalancer
}

func NewOpenRouterModel(ctx context.Context, modelCode string, balancer *KeyBalancer) (*OpenRouterModel, error) {
	return &OpenRouterModel{
		model:    modelCode,
		balancer: balancer,
	}, nil
}

func (orm *OpenRouterModel) Generate(ctx context.Context, prompt string, images [][]byte, config *Config) (string, error) {
	if orm.balancer.KeyCount() == 0 {
		return "", fmt.Errorf("%w: API key is required for OpenRouter", ErrConfiguration)
	}

	apiKey, keyIdx := orm.balancer.PickKey()
	log.Printf("[%s] Attempting generation with API key #%d", orm.model, keyIdx)

	/* TODO: don't support Image yet */
	client := openrouter.NewClient(apiKey)
	req := openrouter.ChatCompletionRequest{
		Model: orm.model,
		Messages: []openrouter.ChatCompletionMessage{
			openrouter.UserMessage(prompt),
		},
	}

	if config != nil {
		if config.Temperature != nil {
			req.Temperature = *config.Temperature
		}
		if config.TopP != nil {
			req.TopP = *config.TopP
		}
		if config.TopK != nil {
			req.TopK = int(*config.TopK)
		}
		if config.OutputLength > 0 {
			req.MaxCompletionTokens = int(config.OutputLength)
		}
	}

	response, err := client.CreateChatCompletion(ctx, req)

	if err != nil {
		return "", fmt.Errorf("OpenRouter API error: %w", err)
	}

	return response.Choices[0].Message.Content.Text, nil
}

func (orm *OpenRouterModel) GenerateStream(ctx context.Context, prompt string, images [][]byte, config *Config) (<-chan string, <-chan error) {
	outCh := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(outCh)
		defer close(errCh)

		if orm.balancer.KeyCount() == 0 {
			errCh <- fmt.Errorf("%w: API key is required for OpenRouter", ErrConfiguration)
			return
		}

		apiKey, keyIdx := orm.balancer.PickKey()
		log.Printf("[%s] Attempting stream generation with API key #%d", orm.model, keyIdx)

		client := openrouter.NewClient(apiKey)
		req := openrouter.ChatCompletionRequest{
			Model: orm.model,
			Messages: []openrouter.ChatCompletionMessage{
				openrouter.UserMessage(prompt),
			},
			Stream: true,
		}

		if config != nil {
			if config.Temperature != nil {
				req.Temperature = *config.Temperature
			}
			if config.TopP != nil {
				req.TopP = *config.TopP
			}
			if config.TopK != nil {
				req.TopK = int(*config.TopK)
			}
			if config.OutputLength > 0 {
				req.MaxCompletionTokens = int(config.OutputLength)
			}
		}
		stream, err := client.CreateChatCompletionStream(ctx, req)

		if err != nil && err != io.EOF {
			errCh <- fmt.Errorf("OpenRouter API error: %w", err)
			return
		}

		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err != nil {
				break
			}
			outCh <- response.Choices[0].Delta.Content
		}
	}()

	return outCh, errCh
}

func (orm *OpenRouterModel) CountTokens(prompt string) (int, error) {
	/* 1 English character ≈ 0.3 token.
	1 Chinese character ≈ 0.6 token. */
	// loop via each char
	var tokenConnt float32 = 0.0
	for _, r := range prompt {
		if r <= 127 {
			// English char
			tokenConnt += 0.3
		} else {
			// Non-English char
			tokenConnt += 0.6
		}
	}
	return int(tokenConnt), nil

}
