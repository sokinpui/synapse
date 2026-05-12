package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/sokinpui/synapse.go/internal/broker"
	"github.com/sokinpui/synapse.go/internal/color"
	"github.com/sokinpui/synapse.go/internal/model"
	"github.com/sokinpui/synapse.go/internal/task"
)

// GenAIWorker dequeues and processes generation tasks.
type GenAIWorker struct {
	workerID    string
	broker      *broker.MemoryBroker
	llmRegistry *model.Registry
	concurrency int
}

func New(b *broker.MemoryBroker, llmRegistry *model.Registry, concurrency int) *GenAIWorker {
	return &GenAIWorker{
		workerID:    fmt.Sprintf("GenAIWorker-%d", os.Getpid()),
		broker:      b,
		llmRegistry: llmRegistry,
		concurrency: concurrency,
	}
}

func (w *GenAIWorker) Run(ctx context.Context) {
	log.Printf("%s started. Waiting for tasks... (concurrency: %d)", color.YellowString(w.workerID), w.concurrency)

	taskCh := w.broker.Dequeue()
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case task, ok := <-taskCh:
					if !ok {
						return
					}
					w.processTask(ctx, task)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	wg.Wait()
	log.Printf("%s all workers stopped.", w.workerID)
}

func (w *GenAIWorker) processTask(ctx context.Context, task *task.GenerationTask) {
	defer log.Printf("<- %s: %s [%s]", color.BlueString("Finished Request"), color.YellowString(task.TaskID), task.ModelCode)

	taskCtx, cancelTask := context.WithCancel(ctx)
	defer cancelTask()

	go w.listenForCancellation(taskCtx, task.TaskID, cancelTask)

	resultChannel := task.TaskID

	defer func() {
		w.broker.Publish(resultChannel, nil)
	}()

	llm, err := w.llmRegistry.GetModel(task.ModelCode)
	if err != nil {
		log.Printf("Error getting model for task %s: %v", task.TaskID, err)
		w.publishError(resultChannel, err)
		return
	}

	if task.Stream {
		err = w.processStream(taskCtx, task, llm)
	} else {
		err = w.process(taskCtx, task, llm)
	}

	if err != nil {
		if err == context.Canceled {
			log.Printf("Task %s was canceled.", task.TaskID)
			return
		}
		log.Printf("Error processing generation task %s: %v", task.TaskID, err)
		w.publishError(resultChannel, err)
	}
}

func (w *GenAIWorker) listenForCancellation(ctx context.Context, taskID string, cancel context.CancelFunc) {
	select {
	case <-w.broker.IsCancelled(taskID):
		cancel()
	case <-ctx.Done():
		return
	}
}

func (w *GenAIWorker) publishError(taskID string, err error) {
	errJSON, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"type":    "synapse_error",
		},
	})
	w.broker.Publish(taskID, &model.Result{Raw: errJSON, IsError: true})
}

func (w *GenAIWorker) process(ctx context.Context, task *task.GenerationTask, llm model.LLM) error {
	req := &model.Request{
		TaskID:  task.TaskID,
		Payload: task.Payload,
	}
	result, err := llm.Generate(ctx, req)
	if err != nil {
		return err
	}
	w.broker.Publish(task.TaskID, result)
	return nil
}

func (w *GenAIWorker) processStream(ctx context.Context, task *task.GenerationTask, llm model.LLM) error {
	req := &model.Request{
		TaskID:  task.TaskID,
		Payload: task.Payload,
	}
	outCh, errCh := llm.GenerateStream(ctx, req)

	for {
		select {
		case chunk, ok := <-outCh:
			if !ok {
				return nil // Stream finished
			}
			w.broker.Publish(task.TaskID, chunk)
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
