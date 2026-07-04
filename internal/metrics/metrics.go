// Package metrics exposes a Prometheus registry. The /metrics endpoint serves
// these counters/histograms. Each adapter increments its own counters where
// relevant (uploads, cache hits, event publishes, etc).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "docket_http_requests_total",
		Help: "Total HTTP requests handled, partitioned by route and status.",
	}, []string{"route", "method", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "docket_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})

	UploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "docket_uploads_total",
		Help: "Files uploaded, partitioned by storage backend mode.",
	}, []string{"mode"})

	EventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "docket_events_published_total",
		Help: "Events published to the event bus, partitioned by subject prefix.",
	}, []string{"topic"})

	CacheOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "docket_cache_ops_total",
		Help: "Cache operations partitioned by op and result.",
	}, []string{"op", "result"})
)
