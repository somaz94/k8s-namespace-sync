package controller

import (
	"context"
	"errors"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// syncResourceList calls syncFn for each name that passes filter. A conflict only skips its name; it is
// returned alongside the first other error, which stops the list.
func (r *NamespaceSyncReconciler) syncResourceList(ctx context.Context, names []string, filter *syncv1.ResourceFilter, syncFn func(string) error, resourceType string, targetNamespace string) ([]error, error) {
	log := log.FromContext(ctx)
	var conflicts []error
	for _, name := range names {
		if filter != nil && !r.shouldSyncResource(name, filter) {
			continue
		}
		switch err := syncFn(name); {
		case errors.Is(err, errSyncConflict):
			log.Info("Skipping resource owned by another NamespaceSync", "resourceType", resourceType, "name", name, "targetNamespace", targetNamespace, "reason", err.Error())
			conflicts = append(conflicts, err)
		case err != nil:
			log.Error(err, "Failed to sync resource", "resourceType", resourceType, "name", name, "targetNamespace", targetNamespace)
			return conflicts, err
		default:
			log.Info("Successfully synced resource", "resourceType", resourceType, "name", name, "targetNamespace", targetNamespace)
		}
	}
	return conflicts, nil
}

// syncResources syncs the listed resources into targetNamespace. peers are the other live NamespaceSyncs,
// whose objects a sync must not overwrite. It returns the conflicts it skipped and the first other error.
func (r *NamespaceSyncReconciler) syncResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string, peers []syncv1.NamespaceSync) ([]error, error) {
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
	source := namespaceSync.Spec.SourceNamespace
	explicit := contains(namespaceSync.Spec.TargetNamespaces, targetNamespace)

	conflicts, err := r.syncResourceList(ctx, namespaceSync.Spec.SecretName, secretFilter, func(name string) error {
		return r.syncSecret(ctx, source, targetNamespace, name, peers, explicit)
	}, "secret", targetNamespace)
	if err != nil {
		return conflicts, err
	}

	configMapConflicts, err := r.syncResourceList(ctx, namespaceSync.Spec.ConfigMapName, configMapFilter, func(name string) error {
		return r.syncConfigMap(ctx, source, targetNamespace, name, peers, explicit)
	}, "configmap", targetNamespace)
	return append(conflicts, configMapConflicts...), err
}

// syncSecret synchronizes a single secret to the target namespace. explicit reports whether the target is
// listed in targetNamespaces rather than reached by default.
func (r *NamespaceSyncReconciler) syncSecret(ctx context.Context, sourceNamespace, targetNamespace, secretName string, peers []syncv1.NamespaceSync, explicit bool) error {
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: sourceNamespace,
		Name:      secretName,
	}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return r.removeStaleCopy(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: targetNamespace},
			}, sourceNamespace, "secret", peers, explicit)
		}
		return err
	}

	if !explicit {
		if err := r.sourceConflict(peers, "secret", targetNamespace, secretName); err != nil {
			recordSyncFailure(targetNamespace, "secret")
			return err
		}
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
		r.copyConflictGuard(peers, "secret", sourceNamespace, explicit),
		func(src, dst *corev1.Secret) {
			dst.Data = src.Data
			dst.StringData = src.StringData
			dst.Type = src.Type
			dst.Labels = src.Labels
			dst.Annotations = src.Annotations
		})
}

// syncConfigMap synchronizes a single configmap to the target namespace. explicit reports whether the target
// is listed in targetNamespaces rather than reached by default.
func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, sourceNamespace, targetNamespace, configMapName string, peers []syncv1.NamespaceSync, explicit bool) error {
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: sourceNamespace,
		Name:      configMapName,
	}, &configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return r.removeStaleCopy(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: targetNamespace},
			}, sourceNamespace, "configmap", peers, explicit)
		}
		return err
	}

	if !explicit {
		if err := r.sourceConflict(peers, "configmap", targetNamespace, configMapName); err != nil {
			recordSyncFailure(targetNamespace, "configmap")
			return err
		}
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
		r.copyConflictGuard(peers, "configmap", sourceNamespace, explicit),
		func(src, dst *corev1.ConfigMap) {
			dst.Data = src.Data
			dst.BinaryData = src.BinaryData
			dst.Labels = src.Labels
			dst.Annotations = src.Annotations
		})
}

// removeStaleCopy deletes the copy of a source object that no longer exists. Outside an explicit target it
// leaves an object a peer reads as its source alone, and it reports no conflict, since nothing is written.
func (r *NamespaceSyncReconciler) removeStaleCopy(ctx context.Context, obj client.Object, sourceNamespace, resourceType string, peers []syncv1.NamespaceSync, explicit bool) error {
	if !explicit && r.sourceOwner(peers, resourceType, obj.GetNamespace(), obj.GetName()) != nil {
		return nil
	}
	deleted, err := r.deleteManagedCopy(ctx, obj, sourceNamespace)
	if err != nil {
		return err
	}
	if deleted {
		log.FromContext(ctx).Info("Deleted "+resourceType+" from target namespace as it was deleted from source",
			resourceType, obj.GetName(),
			"targetNamespace", obj.GetNamespace())
	}
	return nil
}
