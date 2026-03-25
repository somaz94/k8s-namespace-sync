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
		// 1. If the source namespace was changed
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

		// 2. If a target namespace was created/modified
		if r.shouldSyncToNamespace(ctx, namespace.Name, &ns) {
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

// findNamespaceSyncsForResource is a generic handler that finds NamespaceSyncs
// related to a resource change. getNames extracts the relevant resource name list
// from a NamespaceSync spec.
func (r *NamespaceSyncReconciler) findNamespaceSyncsForResource(
	ctx context.Context,
	resourceNamespace string,
	resourceName string,
	resourceType string,
	getNames func(*syncv1.NamespaceSync) []string,
) []reconcile.Request {
	log := log.FromContext(ctx)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		log.Error(err, "Failed to list NamespaceSyncs")
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		isSource := ns.Spec.SourceNamespace == resourceNamespace
		isTarget := r.shouldSyncToNamespace(ctx, resourceNamespace, &ns)

		if (isSource || isTarget) && contains(getNames(&ns), resourceName) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
			log.V(1).Info("Queuing reconcile for NamespaceSync due to "+resourceType+" change",
				"namespacesync", ns.Name,
				resourceType, resourceName,
				"namespace", resourceNamespace)
		}
	}
	return requests
}

// findNamespaceSyncsForSecret handles Secret events and returns related NamespaceSync requests
func (r *NamespaceSyncReconciler) findNamespaceSyncsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	return r.findNamespaceSyncsForResource(ctx, secret.Namespace, secret.Name, "Secret",
		func(ns *syncv1.NamespaceSync) []string { return ns.Spec.SecretName })
}

// findNamespaceSyncsForConfigMap handles ConfigMap events and returns related NamespaceSync requests
func (r *NamespaceSyncReconciler) findNamespaceSyncsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm := obj.(*corev1.ConfigMap)
	return r.findNamespaceSyncsForResource(ctx, cm.Namespace, cm.Name, "ConfigMap",
		func(ns *syncv1.NamespaceSync) []string { return ns.Spec.ConfigMapName })
}
