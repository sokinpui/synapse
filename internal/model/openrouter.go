package model

import (
	"context"
	"fmt"
	"encoding/json"
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

	baseURL := cfg.Models.OpenRouter.BaseURL
	for _, code := range cfg.Models.OpenRouter.Codes {
		model, err := NewOpenRouterModel(ctx, code, balancer, baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenRouter model '%s': %w", code, err)
		}
		models[code] = model
	}
	return models, nil
}

type OpenRouterModel struct {
	model    string
	baseURL  string
	balancer *KeyBalancer
}

func NewOpenRouterModel(ctx context.Context, modelCode string, balancer *KeyBalancer, baseURL string) (*OpenRouterModel, error) {
	return &OpenRouterModel{
		model:    modelCode,
		balancer: balancer,
		baseURL:  baseURL,
	}, nil
}

func (orm *OpenRouterModel) Generate(ctx context.Context, req *Request) (string, error) {
	if orm.balancer.KeyCount() == 0 {
		return "", fmt.Errorf("%w: API key is required for OpenRouter", ErrConfiguration)
	}

	apiKey, keyIdx := orm.balancer.PickKey()
	log.Printf("[%s] Attempting generation with API key #%d", orm.model, keyIdx)

	/* TODO: Image support is not yet implemented for OpenRouter provider */
	client := openrouter.NewClient(apiKey)
	chatReq := openrouter.ChatCompletionRequest{
		Model: orm.model,
		Messages: orm.mapMessages(req.Messages),
	}

	if req.Config != nil {
		orm.applyConfig(&chatReq, req.Config)
	}

	response, err := client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return "", fmt.Errorf("OpenRouter API error: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("%w: no choices in response", ErrGeneration)
	}

	return response.Choices[0].Message.Content.Text, nil
}

func (orm *OpenRouterModel) GenerateStream(ctx context.Context, req *Request) (<-chan string, <-chan error) {
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
		chatReq := openrouter.ChatCompletionRequest{
			Model: orm.model,
			Messages: orm.mapMessages(req.Messages),
			Stream: true,
		}

		if req.Config != nil {
			orm.applyConfig(&chatReq, req.Config)
		}

		stream, err := client.CreateChatCompletionStream(ctx, chatReq)
		if err != nil && err != io.EOF {
			errCh <- fmt.Errorf("OpenRouter API error: %w", err)
			return
		}
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					log.Printf("Stream error for %s: %v", orm.model, err)
				}
				break
			}
			if len(response.Choices) > 0 {
				outCh <- response.Choices[0].Delta.Content
			}
		}
	}()

	return outCh, errCh
}

func (orm *OpenRouterModel) mapMessages(msgs []any) []openrouter.ChatCompletionMessage {
	result := make([]openrouter.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		// Since Messages come from the OpenAI compatible layer in HTTPServer,
		// they are JSON-serializable map[string]any or the OpenAIChatMessage struct.
		// We can use JSON marshalling as a robust way to convert if the underlying type is known.
		data, err := json.Marshal(m)
		if err != nil {
			continue
		}

		var oaiMsg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(data, &oaiMsg); err == nil {
			result = append(result, openrouter.ChatCompletionMessage{Role: oaiMsg.Role, Content: openrouter.ChatCompletionContent{Text: oaiMsg.Content}})
		}
	}
	return result
}

func (orm *OpenRouterModel) applyConfig(chatReq *openrouter.ChatCompletionRequest, cfg *Config) {
	if cfg.Temperature != nil {
		chatReq.Temperature = *cfg.Temperature
	}
	if cfg.TopP != nil {
		chatReq.TopP = *cfg.TopP
	}
	if cfg.TopK != nil {
		chatReq.TopK = int(*cfg.TopK)
	}
	if cfg.OutputLength > 0 {
		chatReq.MaxCompletionTokens = int(cfg.OutputLength)
	}
}

func (orm *OpenRouterModel) CountTokens(prompt string) (int, error) {
	/* Heuristic: 1 English character ≈ 0.3 token, 1 Non-English character ≈ 0.6 token. */
	var tokenCount float32 = 0.0
	for _, r := range prompt {
		if r <= 127 {
			tokenCount += 0.3
			continue
		}
		tokenCount += 0.6
	}
	return int(tokenCount), nil
}
