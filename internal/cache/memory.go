package cache

import (
	"context"
	"sync"
)

type memoryCache struct {
	mu    sync.Mutex
	views map[string]int64
}

func newMemoryCache() *memoryCache { return &memoryCache{views: map[string]int64{}} }

func (m *memoryCache) IncrView(_ context.Context, id string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.views[id]++
	return m.views[id], nil
}

func (m *memoryCache) GetViews(_ context.Context, id string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.views[id], nil
}

func (m *memoryCache) Mode() string                  { return "memory" }
func (m *memoryCache) Close(_ context.Context) error { return nil }
