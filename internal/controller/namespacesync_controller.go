/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// NamespaceSyncReconciler reconciles a NamespaceSync object
type NamespaceSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
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

	// List all namespaces
	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Unable to list namespaces")
		return ctrl.Result{}, err
	}
	log.Info("Found namespaces", "count", len(namespaceList.Items))

	// Process each namespace
	for _, namespace := range namespaceList.Items {
		log.Info("Processing namespace", "namespace", namespace.Name)
		if err := r.syncResources(ctx, &namespaceSync, namespace.Name); err != nil {
			log.Error(err, "Failed to sync resources", "namespace", namespace.Name)
			continue
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syncv1.NamespaceSync{}).
		Named("namespacesync").
		Complete(r)
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
		if err := r.syncSecret(ctx, namespaceSync, targetNamespace); err != nil {
			return fmt.Errorf("failed to sync secret: %w", err)
		}
	}

	// Sync ConfigMap if specified
	if namespaceSync.Spec.ConfigMapName != "" {
		if err := r.syncConfigMap(ctx, namespaceSync, targetNamespace); err != nil {
			return fmt.Errorf("failed to sync configmap: %w", err)
		}
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
