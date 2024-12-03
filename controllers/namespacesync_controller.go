package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

// NamespaceSyncReconciler reconciles a NamespaceSync object
type NamespaceSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NamespaceSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Starting reconciliation", "request", req)

	// Get the NamespaceSync resource
	var namespaceSync syncv1.NamespaceSync
	if err := r.Get(ctx, req.NamespacedName, &namespaceSync); err != nil {
		log.Error(err, "Unable to fetch NamespaceSync")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info("Found NamespaceSync", "spec", namespaceSync.Spec)

	// Get all namespaces
	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Unable to list namespaces")
		return ctrl.Result{}, err
	}

	// Process each namespace
	for _, namespace := range namespaceList.Items {
		// Skip system namespaces
		if namespace.Name == "kube-system" ||
			namespace.Name == "kube-public" ||
			namespace.Name == "kube-node-lease" {
			continue
		}

		log.Info("Processing namespace", "namespace", namespace.Name)
		if err := r.syncResources(ctx, &namespaceSync, namespace.Name); err != nil {
			log.Error(err, "Failed to sync resources", "namespace", namespace.Name)
			continue
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceSyncReconciler) syncResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("Syncing resources",
		"namespaceSync", namespaceSync.Name,
		"targetNamespace", targetNamespace,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace)

	// Skip source namespace
	if targetNamespace == namespaceSync.Spec.SourceNamespace {
		log.Info("Skipping source namespace")
		return nil
	}

	// Sync Secret if specified
	if namespaceSync.Spec.SecretName != "" {
		log.Info("Attempting to sync secret",
			"secretName", namespaceSync.Spec.SecretName,
			"from", namespaceSync.Spec.SourceNamespace,
			"to", targetNamespace)
		if err := r.syncSecret(ctx, namespaceSync, targetNamespace); err != nil {
			log.Error(err, "Failed to sync secret")
			return fmt.Errorf("failed to sync secret: %w", err)
		}
		log.Info("Successfully synced secret")
	}

	// Sync ConfigMap if specified
	if namespaceSync.Spec.ConfigMapName != "" {
		log.Info("Attempting to sync configmap",
			"configMapName", namespaceSync.Spec.ConfigMapName,
			"from", namespaceSync.Spec.SourceNamespace,
			"to", targetNamespace)
		if err := r.syncConfigMap(ctx, namespaceSync, targetNamespace); err != nil {
			log.Error(err, "Failed to sync configmap")
			return fmt.Errorf("failed to sync configmap: %w", err)
		}
		log.Info("Successfully synced configmap")
	}

	return nil
}

func (r *NamespaceSyncReconciler) syncSecret(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceSync.Spec.SourceNamespace,
		Name:      namespaceSync.Spec.SecretName,
	}, &secret); err != nil {
		return err
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: targetNamespace,
		},
		Type:       secret.Type,
		Data:       secret.Data,
		StringData: secret.StringData,
	}

	err := r.Create(ctx, newSecret)
	if errors.IsAlreadyExists(err) {
		return r.Update(ctx, newSecret)
	}
	return err
}

func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceSync.Spec.SourceNamespace,
		Name:      namespaceSync.Spec.ConfigMapName,
	}, &configMap); err != nil {
		return err
	}

	newConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMap.Name,
			Namespace: targetNamespace,
		},
		Data:       configMap.Data,
		BinaryData: configMap.BinaryData,
	}

	err := r.Create(ctx, newConfigMap)
	if errors.IsAlreadyExists(err) {
		return r.Update(ctx, newConfigMap)
	}
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syncv1.NamespaceSync{}).
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(handler.MapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				log := log.FromContext(ctx)
				log.Info("Namespace event detected", "namespace", obj.GetName())

				var namespaceSyncList syncv1.NamespaceSyncList
				if err := r.List(ctx, &namespaceSyncList); err != nil {
					log.Error(err, "Failed to list NamespaceSync resources")
					return nil
				}

				var requests []reconcile.Request
				for _, ns := range namespaceSyncList.Items {
					log.Info("Creating reconcile request", "namespacesync", ns.Name)
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: ns.Name,
						},
					})
				}
				return requests
			})),
		).
		Complete(r)
}
