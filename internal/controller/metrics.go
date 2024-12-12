package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// syncSuccessCounter tracks successful resource synchronizations
	syncSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_success_total",
			Help: "Number of successful resource synchronizations",
		},
		[]string{"namespace", "resource_type"},
	)

	// syncFailureCounter tracks failed resource synchronizations
	syncFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_failure_total",
			Help: "Number of failed resource synchronizations",
		},
		[]string{"namespace", "resource_type"},
	)

	// cleanupSuccessCounter tracks successful resource cleanups
	cleanupSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_cleanup_success_total",
			Help: "Number of successful resource cleanups",
		},
		[]string{"namespace", "resource_type"},
	)

	// cleanupFailureCounter tracks failed resource cleanups
	cleanupFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_cleanup_failure_total",
			Help: "Number of failed resource cleanups",
		},
		[]string{"namespace", "resource_type"},
	)

	// syncDurationHistogram tracks the duration of sync operations
	syncDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "namespacesync_sync_duration_seconds",
			Help:    "Duration of sync operations in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
		[]string{"namespace", "resource_type"},
	)

	// resourceCount tracks the number of resources being managed
	resourceCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "namespacesync_managed_resources",
			Help: "Number of resources being managed by NamespaceSync",
		},
		[]string{"namespace", "resource_type"},
	)
)

func init() {
	// Register all metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		syncSuccessCounter,
		syncFailureCounter,
		cleanupSuccessCounter,
		cleanupFailureCounter,
		syncDurationHistogram,
		resourceCount,
	)
}
