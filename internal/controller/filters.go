package controller

import (
	"context"
	"path"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// shouldSyncResource determines if a resource should be synced based on its name and filter rules
func (r *NamespaceSyncReconciler) shouldSyncResource(name string, filter *syncv1.ResourceFilter) bool {
	if filter == nil {
		return true
	}

	// Check exclude patterns first; a malformed one excludes, so a typo never leaks a resource.
	for _, pattern := range filter.Exclude {
		matched, err := path.Match(pattern, name)
		if err != nil || matched {
			return false
		}
	}

	if len(filter.Include) == 0 {
		return true
	}

	for _, pattern := range filter.Include {
		matched, err := path.Match(pattern, name)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// shouldSyncToNamespace determines if resources should be synced to the given namespace
func (r *NamespaceSyncReconciler) shouldSyncToNamespace(ctx context.Context, namespace string, namespaceSync *syncv1.NamespaceSync) bool {
	shouldSync := r.targetsNamespace(namespace, namespaceSync)
	log.FromContext(ctx).V(1).Info("Evaluated target namespace", "namespace", namespace, "shouldSync", shouldSync)
	return shouldSync
}

// targetsNamespace is shouldSyncToNamespace without logging. Conflict checks judge peers with it, so a
// peer's ownership always matches the targets that peer syncs itself.
func (r *NamespaceSyncReconciler) targetsNamespace(namespace string, namespaceSync *syncv1.NamespaceSync) bool {
	switch {
	case r.isSystemNamespace(namespace),
		namespace == namespaceSync.Spec.SourceNamespace,
		contains(namespaceSync.Spec.Exclude, namespace):
		return false
	case len(namespaceSync.Spec.TargetNamespaces) > 0:
		return contains(namespaceSync.Spec.TargetNamespaces, namespace)
	default:
		return true
	}
}

// isSystemNamespace checks if the namespace is a system namespace
func (r *NamespaceSyncReconciler) isSystemNamespace(namespace string) bool {
	systemNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"default",
		"k8s-namespace-sync-system",
	}
	return contains(systemNamespaces, namespace)
}
