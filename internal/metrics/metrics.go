// Package metrics defines the service's Prometheus instruments.
package metrics

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const namespace = "taskmgmt"

// Metrics owns a private registry, so tests can create independent
// instances and nothing leaks through the global default registry.
type Metrics struct {
	Registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpInFlight prometheus.Gauge
	cacheLookups *prometheus.CounterVec
	breakerState prometheus.Gauge
}

func New(build string) *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),

		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "http_requests_total",
			Help: "HTTP requests by method, route template and status code.",
		}, []string{"method", "route", "status"}),

		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "http_request_duration_seconds",
			Help:    "HTTP request latency by method and route template.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"method", "route"}),

		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "http_requests_in_flight",
			Help: "Requests currently being served.",
		}),

		cacheLookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "cache_lookups_total",
			Help: "Cache lookups by cache name and result (hit, miss, error).",
		}, []string{"cache", "result"}),

		breakerState: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Name: "redis_circuit_breaker_state",
			Help: "Redis circuit breaker state: 0 closed, 1 half-open, 2 open.",
		}),
	}

	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace, Name: "build_info",
		Help:        "Build information; always 1.",
		ConstLabels: prometheus.Labels{"version": build},
	})
	buildInfo.Set(1)

	m.Registry.MustRegister(
		m.httpRequests, m.httpDuration, m.httpInFlight, m.cacheLookups, m.breakerState, buildInfo,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// RegisterDB exports connection-pool statistics (open, in use, waits, ...).
func (m *Metrics) RegisterDB(db *sql.DB, name string) {
	m.Registry.MustRegister(collectors.NewDBStatsCollector(db, name))
}

// HTTP records request count, latency and in-flight requests. Install it
// outside the access logger so the status it sees is final. Routes are
// labelled by template (/tasks/:id), never by raw path, to keep label
// cardinality bounded.
func (m *Metrics) HTTP() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		m.httpInFlight.Inc()
		defer m.httpInFlight.Dec()

		err := c.Next()

		// Fiber/fasthttp strings point into buffers that are reused after
		// the request ends. Label values outlive the request (they become
		// map keys inside the CounterVec), so they must be copied, or a
		// later request rewrites an existing series' labels in place.
		route := strings.Clone(c.Route().Path)
		method := strings.Clone(c.Method())
		m.httpRequests.WithLabelValues(method, route, strconv.Itoa(c.Response().StatusCode())).Inc()
		m.httpDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		return err
	}
}

// CacheLookup counts a cache lookup outcome.
func (m *Metrics) CacheLookup(cache, result string) {
	m.cacheLookups.WithLabelValues(cache, result).Inc()
}

// SetBreakerState records the Redis breaker state (0 closed, 1 half-open, 2 open).
func (m *Metrics) SetBreakerState(state int) {
	m.breakerState.Set(float64(state))
}
