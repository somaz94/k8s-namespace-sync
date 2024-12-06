package controller

import (
	"context"
	"fmt"
	"time"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// updateStatus updates the status of the NamespaceSync resource
func (r *NamespaceSyncReconciler) updateStatus(ctx context.Context, namespaceSync *syncv1.NamespaceSync, syncedNamespaces []string, failedNamespaces map[string]string) error {
	log := log.FromContext(ctx)

	// Skip status update if resource is being deleted
	if namespaceSync.DeletionTimestamp != nil {
		log.Info("Resource is being deleted, skipping status update")
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version before updating status
		latest := &syncv1.NamespaceSync{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(namespaceSync), latest); err != nil {
			return err
		}

		// Update status fields
		latest.Status.LastSyncTime = metav1.NewTime(time.Now())
		latest.Status.SyncedNamespaces = syncedNamespaces
		latest.Status.FailedNamespaces = failedNamespaces
		latest.Status.ObservedGeneration = latest.Generation

		// Update Ready condition
		readyCondition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "SyncComplete",
			Message:            fmt.Sprintf("Successfully synced to %d namespaces", len(syncedNamespaces)),
		}

		if len(failedNamespaces) > 0 {
			readyCondition.Status = metav1.ConditionFalse
			readyCondition.Reason = "SyncFailed"
			readyCondition.Message = fmt.Sprintf("Failed to sync to %d namespaces", len(failedNamespaces))
		}

		meta.SetStatusCondition(&latest.Status.Conditions, readyCondition)

		// Update the status
		return r.Status().Update(ctx, latest)
	})
}

// calculateSyncStats returns statistics about sync operations
func calculateSyncStats(syncedNamespaces []string, failedNamespaces map[string]string) (int, int, float64) {
	totalNamespaces := len(syncedNamespaces) + len(failedNamespaces)
	successCount := len(syncedNamespaces)
	failureCount := len(failedNamespaces)

	var successRate float64
	if totalNamespaces > 0 {
		successRate = float64(successCount) / float64(totalNamespaces) * 100
	}

	return successCount, failureCount, successRate
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// logSyncStats logs sync operation statistics
func logSyncStats(ctx context.Context, successCount, failureCount int, successRate float64, duration time.Duration) {
	log := log.FromContext(ctx)
	log.Info("Sync operation completed",
		"successCount", successCount,
		"failureCount", failureCount,
		"successRate", fmt.Sprintf("%.1f%%", successRate),
		"duration", formatDuration(duration))
}

// validateNamespaceSync validates the NamespaceSync resource
func validateNamespaceSync(namespaceSync *syncv1.NamespaceSync) error {
	if namespaceSync.Spec.SourceNamespace == "" {
		return fmt.Errorf("sourceNamespace is required")
	}

	if len(namespaceSync.Spec.SecretName) == 0 && len(namespaceSync.Spec.ConfigMapName) == 0 {
		return fmt.Errorf("at least one secret or configmap must be specified")
	}

	return nil
}
