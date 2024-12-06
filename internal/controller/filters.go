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

	// 먼저 exclude 패턴 체크
	for _, pattern := range filter.Exclude {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return false
		}
	}

	// include 패턴이 없으면 모든 리소스 포함
	if len(filter.Include) == 0 {
		return true
	}

	// include 패턴 체크
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

	// 시스템 네임스페이스 체크
	if r.isSystemNamespace(namespace) {
		logger.Info("Namespace is system namespace, skipping sync")
		return false
	}

	// 소스 네임스페이스와 동일한 경우 스킵
	if namespace == namespaceSync.Spec.SourceNamespace {
		logger.Info("Namespace is source namespace, skipping sync")
		return false
	}

	// 제외 목록에 있는 경우 스킵
	if contains(namespaceSync.Spec.Exclude, namespace) {
		logger.Info("Namespace is in exclude list, skipping sync")
		return false
	}

	// targetNamespaces가 지정된 경우, 해당 목록에 있는 네임스페이스만 동기화
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
	// 시스템 네임스페이스 체크
	if r.isSystemNamespace(namespace) {
		return true
	}

	// 소스 네임스페이스와 동일한 경우 스킵
	if namespace == sourceNamespace {
		return true
	}

	// 제외된 네임스페이스 목록에 포함되어 있는지 체크
	return contains(excludedNamespaces, namespace)
}
