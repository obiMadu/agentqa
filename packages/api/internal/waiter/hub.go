package waiter

import (
	"context"
	"sync"
)

type Result struct {
	Status  string
	Answers [][]string
}

type Hub struct {
	mu      sync.Mutex
	waiters map[string][]chan Result
}

func NewHub() *Hub {
	return &Hub{waiters: make(map[string][]chan Result)}
}

func (h *Hub) Wait(ctx context.Context, questionID string) (Result, error) {
	ch := make(chan Result, 1)

	h.mu.Lock()
	h.waiters[questionID] = append(h.waiters[questionID], ch)
	h.mu.Unlock()

	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (h *Hub) Notify(questionID string, result Result) {
	h.mu.Lock()
	waiters := h.waiters[questionID]
	delete(h.waiters, questionID)
	h.mu.Unlock()

	for _, ch := range waiters {
		ch <- result
		close(ch)
	}
}
