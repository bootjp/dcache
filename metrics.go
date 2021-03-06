package dcache

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: name,
		Name:      "hits_total",
		Help:      "The count of cache hits.",
	}, []string{"server"})

	cacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: name,
		Name:      "misses_total",
		Help:      "The count of cache misses.",
	}, []string{"server"})

	corruptedCache = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: name,
		Name:      "discard_cache_total",
		Help:      "The count of cache discard data of corrupted.",
	}, []string{"server"})

	redisErr = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: name,
		Name:      "redis_errors_total",
		Help:      "The count of errors when publish and subscribe entries to redis.",
	}, []string{"server"})
)
