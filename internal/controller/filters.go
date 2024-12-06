package controller

import (
	"path/filepath"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
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
	// 시스템 네임스페이스 체크
	if r.isSystemNamespace(namespace) {
		return false
	}

	// 소스 네임스페이스와 동일한 경우 스킵
	if namespace == namespaceSync.Spec.SourceNamespace {
		return false
	}

	// 제외 목록에 있는 경우 스킵
	if contains(namespaceSync.Spec.Exclude, namespace) {
		return false
	}

	// targetNamespaces가 지정된 경우, 해당 목록에 있는 네임스페이스만 동기화
	if len(namespaceSync.Spec.TargetNamespaces) > 0 {
		return contains(namespaceSync.Spec.TargetNamespaces, namespace)
	}

	// targetNamespaces가 지정되지 않은 경우 모든 네임스페이스에 동기화
	return true
}

// isSystemNamespace checks if the namespace is a system namespace
func (r *NamespaceSyncReconciler) isSystemNamespace(namespace string) bool {
	systemNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"default",
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
