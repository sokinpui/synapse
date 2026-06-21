package server

import (
	"encoding/json"
	"io"
	"log"
	"strings"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sokinpui/synapse/internal/broker"
	"github.com/sokinpui/synapse/internal/color"
	"github.com/sokinpui/synapse/internal/model"
	"github.com/sokinpui/synapse/internal/task"
)

type HTTPServer struct {
	broker      *broker.MemoryBroker
	llmRegistry *model.Registry
}

func NewHTTPServer(b *broker.MemoryBroker, llmRegistry *model.Registry) *HTTPServer {
	return &HTTPServer{
		broker:      b,
		llmRegistry: llmRegistry,
	}
}

func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux) {
	// OpenAI Compatible API
	mux.HandleFunc("GET /v1/models", s.handleOpenAIListModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAIChatCompletions)
	mux.HandleFunc("POST /v1/images/generations", s.handleOpenAIImageGenerations)
}

func (s *HTTPServer) handleOpenAIListModels(w http.ResponseWriter, r *http.Request) {
	log.Printf("-> %s %s", color.BlueString(r.Method), r.URL.Path)

	modelCodes := s.llmRegistry.ListModels()
	now := time.Now().Unix()
	data := make([]model.ModelJSON, len(modelCodes))
	for i, m := range modelCodes {
		data[i] = model.ModelJSON{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "synapse",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.ModelListJSON{Object: "list", Data: data})
}

func (s *HTTPServer) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	modelCode, _ := payload["model"].(string)
	stream, _ := payload["stream"].(bool)

	parts := strings.SplitN(modelCode, "/", 2)
	if len(parts) == 2 {
		payload["model"] = parts[1]
	}

	s.ensureThoughtSignatures(payload)
	modifiedBody, _ := json.Marshal(payload)

	taskID := uuid.New().String()
	log.Printf("-> %s %s %s", color.BlueString(r.Method), r.URL.Path, color.YellowString(taskID))
	t := &task.GenerationTask{
		TaskID:    taskID,
		ModelCode: modelCode,
		Endpoint:  "/chat/completions",
		Stream:    stream,
		Payload:   modifiedBody,
	}

	resCh := s.broker.Subscribe(taskID)
	defer s.broker.Unsubscribe(taskID)
	s.broker.Enqueue(t)

	if t.Stream {
		s.streamOpenAIResults(w, r, t, resCh)
		return
	}
	s.redirectRawResult(w, resCh)
}

func (s *HTTPServer) handleOpenAIImageGenerations(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	modelCode, _ := payload["model"].(string)
	parts := strings.SplitN(modelCode, "/", 2)
	if len(parts) == 2 {
		payload["model"] = parts[1]
	}
	modifiedBody, _ := json.Marshal(payload)

	taskID := uuid.New().String()
	log.Printf("-> %s %s %s", color.BlueString(r.Method), r.URL.Path, color.YellowString(taskID))
	// Image generation typically isn't streamed in standard OpenAI API
	t := &task.GenerationTask{
		TaskID:    taskID,
		ModelCode: modelCode,
		Endpoint:  "/images/generations",
		Stream:    false,
		Payload:   modifiedBody,
	}

	resCh := s.broker.Subscribe(taskID)
	defer s.broker.Unsubscribe(taskID)

	s.broker.Enqueue(t)

	s.redirectRawResult(w, resCh)
}

func (s *HTTPServer) ensureThoughtSignatures(payload map[string]any) {
	messages, ok := payload["messages"].([]any)
	if !ok {
		return
	}

	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}

		toolCalls, ok := msg["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			continue
		}

		// Gemini requires a thought_signature on the first tool call of a response turn.
		// If missing (common in OpenAI clients), we inject a dummy to bypass validation.
		firstCall, ok := toolCalls[0].(map[string]any)
		if !ok {
			continue
		}

		if !hasGoogleSignature(firstCall) {
			injectDummySignature(firstCall)
		}
	}
}

func hasGoogleSignature(toolCall map[string]any) bool {
	extra, ok := toolCall["extra_content"].(map[string]any)
	if !ok {
		return false
	}
	google, ok := extra["google"].(map[string]any)
	if !ok {
		return false
	}
	_, exists := google["thought_signature"]
	return exists
}

func injectDummySignature(toolCall map[string]any) {
	toolCall["extra_content"] = map[string]any{
		"google": map[string]any{
			"thought_signature": "skip_thought_signature_validator",
		},
	}
}

func (s *HTTPServer) streamOpenAIResults(w http.ResponseWriter, r *http.Request, t *task.GenerationTask, ch <-chan *model.Result) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok || data == nil || data.IsDone {
				io.WriteString(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			io.WriteString(w, "data: ")
			w.Write(data.Raw)
			io.WriteString(w, "\n\n")
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) redirectRawResult(w http.ResponseWriter, ch <-chan *model.Result) {
	data := <-ch
	if data == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data.Raw)
}
