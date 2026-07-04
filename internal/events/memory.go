package events

import (
	"context"
	"log/slog"
	"sync"
)

type memoryBus struct {
	mu     sync.Mutex
	events []Event
	log    *slog.Logger
}

func newMemoryBus(log *slog.Logger) *memoryBus { return &memoryBus{log: log} }

func (m *memoryBus) Publish(_ context.Context, ev Event) error {
	m.mu.Lock()
	m.events = append(m.events, ev)
	m.mu.Unlock()
	m.log.Info("memory event published (no downstream consumer)", "type", ev.Type, "file_id", ev.FileID)
	return nil
}

func (m *memoryBus) Mode() string                  { return "memory" }
func (m *memoryBus) Close(_ context.Context) error { return nil }
