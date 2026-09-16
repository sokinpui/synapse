package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

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
	targetURL := BuildTargetURL(reqCtx.BaseURL, reqCtx.Endpoint)

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

func (a *transparentAdapter) TransformResponse(ctx context.Context, reqCtx *RequestContext, resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}

func (a *transparentAdapter) TransformStream(ctx context.Context, resp *http.Response, out chan<- *Result) error {
	buf := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			out <- &Result{Raw: chunk}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
