package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type aisrpImageAdapter struct {
	transparent ProviderAdapter
}

func AISRPImageAdapter() ProviderAdapter {
	return &aisrpImageAdapter{
		transparent: TransparentAdapter(),
	}
}

func NewAISRPImageAdapter() ProviderAdapter {
	return AISRPImageAdapter()
}

func (a *aisrpImageAdapter) Name() string {
	return "aisrp-image"
}

func (a *aisrpImageAdapter) BuildRequest(ctx context.Context, reqCtx *RequestContext) (*http.Request, error) {
	if !isImageGenerationEndpoint(reqCtx.Endpoint) {
		return a.transparent.BuildRequest(ctx, reqCtx)
	}

	var imgReq struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(reqCtx.Payload, &imgReq); err != nil {
		return nil, fmt.Errorf("invalid image generation payload: %w", err)
	}

	chatPayload := map[string]any{
		"model": reqCtx.ModelCode,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": imgReq.Prompt,
			},
		},
	}

	marshaled, err := json.Marshal(chatPayload)
	if err != nil {
		return nil, err
	}

	targetURL := BuildTargetURL(reqCtx.BaseURL, "/chat/completions")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(marshaled))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if reqCtx.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+reqCtx.APIKey)
	}

	return req, nil
}

func (a *aisrpImageAdapter) TransformResponse(ctx context.Context, reqCtx *RequestContext, resp *http.Response) ([]byte, error) {
	if !isImageGenerationEndpoint(reqCtx.Endpoint) {
		return a.transparent.TransformResponse(ctx, reqCtx, resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse upstream chat completion: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("upstream provider returned no completion choices")
	}

	base64Data, err := extractBase64(chatResp.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}

	imageResult := map[string]any{
		"created": time.Now().Unix(),
		"data": []map[string]string{
			{
				"b64_json": base64Data,
			},
		},
	}

	return json.Marshal(imageResult)
}

func (a *aisrpImageAdapter) TransformStream(ctx context.Context, resp *http.Response, out chan<- *Result) error {
	return a.transparent.TransformStream(ctx, resp, out)
}

func isImageGenerationEndpoint(endpoint string) bool {
	ep := strings.TrimRight(endpoint, "/")
	return strings.HasSuffix(ep, "/images/generations")
}

func extractBase64(content string) (string, error) {
	idx := strings.Index(content, "base64,")
	if idx == -1 {
		return "", errors.New("no base64 image found in upstream response content")
	}

	raw := content[idx+len("base64,"):]
	end := strings.IndexFunc(raw, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' || r == '-' || r == '_')
	})
	if end != -1 {
		raw = raw[:end]
	}
	if raw == "" {
		return "", errors.New("empty base64 image data")
	}
	return raw, nil
}
