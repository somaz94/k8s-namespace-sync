package controller

import (
	"context"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// syncResources handles the synchronization of resources to target namespace
func (r *NamespaceSyncReconciler) syncResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("syncResources called",
		"targetNamespace", targetNamespace,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"secretCount", len(namespaceSync.Spec.SecretName),
		"configMapCount", len(namespaceSync.Spec.ConfigMapName))

	// Sync Secrets
	if namespaceSync.Spec.ResourceFilters == nil || namespaceSync.Spec.ResourceFilters.Secrets == nil {
		// 기존 로직 유지
		for _, secretName := range namespaceSync.Spec.SecretName {
			if err := r.syncSecret(ctx, namespaceSync.Spec.SourceNamespace, targetNamespace, secretName); err != nil {
				log.Error(err, "Failed to sync secret",
					"secretName", secretName,
					"targetNamespace", targetNamespace)
				return err
			}
			log.Info("Successfully synced secret",
				"secretName", secretName,
				"targetNamespace", targetNamespace)
		}
	} else {
		// 필터링 적용
		for _, secretName := range namespaceSync.Spec.SecretName {
			if r.shouldSyncResource(secretName, namespaceSync.Spec.ResourceFilters.Secrets) {
				if err := r.syncSecret(ctx, namespaceSync.Spec.SourceNamespace, targetNamespace, secretName); err != nil {
					log.Error(err, "Failed to sync secret",
						"secretName", secretName,
						"targetNamespace", targetNamespace)
					return err
				}
				log.Info("Successfully synced secret",
					"secretName", secretName,
					"targetNamespace", targetNamespace)
			}
		}
	}

	// Sync ConfigMaps
	if namespaceSync.Spec.ResourceFilters == nil || namespaceSync.Spec.ResourceFilters.ConfigMaps == nil {
		// 기존 로직 유지
		for _, configMapName := range namespaceSync.Spec.ConfigMapName {
			if err := r.syncConfigMap(ctx, namespaceSync, targetNamespace, configMapName); err != nil {
				log.Error(err, "Failed to sync configmap",
					"configMapName", configMapName,
					"targetNamespace", targetNamespace)
				return err
			}
			log.Info("Successfully synced configmap",
				"configMapName", configMapName,
				"targetNamespace", targetNamespace)
		}
	} else {
		// 필터링 적용
		for _, configMapName := range namespaceSync.Spec.ConfigMapName {
			if r.shouldSyncResource(configMapName, namespaceSync.Spec.ResourceFilters.ConfigMaps) {
				if err := r.syncConfigMap(ctx, namespaceSync, targetNamespace, configMapName); err != nil {
					log.Error(err, "Failed to sync configmap",
						"configMapName", configMapName,
						"targetNamespace", targetNamespace)
					return err
				}
				log.Info("Successfully synced configmap",
					"configMapName", configMapName,
					"targetNamespace", targetNamespace)
			}
		}
	}

	return nil
}

// syncSecret synchronizes a single secret to the target namespace
func (r *NamespaceSyncReconciler) syncSecret(ctx context.Context, sourceNamespace, targetNamespace, secretName string) error {
	log := log.FromContext(ctx)

	// Get source secret
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: sourceNamespace,
		Name:      secretName,
	}, &secret); err != nil {
		if errors.IsNotFound(err) {
			// Source secret was deleted, delete from target namespace
			targetSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: targetNamespace,
				},
			}
			if err := r.Delete(ctx, targetSecret); err != nil && !errors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted secret from target namespace as it was deleted from source",
				"secret", secretName,
				"targetNamespace", targetNamespace)
			return nil
		}
		return err
	}

	// Create or update secret in target namespace
	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespace,
		},
		Type: secret.Type,
		Data: secret.Data,
	}
	r.copyLabelsAndAnnotations(&secret.ObjectMeta, &newSecret.ObjectMeta)

	return r.createOrUpdateSecret(ctx, newSecret)
}

// syncConfigMap synchronizes a single configmap to the target namespace
func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace, configMapName string) error {
	log := log.FromContext(ctx)

	// Get source configmap
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceSync.Spec.SourceNamespace,
		Name:      configMapName,
	}, &configMap); err != nil {
		if errors.IsNotFound(err) {
			// Source configmap was deleted, delete from target namespace
			targetConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: targetNamespace,
				},
			}
			if err := r.Delete(ctx, targetConfigMap); err != nil && !errors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted configmap from target namespace as it was deleted from source",
				"configmap", configMapName,
				"targetNamespace", targetNamespace)
			return nil
		}
		return err
	}

	// Create or update configmap in target namespace
	newConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: targetNamespace,
		},
		Data:       configMap.Data,
		BinaryData: configMap.BinaryData,
	}
	r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &newConfigMap.ObjectMeta)

	return r.createOrUpdateConfigMap(ctx, newConfigMap)
}
