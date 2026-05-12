package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sokinpui/synapse.go/internal/broker"
	"github.com/sokinpui/synapse.go/internal/color"
	"github.com/sokinpui/synapse.go/internal/model"
	"github.com/sokinpui/synapse.go/internal/task"
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
}

func (s *HTTPServer) handleOpenAIListModels(w http.ResponseWriter, r *http.Request) {
	log.Printf("-> %s %s", color.BlueString(r.Method), r.URL.Path)

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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var head struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &head)

	taskID := uuid.New().String()
	log.Printf("-> %s %s %s", color.BlueString(r.Method), r.URL.Path, color.YellowString(taskID))

	t := &task.GenerationTask{
		TaskID:    taskID,
		ModelCode: head.Model,
		Stream:    head.Stream,
		Payload:   body,
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
	if data == nil { return }
	w.Header().Set("Content-Type", "application/json")
	w.Write(data.Raw)
}
