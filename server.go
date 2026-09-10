package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type HTTPServer struct {
	broker      *MemoryBroker
	llmRegistry *Registry
}

func NewHTTPServer(b *MemoryBroker, llmRegistry *Registry) *HTTPServer {
	return &HTTPServer{
		broker:      b,
		llmRegistry: llmRegistry,
	}
}

func (s *HTTPServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/models", s.handleOpenAIListModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleTask)
	mux.HandleFunc("POST /v1/images/generations", s.handleTask)
	mux.HandleFunc("POST /v1/responses", s.handleTask)
}

func (s *HTTPServer) handleOpenAIListModels(w http.ResponseWriter, r *http.Request) {
	log.Printf("-> %s %s", blueString(r.Method), r.URL.Path)

	modelCodes := s.llmRegistry.ListModels()
	now := time.Now().Unix()
	data := make([]ModelJSON, len(modelCodes))
	for i, m := range modelCodes {
		data[i] = ModelJSON{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "synapse",
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ModelListJSON{Object: "list", Data: data})
}

type routingHeader struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

func (s *HTTPServer) handleTask(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var header routingHeader
	if err := json.Unmarshal(body, &header); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if header.Model == "" {
		http.Error(w, "missing model in request", http.StatusBadRequest)
		return
	}

	taskID := uuid.New().String()
	log.Printf("-> %s %s %s", blueString(r.Method), r.URL.Path, yellowString(taskID))

	t := &GenerationTask{
		TaskID:    taskID,
		ModelCode: header.Model,
		Endpoint:  r.URL.Path,
		Stream:    header.Stream,
		Payload:   body,
	}

	resCh := s.broker.Subscribe(taskID)
	defer s.broker.Unsubscribe(taskID)

	s.broker.Enqueue(t)
	if t.Stream {
		s.streamOpenAIResults(w, r, t, resCh)
		return
	}
	s.redirectRawResult(w, r, taskID, resCh)
}

func (s *HTTPServer) streamOpenAIResults(w http.ResponseWriter, r *http.Request, t *GenerationTask, ch <-chan *Result) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	var firstResult *Result
	select {
	case <-r.Context().Done():
		s.broker.SignalCancel(t.TaskID)
		return
	case res, ok := <-ch:
		if !ok || res == nil {
			return
		}
		firstResult = res
	}

	if firstResult.IsError {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write(firstResult.Raw)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeChunk := func(data *Result) bool {
		if data.IsDone {
			io.WriteString(w, "data: [DONE]\n\n")
			flusher.Flush()
			return false
		}
		io.WriteString(w, "data: ")
		w.Write(data.Raw)
		io.WriteString(w, "\n\n")
		flusher.Flush()
		return true
	}

	if !writeChunk(firstResult) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			s.broker.SignalCancel(t.TaskID)
			return
		case data, ok := <-ch:
			if !ok || data == nil {
				io.WriteString(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			if data.IsError {
				io.WriteString(w, "data: ")
				w.Write(data.Raw)
				io.WriteString(w, "\n\n")
				flusher.Flush()
				return
			}
			if !writeChunk(data) {
				return
			}
		}
	}
}

func (s *HTTPServer) redirectRawResult(w http.ResponseWriter, r *http.Request, taskID string, ch <-chan *Result) {
	var data *Result
	select {
	case <-r.Context().Done():
		s.broker.SignalCancel(taskID)
		return
	case data = <-ch:
	}
	if data == nil {
		http.Error(w, "no response from worker", http.StatusBadGateway)
		return
	}
	if data.IsError {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write(data.Raw)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data.Raw)
}
