package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type RequestContext struct {
	BaseURL   string
	Endpoint  string
	ModelCode string
	APIKey    string
	Stream    bool
	Payload   json.RawMessage
}

type ProviderAdapter interface {
	Name() string
	BuildRequest(ctx context.Context, reqCtx *RequestContext) (*http.Request, error)
	TransformResponse(resp *http.Response) ([]byte, error)
	TransformStream(ctx context.Context, resp *http.Response, out chan<- *Result) error
}

type transparentAdapter struct{}

func TransparentAdapter() ProviderAdapter {
	return &transparentAdapter{}
}

func NewTransparentAdapter() ProviderAdapter {
	return TransparentAdapter()
}

func (a *transparentAdapter) Name() string {
	return "transparent"
}
func (a *transparentAdapter) BuildRequest(ctx context.Context, reqCtx *RequestContext) (*http.Request, error) {
	targetURL := buildTargetURL(reqCtx.BaseURL, reqCtx.Endpoint)

	payload := reqCtx.Payload
	if len(payload) > 0 {
		var rawMap map[string]any
		if err := json.Unmarshal(payload, &rawMap); err == nil {
			rawMap["model"] = reqCtx.ModelCode
			if marshaled, err := json.Marshal(rawMap); err == nil {
				payload = marshaled
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if reqCtx.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+reqCtx.APIKey)
	}

	return req, nil
}

func (a *transparentAdapter) TransformResponse(resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}

func (a *transparentAdapter) TransformStream(ctx context.Context, resp *http.Response, out chan<- *Result) error {
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "data: [DONE]" {
			out <- &Result{IsDone: true}
			return nil
		}

		if after, ok := strings.CutPrefix(line, "data: "); ok {
			out <- &Result{Raw: json.RawMessage(after)}
		}
	}

	return scanner.Err()
}

func buildTargetURL(baseURL, endpoint string) string {
	base := strings.TrimRight(baseURL, "/")
	ep := strings.TrimLeft(endpoint, "/")

	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(ep, "v1/") {
		ep = strings.TrimPrefix(ep, "v1/")
	}

	return base + "/" + ep
}
