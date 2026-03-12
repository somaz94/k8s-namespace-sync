package controller

import (
	"context"
	"path/filepath"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// shouldSyncResource determines if a resource should be synced based on its name and filter rules
func (r *NamespaceSyncReconciler) shouldSyncResource(name string, filter *syncv1.ResourceFilter) bool {
	if filter == nil {
		return true
	}

	// Check exclude patterns first
	for _, pattern := range filter.Exclude {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return false
		}
	}

	// If there are no include patterns, include all resources
	if len(filter.Include) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range filter.Include {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// shouldSyncToNamespace determines if resources should be synced to the given namespace
func (r *NamespaceSyncReconciler) shouldSyncToNamespace(namespace string, namespaceSync *syncv1.NamespaceSync) bool {
	logger := log.FromContext(context.Background()).WithValues("namespace", namespace)

	// Check if it is a system namespace
	if r.isSystemNamespace(namespace) {
		logger.Info("Namespace is system namespace, skipping sync")
		return false
	}

	// Skip if it is the same as the source namespace
	if namespace == namespaceSync.Spec.SourceNamespace {
		logger.Info("Namespace is source namespace, skipping sync")
		return false
	}

	// Skip if the namespace is in the exclude list
	if contains(namespaceSync.Spec.Exclude, namespace) {
		logger.Info("Namespace is in exclude list, skipping sync")
		return false
	}

	// If targetNamespaces is specified, only sync to namespaces in that list
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

// shouldSkipNamespace checks if the namespace should be skipped for synchronization
func (r *NamespaceSyncReconciler) shouldSkipNamespace(namespace, sourceNamespace string, excludedNamespaces []string) bool {
	// Check if it is a system namespace
	if r.isSystemNamespace(namespace) {
		return true
	}

	// Skip if it is the same as the source namespace
	if namespace == sourceNamespace {
		return true
	}

	// Check if the namespace is in the excluded namespaces list
	return contains(excludedNamespaces, namespace)
}
