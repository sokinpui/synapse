package adapter

import (
	"context"
	"encoding/json"
	"fmt"
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

type Result struct {
	Raw     []byte
	IsError bool
}

type ProviderAdapter interface {
	Name() string
	BuildRequest(ctx context.Context, reqCtx *RequestContext) (*http.Request, error)
	TransformResponse(ctx context.Context, reqCtx *RequestContext, resp *http.Response) ([]byte, error)
	TransformStream(ctx context.Context, reqCtx *RequestContext, resp *http.Response, out chan<- *Result) error
}

func BuildTargetURL(baseURL, endpoint string) string {
	base := strings.TrimRight(baseURL, "/")
	ep := strings.TrimLeft(endpoint, "/")

	if strings.HasPrefix(ep, "v1/") && isVersionedBase(base) {
		ep = strings.TrimPrefix(ep, "v1/")
	}

	return base + "/" + ep
}

func isVersionedBase(baseURL string) bool {
	lastSlash := strings.LastIndex(baseURL, "/")
	if lastSlash == -1 {
		return false
	}

	segment := baseURL[lastSlash+1:]
	if len(segment) < 2 || (segment[0] != 'v' && segment[0] != 'V') {
		return false
	}

	for _, ch := range segment[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func EmitSSEEvent(out chan<- *Result, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	out <- &Result{Raw: fmt.Appendf(nil, "data: %s\n\n", data)}
}
