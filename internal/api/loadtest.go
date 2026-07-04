package api

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/example/docket/internal/logging"
	"github.com/example/docket/internal/metadata"
)

// LoadTest fans out N concurrent internal "view file" operations and returns
// per-percentile latency. It exercises Postgres (metadata lookup) + Valkey (incr)
// in tight succession so a developer can watch traces and Prom metrics light
// up under load.
func (h *handlers) LoadTest(w http.ResponseWriter, r *http.Request) {
	n := atoiDefault(r.URL.Query().Get("n"), 1000)
	if n > 10000 {
		n = 10000
	}
	conc := atoiDefault(r.URL.Query().Get("concurrency"), 50)
	if conc < 1 {
		conc = 1
	}
	if conc > n {
		conc = n
	}

	log := logging.FromContext(r.Context(), h.app.Log)
	log.Info("loadtest starting", "n", n, "concurrency", conc)

	list, _ := h.app.Metadata.List(r.Context(), 100, 0)
	if len(list) == 0 {
		http.Error(w, "no files to load-test against; call /seed first", http.StatusBadRequest)
		return
	}

	durations := make([]time.Duration, n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, conc)
	start := time.Now()

	for i := 0; i < n; i++ {
		i := i
		target := list[i%len(list)]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			durations[i] = oneOp(r.Context(), h, target)
		}()
	}
	wg.Wait()
	total := time.Since(start)

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p := func(pct float64) time.Duration { return durations[int(float64(len(durations))*pct)] }

	writeJSON(w, http.StatusOK, map[string]any{
		"requests":    n,
		"concurrency": conc,
		"total_ms":    total.Milliseconds(),
		"rps":         float64(n) / total.Seconds(),
		"p50_ms":      p(0.50).Milliseconds(),
		"p90_ms":      p(0.90).Milliseconds(),
		"p95_ms":      p(0.95).Milliseconds(),
		"p99_ms":      p(0.99).Milliseconds(),
		"max_ms":      durations[len(durations)-1].Milliseconds(),
	})
}

func oneOp(ctx context.Context, h *handlers, target metadata.Meta) time.Duration {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	t := time.Now()
	_, _ = h.app.Metadata.Get(opCtx, target.ID)
	_, _ = h.app.Cache.IncrView(opCtx, target.ID)
	return time.Since(t)
}
