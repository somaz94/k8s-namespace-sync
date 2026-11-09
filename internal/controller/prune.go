package controller

import (
	"context"
	"fmt"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// pruneOrphanedCopies deletes copies synced from namespaceSync's source that it
// no longer wants (a name dropped from the spec, a namespace newly excluded or
// left out of targetNamespaces, a tightened filter) and that no peer claims.
// Name-based cleanup only reaches what the current spec lists, so without this
// every such copy would stay behind for good. When deleting is set, nothing is
// wanted any more.
func (r *NamespaceSyncReconciler) pruneOrphanedCopies(
	ctx context.Context, namespaceSync *syncv1.NamespaceSync, peers []syncv1.NamespaceSync, deleting bool,
) []error {
	source := namespaceSync.Spec.SourceNamespace
	wanted := func(resourceType, namespace, name string) bool {
		return !deleting &&
			r.targetsNamespace(namespace, namespaceSync) &&
			r.syncsName(namespaceSync, resourceType, name)
	}

	var errs []error
	prune := func(resourceType string, items []metav1.ObjectMeta, stub func(name, namespace string) client.Object) {
		for _, meta := range items {
			if meta.Namespace == source || meta.Annotations[AnnotationSourceNamespace] != source ||
				meta.Annotations[AnnotationSourceName] != meta.Name ||
				wanted(resourceType, meta.Namespace, meta.Name) ||
				len(r.unclaimed(ctx, peers, resourceType, source, meta.Namespace, []string{meta.Name})) == 0 {
				continue
			}
			errs = append(errs, r.cleanupResource(ctx, source, meta.Namespace, []string{meta.Name}, resourceType, stub)...)
		}
	}

	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets, client.UnsafeDisableDeepCopy); err != nil {
		return append(errs, fmt.Errorf("list secrets: %w", err))
	}
	secretMeta := make([]metav1.ObjectMeta, len(secrets.Items))
	for i := range secrets.Items {
		secretMeta[i] = secrets.Items[i].ObjectMeta
	}
	prune("secret", secretMeta, func(name, namespace string) client.Object {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	})

	var configMaps corev1.ConfigMapList
	if err := r.List(ctx, &configMaps, client.UnsafeDisableDeepCopy); err != nil {
		return append(errs, fmt.Errorf("list configmaps: %w", err))
	}
	configMapMeta := make([]metav1.ObjectMeta, len(configMaps.Items))
	for i := range configMaps.Items {
		configMapMeta[i] = configMaps.Items[i].ObjectMeta
	}
	prune("configmap", configMapMeta, func(name, namespace string) client.Object {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	})

	if len(errs) > 0 {
		log.FromContext(ctx).Error(errs[0], "Failed to prune orphaned copies", "failures", len(errs))
	}
	return errs
}
