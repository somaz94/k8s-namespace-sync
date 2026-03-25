package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	finalizerName             = "namespacesync.nsync.dev/finalizer"
	AnnotationSourceNamespace = "namespacesync.nsync.dev/source-namespace"
	AnnotationSourceName      = "namespacesync.nsync.dev/source-name"
	AnnotationLastSync        = "namespacesync.nsync.dev/last-sync"
)

// handleDeletionAndStatus handles resource deletion and status updates
func (r *NamespaceSyncReconciler) handleDeletionAndStatus(ctx context.Context, namespacesync *syncv1.NamespaceSync) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(namespacesync, finalizerName) {
		// Clean up synced resources
		if err := r.cleanupSyncedResources(ctx, namespacesync); err != nil {
			log.Error(err, "Failed to cleanup resources")
			if r.Recorder != nil {
				r.Recorder.Eventf(namespacesync, corev1.EventTypeWarning, "CleanupFailed", "Failed to clean up synced resources: %v", err)
			}
			return ctrl.Result{}, err
		}

		if r.Recorder != nil {
			r.Recorder.Event(namespacesync, corev1.EventTypeNormal, "CleanupComplete", "Successfully cleaned up synced resources")
		}

		// Remove the finalizer
		controllerutil.RemoveFinalizer(namespacesync, finalizerName)
		if err := r.Update(ctx, namespacesync); err != nil {
			// Ignore if the resource has already been deleted
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// createOrUpdateResource is a generic function that handles creation or update of a Kubernetes resource.
// updateFields copies resource-specific data from desired to existing.
func createOrUpdateResource[T client.Object](
	r *NamespaceSyncReconciler,
	ctx context.Context,
	desired T,
	existing T,
	resourceType string,
	updateFields func(src, dst T),
) error {
	log := log.FromContext(ctx).WithValues(
		"namespace", desired.GetNamespace(),
		"name", desired.GetName(),
	)

	err := r.Get(ctx, types.NamespacedName{
		Namespace: desired.GetNamespace(),
		Name:      desired.GetName(),
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				log.Error(err, fmt.Sprintf("Failed to create %s", resourceType))
				recordSyncFailure(desired.GetNamespace(), resourceType)
				return err
			}
			log.Info(fmt.Sprintf("Successfully created %s", resourceType))
			recordSyncSuccess(desired.GetNamespace(), resourceType)
			return nil
		}
		return err
	}

	updateFields(desired, existing)

	if err := r.Update(ctx, existing); err != nil {
		log.Error(err, fmt.Sprintf("Failed to update %s", resourceType))
		recordSyncFailure(desired.GetNamespace(), resourceType)
		return err
	}

	log.Info(fmt.Sprintf("Successfully updated %s", resourceType))
	recordSyncSuccess(desired.GetNamespace(), resourceType)
	return nil
}

// copyLabelsAndAnnotations copies labels and annotations from source to destination
func (r *NamespaceSyncReconciler) copyLabelsAndAnnotations(src, dst *metav1.ObjectMeta) {
	if dst.Labels == nil {
		dst.Labels = make(map[string]string)
	}
	if dst.Annotations == nil {
		dst.Annotations = make(map[string]string)
	}

	// Copy labels and annotations, excluding kubernetes.io/ prefixed ones
	for k, v := range src.Labels {
		if !strings.HasPrefix(k, "kubernetes.io/") {
			dst.Labels[k] = v
		}
	}
	for k, v := range src.Annotations {
		if !strings.HasPrefix(k, "kubernetes.io/") {
			dst.Annotations[k] = v
		}
	}

	// Add sync metadata
	dst.Annotations[AnnotationSourceNamespace] = src.Namespace
	dst.Annotations[AnnotationSourceName] = src.Name
	dst.Annotations[AnnotationLastSync] = time.Now().Format(time.RFC3339)
}

// cleanupSyncedResources cleans up all synced resources
func (r *NamespaceSyncReconciler) cleanupSyncedResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync) error {
	log := log.FromContext(ctx)
	log.Info("Starting cleanup of synced resources")

	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Failed to list namespaces during cleanup")
		return err
	}

	var errs []error
	for _, ns := range namespaceList.Items {
		if !r.shouldSyncToNamespace(ctx, ns.Name, namespaceSync) {
			continue
		}

		// Delete Secrets
		for _, secretName := range namespaceSync.Spec.SecretName {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ns.Name,
				},
			}
			if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete synced Secret",
					"namespace", ns.Name,
					"name", secretName)
				recordCleanupFailure(ns.Name, "secret")
				errs = append(errs, err)
			} else {
				log.Info("Successfully deleted Secret",
					"namespace", ns.Name,
					"name", secretName)
				recordCleanupSuccess(ns.Name, "secret")
			}
		}

		// Delete ConfigMaps
		for _, configMapName := range namespaceSync.Spec.ConfigMapName {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: ns.Name,
				},
			}
			if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete synced ConfigMap",
					"namespace", ns.Name,
					"name", configMapName)
				recordCleanupFailure(ns.Name, "configmap")
				errs = append(errs, err)
			} else {
				log.Info("Successfully deleted ConfigMap",
					"namespace", ns.Name,
					"name", configMapName)
				recordCleanupSuccess(ns.Name, "configmap")
			}
		}
	}

	log.Info("Completed cleanup of synced resources")

	if len(errs) > 0 {
		return fmt.Errorf("encountered errors during cleanup: %v", errs)
	}
	return nil
}
