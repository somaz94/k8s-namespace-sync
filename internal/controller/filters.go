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
	logger := log.FromContext(ctx).WithValues("namespace", namespace)

	if r.isSystemNamespace(namespace) {
		logger.Info("Namespace is system namespace, skipping sync")
		return false
	}

	if namespace == namespaceSync.Spec.SourceNamespace {
		logger.Info("Namespace is source namespace, skipping sync")
		return false
	}

	if contains(namespaceSync.Spec.Exclude, namespace) {
		logger.Info("Namespace is in exclude list, skipping sync")
		return false
	}

	if len(namespaceSync.Spec.TargetNamespaces) > 0 {
		shouldSync := contains(namespaceSync.Spec.TargetNamespaces, namespace)
		logger.Info("Checking target namespaces", "shouldSync", shouldSync)
		return shouldSync
	}

	logger.Info("Namespace will be synced")
	return true
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
