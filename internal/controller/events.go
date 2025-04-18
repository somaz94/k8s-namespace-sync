package controller

import (
	"context"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// findNamespaceSyncs handles namespace events and returns related NamespaceSync requests
func (r *NamespaceSyncReconciler) findNamespaceSyncs(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)
	namespace := obj.(*corev1.Namespace)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		log.Error(err, "Failed to list NamespaceSyncs")
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		// 1. 소스 네임스페이스가 변경된 경우
		if namespace.Name == ns.Spec.SourceNamespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
			log.V(1).Info("Queuing reconcile for NamespaceSync due to source namespace change",
				"namespacesync", ns.Name,
				"namespace", namespace.Name)
		}

		// 2. 대상 네임스페이스가 생성/수정된 경우
		if !r.shouldSkipNamespace(namespace.Name, ns.Spec.SourceNamespace, ns.Spec.Exclude) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
			log.V(1).Info("Queuing reconcile for NamespaceSync due to target namespace change",
				"namespacesync", ns.Name,
				"namespace", namespace.Name)
		}
	}

	return requests
}

// findNamespaceSyncsForSecret handles Secret events and returns related NamespaceSync requests
func (r *NamespaceSyncReconciler) findNamespaceSyncsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)
	secret := obj.(*corev1.Secret)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		log.Error(err, "Failed to list NamespaceSyncs")
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		// Check if this secret is from source namespace or target namespaces
		isSourceSecret := ns.Spec.SourceNamespace == secret.Namespace
		isTargetSecret := r.shouldSyncToNamespace(secret.Namespace, &ns)

		// Trigger reconciliation for both source and target namespace events
		if (isSourceSecret && contains(ns.Spec.SecretName, secret.Name)) ||
			(isTargetSecret && contains(ns.Spec.SecretName, secret.Name)) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
			log.V(1).Info("Queuing reconcile for NamespaceSync due to Secret change",
				"namespacesync", ns.Name,
				"secret", secret.Name,
				"namespace", secret.Namespace)
		}
	}
	return requests
}

// findNamespaceSyncsForConfigMap handles ConfigMap events and returns related NamespaceSync requests
func (r *NamespaceSyncReconciler) findNamespaceSyncsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)
	configMap := obj.(*corev1.ConfigMap)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		log.Error(err, "Failed to list NamespaceSyncs")
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		// Check if this configmap is from source namespace or target namespaces
		isSourceConfigMap := ns.Spec.SourceNamespace == configMap.Namespace
		isTargetConfigMap := r.shouldSyncToNamespace(configMap.Namespace, &ns)

		// Trigger reconciliation for both source and target namespace events
		if (isSourceConfigMap && contains(ns.Spec.ConfigMapName, configMap.Name)) ||
			(isTargetConfigMap && contains(ns.Spec.ConfigMapName, configMap.Name)) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
			log.V(1).Info("Queuing reconcile for NamespaceSync due to ConfigMap change",
				"namespacesync", ns.Name,
				"configmap", configMap.Name,
				"namespace", configMap.Namespace)
		}
	}
	return requests
}
