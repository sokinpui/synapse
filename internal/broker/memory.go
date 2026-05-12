package broker

import (
	"sync"

	"github.com/sokinpui/synapse.go/internal/model"
	"github.com/sokinpui/synapse.go/internal/task"
)

type MemoryBroker struct {
	tasks         chan *task.GenerationTask
	subscribers   map[string]chan *model.Result
	cancellations map[string]chan struct{}
	mu            sync.RWMutex
}

func NewMemoryBroker(bufferSize int) *MemoryBroker {
	return &MemoryBroker{
		tasks:         make(chan *task.GenerationTask, bufferSize),
		subscribers:   make(map[string]chan *model.Result),
		cancellations: make(map[string]chan struct{}),
	}
}

func (b *MemoryBroker) Enqueue(task *task.GenerationTask) {
	b.tasks <- task
}

func (b *MemoryBroker) Dequeue() <-chan *task.GenerationTask {
	return b.tasks
}

func (b *MemoryBroker) Subscribe(id string) chan *model.Result {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *model.Result, 100)
	b.subscribers[id] = ch
	return ch
}

func (b *MemoryBroker) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
	}

	if cancelCh, ok := b.cancellations[id]; ok {
		close(cancelCh)
		delete(b.cancellations, id)
	}
}

func (b *MemoryBroker) Publish(id string, msg *model.Result) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if ch, ok := b.subscribers[id]; ok {
		ch <- msg
	}
}

func (b *MemoryBroker) SignalCancel(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.cancellations[id]; !ok {
		b.cancellations[id] = make(chan struct{})
	}
	close(b.cancellations[id])
	delete(b.cancellations, id)
}

func (b *MemoryBroker) IsCancelled(id string) <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.cancellations[id]; ok {
		return ch
	}

	ch := make(chan struct{})
	b.cancellations[id] = ch
	return ch
}
