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

const finalizerName = "namespacesync.nsync.dev/finalizer"

// handleDeletionAndStatus handles resource deletion and status updates
func (r *NamespaceSyncReconciler) handleDeletionAndStatus(ctx context.Context, namespacesync *syncv1.NamespaceSync) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(namespacesync, finalizerName) {
		// 동기화된 리소스 정리
		if err := r.cleanupSyncedResources(ctx, namespacesync); err != nil {
			log.Error(err, "Failed to cleanup resources")
			return ctrl.Result{}, err
		}

		// Finalizer 제거
		controllerutil.RemoveFinalizer(namespacesync, finalizerName)
		if err := r.Update(ctx, namespacesync); err != nil {
			// 리소스가 이미 삭제된 경우 무시
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// createOrUpdateSecret handles the creation or update of a Secret
func (r *NamespaceSyncReconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	log := log.FromContext(ctx).WithValues(
		"namespace", secret.Namespace,
		"name", secret.Name,
	)

	// Check if secret exists in target namespace
	var existingSecret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, &existingSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new secret if it doesn't exist
			if err := r.Create(ctx, secret); err != nil {
				log.Error(err, "Failed to create Secret")
				syncFailureCounter.WithLabelValues(secret.Namespace, "secret").Inc()
				return err
			}
			log.Info("Successfully created Secret")
			syncSuccessCounter.WithLabelValues(secret.Namespace, "secret").Inc()
			return nil
		}
		return err
	}

	// Update existing secret
	existingSecret.Data = secret.Data
	existingSecret.StringData = secret.StringData
	existingSecret.Type = secret.Type
	r.copyLabelsAndAnnotations(&secret.ObjectMeta, &existingSecret.ObjectMeta)

	if err := r.Update(ctx, &existingSecret); err != nil {
		log.Error(err, "Failed to update Secret")
		syncFailureCounter.WithLabelValues(secret.Namespace, "secret").Inc()
		return err
	}

	log.Info("Successfully updated Secret")
	syncSuccessCounter.WithLabelValues(secret.Namespace, "secret").Inc()
	return nil
}

// createOrUpdateConfigMap handles the creation or update of a ConfigMap
func (r *NamespaceSyncReconciler) createOrUpdateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) error {
	log := log.FromContext(ctx).WithValues(
		"namespace", configMap.Namespace,
		"name", configMap.Name,
	)

	// Try to create or update the configmap
	var existingConfigMap corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{
		Namespace: configMap.Namespace,
		Name:      configMap.Name,
	}, &existingConfigMap)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new configmap if it doesn't exist
			if err := r.Create(ctx, configMap); err != nil {
				log.Error(err, "Failed to create ConfigMap")
				syncFailureCounter.WithLabelValues(configMap.Namespace, "configmap").Inc()
				return err
			}
			log.Info("Successfully created ConfigMap")
			syncSuccessCounter.WithLabelValues(configMap.Namespace, "configmap").Inc()
			return nil
		}
		return err
	}

	// Update existing configmap
	existingConfigMap.Data = configMap.Data
	existingConfigMap.BinaryData = configMap.BinaryData
	r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &existingConfigMap.ObjectMeta)

	if err := r.Update(ctx, &existingConfigMap); err != nil {
		log.Error(err, "Failed to update ConfigMap")
		syncFailureCounter.WithLabelValues(configMap.Namespace, "configmap").Inc()
		return err
	}

	log.Info("Successfully updated ConfigMap")
	syncSuccessCounter.WithLabelValues(configMap.Namespace, "configmap").Inc()
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
	dst.Annotations["namespacesync.nsync.dev/source-namespace"] = src.Namespace
	dst.Annotations["namespacesync.nsync.dev/source-name"] = src.Name
	dst.Annotations["namespacesync.nsync.dev/last-sync"] = time.Now().Format(time.RFC3339)
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
		if r.shouldSkipNamespace(ns.Name, namespaceSync.Spec.SourceNamespace, namespaceSync.Spec.Exclude) {
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
				errs = append(errs, err)
			} else {
				log.Info("Successfully deleted Secret",
					"namespace", ns.Name,
					"name", secretName)
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
				errs = append(errs, err)
			} else {
				log.Info("Successfully deleted ConfigMap",
					"namespace", ns.Name,
					"name", configMapName)
			}
		}
	}

	log.Info("Completed cleanup of synced resources")

	if len(errs) > 0 {
		return fmt.Errorf("encountered errors during cleanup: %v", errs)
	}
	return nil
}
