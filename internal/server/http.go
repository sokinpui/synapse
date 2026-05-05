package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sokinpui/synapse.go/internal/broker"
	"github.com/sokinpui/synapse.go/internal/color"
	"github.com/sokinpui/synapse.go/internal/model"
	"github.com/sokinpui/synapse.go/internal/task"
)

const sentinel = "[DONE]"

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
}

func (s *HTTPServer) handleOpenAIListModels(w http.ResponseWriter, r *http.Request) {
	modelCodes := s.llmRegistry.ListModels()
	now := time.Now().Unix()
	data := make([]OpenAIModel, len(modelCodes))
	for i, m := range modelCodes {
		data[i] = OpenAIModel{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "synapse",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(OpenAIModelList{Object: "list", Data: data})
}


func (s *HTTPServer) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	var oaiReq OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&oaiReq); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	taskID := uuid.New().String()
	log.Printf("-> %s: %s [%s]", color.BlueString("Received request"), taskID, oaiReq.Model)

	messages := make([]any, len(oaiReq.Messages))
	for i, m := range oaiReq.Messages {
		messages[i] = m
	}

	t := &task.GenerationTask{
		TaskID:    taskID,
		Messages:  messages,
		ModelCode: oaiReq.Model,
		Stream:    oaiReq.Stream,
		Config: &model.Config{
			Temperature:  oaiReq.Temperature,
			TopP:         oaiReq.TopP,
			OutputLength: oaiReq.MaxTokens,
		},
	}

	resCh := s.broker.Subscribe(taskID)
	defer s.broker.Unsubscribe(taskID)
	s.broker.Enqueue(t)

	if t.Stream {
		s.streamOpenAIResults(w, r, t, resCh)
		return
	}
	s.aggregateOpenAIResults(w, t, resCh)
}


func (s *HTTPServer) streamOpenAIResults(w http.ResponseWriter, r *http.Request, t *task.GenerationTask, ch <-chan string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	now := time.Now().Unix()
	first := true

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok || data == sentinel {
				stop := "stop"
				finalChunk := ChatCompletionChunk{
					ID:      fmt.Sprintf("chatcmpl-%s", t.TaskID),
					Object:  "chat.completion.chunk",
					Created: now,
					Model:   t.ModelCode,
					Choices: []ChunkChoice{
						{
							Index:        0,
							Delta:        OpenAIChatMessage{},
							FinishReason: &stop,
						},
					},
				}

				if jsonData, err := json.Marshal(finalChunk); err == nil {
					fmt.Fprintf(w, "data: %s\n\n", jsonData)
					flusher.Flush()
				}

				io.WriteString(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			chunk := ChatCompletionChunk{
				ID:      fmt.Sprintf("chatcmpl-%s", t.TaskID),
				Object:  "chat.completion.chunk",
				Created: now,
				Model:   t.ModelCode,
			}

			delta := OpenAIChatMessage{Content: data}
			if first {
				delta.Role = "assistant"
				first = false
			}

			chunk.Choices = []ChunkChoice{
				{
					Index:        0,
					Delta:        delta,
					FinishReason: nil,
				},
			}

			jsonData, err := json.Marshal(chunk)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}

func (s *HTTPServer) aggregateOpenAIResults(w http.ResponseWriter, t *task.GenerationTask, ch <-chan string) {
	var sb strings.Builder
	for data := range ch {
		if data == sentinel {
			break
		}
		sb.WriteString(data)
	}

	now := time.Now().Unix()

	resp := OpenAIChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", t.TaskID),
		Object:  "chat.completion",
		Created: now,
		Model:   t.ModelCode,
		Choices: []Choice{
			{
				Index: 0,
				Message: OpenAIChatMessage{
					Role:    "assistant",
					Content: sb.String(),
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
