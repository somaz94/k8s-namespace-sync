package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	syncSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_success_total",
			Help: "Number of successful resource synchronizations",
		},
		[]string{"namespace", "resource_type"},
	)

	syncFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_failure_total",
			Help: "Number of failed resource synchronizations",
		},
		[]string{"namespace", "resource_type"},
	)

	syncConflictCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_conflict_total",
			Help: "Number of resource synchronizations skipped because another owner holds the object",
		},
		[]string{"namespace", "resource_type"},
	)

	cleanupSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_cleanup_success_total",
			Help: "Number of successful resource cleanups",
		},
		[]string{"namespace", "resource_type"},
	)

	cleanupFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_cleanup_failure_total",
			Help: "Number of failed resource cleanups",
		},
		[]string{"namespace", "resource_type"},
	)

	syncDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "namespacesync_sync_duration_seconds",
			Help:    "Duration of sync operations in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to 5.12s
		},
		[]string{"namespace", "resource_type"},
	)

	resourceCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "namespacesync_managed_resources",
			Help: "Number of resources being managed by NamespaceSync",
		},
		[]string{"namespace", "resource_type"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		syncSuccessCounter,
		syncFailureCounter,
		syncConflictCounter,
		cleanupSuccessCounter,
		cleanupFailureCounter,
		syncDurationHistogram,
		resourceCount,
	)
}

// Metric recording helpers

func recordSyncSuccess(namespace, resourceType string) {
	syncSuccessCounter.WithLabelValues(namespace, resourceType).Inc()
}

func recordSyncFailure(namespace, resourceType string) {
	syncFailureCounter.WithLabelValues(namespace, resourceType).Inc()
}

func recordSyncConflict(namespace, resourceType string) {
	syncConflictCounter.WithLabelValues(namespace, resourceType).Inc()
}

func recordCleanupSuccess(namespace, resourceType string) {
	cleanupSuccessCounter.WithLabelValues(namespace, resourceType).Inc()
}

func recordCleanupFailure(namespace, resourceType string) {
	cleanupFailureCounter.WithLabelValues(namespace, resourceType).Inc()
}
