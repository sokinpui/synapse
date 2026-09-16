package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type glmAdapter struct {
	transparent ProviderAdapter
}

func GLMAdapter() ProviderAdapter {
	return &glmAdapter{
		transparent: TransparentAdapter(),
	}
}

func NewGLMAdapter() ProviderAdapter {
	return GLMAdapter()
}

func (a *glmAdapter) Name() string {
	return "glm"
}

func (a *glmAdapter) BuildRequest(ctx context.Context, reqCtx *RequestContext) (*http.Request, error) {
	if !isResponsesEndpoint(reqCtx.Endpoint) {
		return a.transparent.BuildRequest(ctx, reqCtx)
	}

	var respReq struct {
		Model        string         `json:"model"`
		Instructions string         `json:"instructions"`
		Input        any            `json:"input"`
		Stream       bool           `json:"stream"`
		Tools        []any          `json:"tools"`
		ToolChoice   any            `json:"tool_choice"`
		Reasoning    map[string]any `json:"reasoning"`
		Temperature  *float64       `json:"temperature"`
		MaxTokens    *int           `json:"max_output_tokens"`
	}

	if err := json.Unmarshal(reqCtx.Payload, &respReq); err != nil {
		return nil, fmt.Errorf("invalid responses payload: %w", err)
	}

	messages, err := convertResponsesInputToMessages(respReq.Instructions, respReq.Input)
	if err != nil {
		return nil, err
	}

	chatPayload := map[string]any{
		"model":    reqCtx.ModelCode,
		"messages": messages,
		"stream":   reqCtx.Stream,
	}

	if len(respReq.Tools) > 0 {
		chatPayload["tools"] = convertTools(respReq.Tools)
		if respReq.ToolChoice != nil {
			chatPayload["tool_choice"] = respReq.ToolChoice
		}
	}

	if respReq.Temperature != nil {
		chatPayload["temperature"] = *respReq.Temperature
	}
	if respReq.MaxTokens != nil {
		chatPayload["max_tokens"] = *respReq.MaxTokens
	}
	if effort, ok := respReq.Reasoning["effort"].(string); ok && effort != "" {
		chatPayload["thinking"] = map[string]any{
			"type": "enabled",
		}
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

func (a *glmAdapter) TransformResponse(ctx context.Context, reqCtx *RequestContext, resp *http.Response) ([]byte, error) {
	if !isResponsesEndpoint(reqCtx.Endpoint) {
		return a.transparent.TransformResponse(ctx, reqCtx, resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse upstream chat completion: %w", err)
	}

	if chatResp.Error != nil {
		return body, nil
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("upstream provider returned no completion choices")
	}

	msg := chatResp.Choices[0].Message
	var output []map[string]any

	if msg.Content != "" {
		output = append(output, map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{
					"type": "output_text",
					"text": msg.Content,
				},
			},
		})
	}

	for _, tc := range msg.ToolCalls {
		output = append(output, map[string]any{
			"type":      "function_call",
			"id":        tc.ID,
			"call_id":   tc.ID,
			"name":      tc.Function.Name,
			"arguments": tc.Function.Arguments,
		})
	}

	respResult := map[string]any{
		"output": output,
	}
	return json.Marshal(respResult)
}

func (a *glmAdapter) TransformStream(ctx context.Context, resp *http.Response, out chan<- *Result) error {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	var activeToolCallID string
	var activeToolName string
	var activeToolArgs strings.Builder

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if activeToolCallID != "" {
				EmitSSEEvent(out, map[string]any{
					"type": "response.output_item.done",
					"item": map[string]any{
						"type":      "function_call",
						"id":        activeToolCallID,
						"call_id":   activeToolCallID,
						"name":      activeToolName,
						"arguments": activeToolArgs.String(),
					},
				})
			}
			EmitSSEEvent(out, map[string]any{"type": "response.completed"})
			out <- &Result{Raw: []byte("data: [DONE]\n\n")}
			return nil
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			EmitSSEEvent(out, map[string]any{
				"type": "error",
				"error": map[string]any{
					"message": chunk.Error.Message,
				},
			})
			return nil
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.ReasoningContent != "" {
			EmitSSEEvent(out, map[string]any{
				"type":  "response.reasoning_text.delta",
				"delta": delta.ReasoningContent,
			})
		}

		if delta.Content != "" {
			EmitSSEEvent(out, map[string]any{
				"type":  "response.output_text.delta",
				"delta": delta.Content,
			})
		}

		if len(delta.ToolCalls) > 0 {
			tc := delta.ToolCalls[0]
			if tc.ID != "" && tc.ID != activeToolCallID {
				if activeToolCallID != "" {
					EmitSSEEvent(out, map[string]any{
						"type": "response.output_item.done",
						"item": map[string]any{
							"type":      "function_call",
							"id":        activeToolCallID,
							"call_id":   activeToolCallID,
							"name":      activeToolName,
							"arguments": activeToolArgs.String(),
						},
					})
					activeToolArgs.Reset()
				}
				activeToolCallID = tc.ID
				activeToolName = tc.Function.Name
				EmitSSEEvent(out, map[string]any{
					"type": "response.output_item.added",
					"item": map[string]any{
						"type":    "function_call",
						"id":      tc.ID,
						"call_id": tc.ID,
						"name":    tc.Function.Name,
					},
				})
			}
			if tc.Function.Arguments != "" {
				activeToolArgs.WriteString(tc.Function.Arguments)
				EmitSSEEvent(out, map[string]any{
					"type":      "response.function_call_arguments.delta",
					"delta":     tc.Function.Arguments,
					"arguments": tc.Function.Arguments,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func isResponsesEndpoint(endpoint string) bool {
	ep := strings.TrimRight(endpoint, "/")
	return strings.HasSuffix(ep, "/responses")
}

func convertTools(tools []any) []any {
	var result []any
	for _, t := range tools {
		tmap, ok := t.(map[string]any)
		if !ok {
			result = append(result, t)
			continue
		}
		if _, hasFunc := tmap["function"]; hasFunc {
			result = append(result, t)
			continue
		}

		fn := make(map[string]any)
		if name, ok := tmap["name"]; ok {
			fn["name"] = name
		}
		if desc, ok := tmap["description"]; ok {
			fn["description"] = desc
		}
		if params, ok := tmap["parameters"]; ok {
			fn["parameters"] = params
		}

		result = append(result, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	return result
}

func convertResponsesInputToMessages(instructions string, input any) ([]map[string]any, error) {
	var messages []map[string]any

	if strings.TrimSpace(instructions) != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": instructions,
		})
	}

	if input == nil {
		return messages, nil
	}

	if inputStr, ok := input.(string); ok {
		messages = append(messages, map[string]any{
			"role":    "user",
			"content": inputStr,
		})
		return messages, nil
	}

	inputItems, ok := input.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid input format: expected string or array")
	}

	for _, item := range inputItems {
		rawMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		itemType, _ := rawMap["type"].(string)
		switch itemType {
		case "function_call":
			callID, _ := rawMap["call_id"].(string)
			name, _ := rawMap["name"].(string)
			arguments, _ := rawMap["arguments"].(string)
			messages = append(messages, map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{
					{
						"id":   callID,
						"type": "function",
						"function": map[string]any{
							"name":      name,
							"arguments": arguments,
						},
					},
				},
			})
		case "function_call_output":
			callID, _ := rawMap["call_id"].(string)
			output, _ := rawMap["output"].(string)
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      output,
			})
		default:
			role, _ := rawMap["role"].(string)
			if role == "" {
				role = "user"
			}
			content := normalizeContent(rawMap["content"])
			if len(messages) > 0 && messages[len(messages)-1]["role"] == role {
				prevContent, isPrevStr := messages[len(messages)-1]["content"].(string)
				currContent, isCurrStr := content.(string)
				if isPrevStr && isCurrStr {
					messages[len(messages)-1]["content"] = prevContent + "\n\n" + currContent
					continue
				}
			}
			messages = append(messages, map[string]any{
				"role":    role,
				"content": content,
			})
		}
	}

	return messages, nil
}

func normalizeContent(rawContent any) any {
	if rawContent == nil {
		return ""
	}
	if str, ok := rawContent.(string); ok {
		return str
	}

	parts, ok := rawContent.([]any)
	if !ok {
		return rawContent
	}

	hasImage := false
	for _, p := range parts {
		pmap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		t, _ := pmap["type"].(string)
		if t == "image_url" || t == "input_image" {
			hasImage = true
			break
		}
	}

	if !hasImage {
		var sb strings.Builder
		for _, p := range parts {
			if str, ok := p.(string); ok {
				sb.WriteString(str)
				continue
			}
			if pmap, ok := p.(map[string]any); ok {
				if text, ok := pmap["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	}

	var converted []map[string]any
	for _, p := range parts {
		pmap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		t, _ := pmap["type"].(string)
		switch t {
		case "input_text", "text":
			text, _ := pmap["text"].(string)
			converted = append(converted, map[string]any{
				"type": "text",
				"text": text,
			})
		case "input_image", "image_url":
			converted = append(converted, map[string]any{
				"type":      "image_url",
				"image_url": pmap["image_url"],
			})
		default:
			converted = append(converted, pmap)
		}
	}
	return converted
}
