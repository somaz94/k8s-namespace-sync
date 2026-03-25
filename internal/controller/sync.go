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

// syncResourceList iterates over resource names and calls syncFn for each,
// optionally filtering by a ResourceFilter. This eliminates duplication
// between secret and configmap sync loops.
func (r *NamespaceSyncReconciler) syncResourceList(ctx context.Context, names []string, filter *syncv1.ResourceFilter, syncFn func(string) error, resourceType string, targetNamespace string) error {
	log := log.FromContext(ctx)
	for _, name := range names {
		if filter != nil && !r.shouldSyncResource(name, filter) {
			continue
		}
		if err := syncFn(name); err != nil {
			log.Error(err, "Failed to sync resource", "resourceType", resourceType, "name", name, "targetNamespace", targetNamespace)
			return err
		}
		log.Info("Successfully synced resource", "resourceType", resourceType, "name", name, "targetNamespace", targetNamespace)
	}
	return nil
}

// syncResources handles the synchronization of resources to target namespace
func (r *NamespaceSyncReconciler) syncResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("syncResources called",
		"targetNamespace", targetNamespace,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"secretCount", len(namespaceSync.Spec.SecretName),
		"configMapCount", len(namespaceSync.Spec.ConfigMapName))

	var secretFilter, configMapFilter *syncv1.ResourceFilter
	if namespaceSync.Spec.ResourceFilters != nil {
		secretFilter = namespaceSync.Spec.ResourceFilters.Secrets
		configMapFilter = namespaceSync.Spec.ResourceFilters.ConfigMaps
	}

	// Sync Secrets
	if err := r.syncResourceList(ctx, namespaceSync.Spec.SecretName, secretFilter, func(name string) error {
		return r.syncSecret(ctx, namespaceSync.Spec.SourceNamespace, targetNamespace, name)
	}, "secret", targetNamespace); err != nil {
		return err
	}

	// Sync ConfigMaps
	return r.syncResourceList(ctx, namespaceSync.Spec.ConfigMapName, configMapFilter, func(name string) error {
		return r.syncConfigMap(ctx, namespaceSync.Spec.SourceNamespace, targetNamespace, name)
	}, "configmap", targetNamespace)
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

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespace,
		},
		Type: secret.Type,
		Data: secret.Data,
	}
	r.copyLabelsAndAnnotations(&secret.ObjectMeta, &newSecret.ObjectMeta)

	return createOrUpdateResource(r, ctx, newSecret, &corev1.Secret{}, "secret",
		func(src, dst *corev1.Secret) {
			dst.Data = src.Data
			dst.StringData = src.StringData
			dst.Type = src.Type
			dst.Labels = src.Labels
			dst.Annotations = src.Annotations
		})
}

// syncConfigMap synchronizes a single configmap to the target namespace
func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, sourceNamespace, targetNamespace, configMapName string) error {
	log := log.FromContext(ctx)

	// Get source configmap
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: sourceNamespace,
		Name:      configMapName,
	}, &configMap); err != nil {
		if errors.IsNotFound(err) {
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

	newConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: targetNamespace,
		},
		Data:       configMap.Data,
		BinaryData: configMap.BinaryData,
	}
	r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &newConfigMap.ObjectMeta)

	return createOrUpdateResource(r, ctx, newConfigMap, &corev1.ConfigMap{}, "configmap",
		func(src, dst *corev1.ConfigMap) {
			dst.Data = src.Data
			dst.BinaryData = src.BinaryData
			dst.Labels = src.Labels
			dst.Annotations = src.Annotations
		})
}
