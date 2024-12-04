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
	"strings"
	"time"

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

//+kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs/finalizers,verbs=update
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="authentication.k8s.io",resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups="authorization.k8s.io",resources=subjectaccessreviews,verbs=create

// NamespaceSyncReconciler reconciles a NamespaceSync object
type NamespaceSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NamespaceSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Starting reconciliation",
		"controller", "NamespaceSync",
		"namespace", req.Namespace,
		"name", req.Name)

	// Get the NamespaceSync resource
	var namespaceSync syncv1.NamespaceSync
	if err := r.Get(ctx, req.NamespacedName, &namespaceSync); err != nil {
		if errors.IsNotFound(err) {
			log.Info("NamespaceSync resource not found", "request", req)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch NamespaceSync")
		return ctrl.Result{}, err
	}
	log.Info("Processing NamespaceSync",
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"secretName", namespaceSync.Spec.SecretName,
		"configMapName", namespaceSync.Spec.ConfigMapName)

	// List all namespaces
	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Unable to list namespaces")
		return ctrl.Result{}, err
	}
	log.Info("Found namespaces for sync", "count", len(namespaceList.Items))

	var syncErrors []error
	for _, namespace := range namespaceList.Items {
		if r.shouldSkipNamespace(namespace.Name, namespaceSync.Spec.SourceNamespace) {
			log.Info("Skipping system namespace",
				"namespace", namespace.Name,
				"reason", "system namespace or source namespace")
			continue
		}

		log.Info("Processing namespace",
			"namespace", namespace.Name,
			"sourceNamespace", namespaceSync.Spec.SourceNamespace)

		if err := r.syncResources(ctx, &namespaceSync, namespace.Name); err != nil {
			log.Error(err, "Failed to sync resources",
				"namespace", namespace.Name,
				"error", err.Error())
			syncErrors = append(syncErrors, fmt.Errorf("namespace %s: %w", namespace.Name, err))
		} else {
			log.Info("Successfully synced resources",
				"namespace", namespace.Name,
				"sourceNamespace", namespaceSync.Spec.SourceNamespace)
		}
	}

	if len(syncErrors) > 0 {
		log.Error(fmt.Errorf("sync errors occurred"),
			"errorCount", len(syncErrors),
			"errors", fmt.Sprintf("%v", syncErrors))
		return ctrl.Result{RequeueAfter: time.Second * 30}, fmt.Errorf("sync errors: %v", syncErrors)
	}

	log.Info("Reconciliation completed successfully",
		"nextReconciliation", time.Now().Add(time.Minute))
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syncv1.NamespaceSync{}).
		// Watch Namespaces
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				log := log.FromContext(ctx)
				log.Info("Namespace event detected",
					"namespace", obj.GetName(),
					"event", "watch")

				var namespaceSyncList syncv1.NamespaceSyncList
				if err := r.List(ctx, &namespaceSyncList); err != nil {
					log.Error(err, "Failed to list NamespaceSync resources")
					return nil
				}
				log.Info("Found NamespaceSync resources", "count", len(namespaceSyncList.Items))

				var requests []reconcile.Request
				for _, ns := range namespaceSyncList.Items {
					log.Info("Queuing reconciliation request",
						"namespacesync", ns.Name,
						"namespace", ns.Namespace)
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      ns.Name,
							Namespace: ns.Namespace,
						},
					})
				}
				return requests
			}),
		).
		// Watch Secrets with similar logging
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				log := log.FromContext(ctx)
				log.Info("Secret event detected",
					"namespace", obj.GetNamespace(),
					"name", obj.GetName())

				var namespaceSyncList syncv1.NamespaceSyncList
				if err := r.List(ctx, &namespaceSyncList); err != nil {
					log.Error(err, "Failed to list NamespaceSync resources")
					return nil
				}

				var requests []reconcile.Request
				for _, ns := range namespaceSyncList.Items {
					if ns.Spec.SecretName == obj.GetName() &&
						ns.Spec.SourceNamespace == obj.GetNamespace() {
						log.Info("Queuing reconciliation for Secret change",
							"namespacesync", ns.Name,
							"namespace", ns.Namespace)
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      ns.Name,
								Namespace: ns.Namespace,
							},
						})
					}
				}
				return requests
			}),
		).
		// Watch ConfigMaps with similar logging
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				log := log.FromContext(ctx)
				log.Info("ConfigMap event detected",
					"namespace", obj.GetNamespace(),
					"name", obj.GetName())

				var namespaceSyncList syncv1.NamespaceSyncList
				if err := r.List(ctx, &namespaceSyncList); err != nil {
					log.Error(err, "Failed to list NamespaceSync resources")
					return nil
				}

				var requests []reconcile.Request
				for _, ns := range namespaceSyncList.Items {
					if ns.Spec.ConfigMapName == obj.GetName() &&
						ns.Spec.SourceNamespace == obj.GetNamespace() {
						log.Info("Queuing reconciliation for ConfigMap change",
							"namespacesync", ns.Name,
							"namespace", ns.Namespace)
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      ns.Name,
								Namespace: ns.Namespace,
							},
						})
					}
				}
				return requests
			}),
		).
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
	log := log.FromContext(ctx)
	log.Info("Starting Secret sync",
		"secret", namespaceSync.Spec.SecretName,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"targetNamespace", targetNamespace)

	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceSync.Spec.SourceNamespace,
		Name:      namespaceSync.Spec.SecretName,
	}, &secret); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Secret not found in source namespace",
				"secret", namespaceSync.Spec.SecretName,
				"namespace", namespaceSync.Spec.SourceNamespace)
			return nil
		}
		log.Error(err, "Failed to get source Secret",
			"secret", namespaceSync.Spec.SecretName,
			"namespace", namespaceSync.Spec.SourceNamespace)
		return err
	}
	log.Info("Found source Secret",
		"name", secret.Name,
		"namespace", secret.Namespace,
		"type", secret.Type,
		"labels", secret.Labels,
		"annotations", secret.Annotations)

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: targetNamespace,
		},
		Type:       secret.Type,
		Data:       secret.Data,
		StringData: secret.StringData,
	}

	// 메타데이터 복사
	r.copyLabelsAndAnnotations(&secret.ObjectMeta, &newSecret.ObjectMeta)
	log.Info("Prepared new Secret with metadata",
		"name", newSecret.Name,
		"namespace", newSecret.Namespace,
		"type", newSecret.Type,
		"labels", newSecret.Labels,
		"annotations", newSecret.Annotations)

	err := r.Create(ctx, newSecret)
	if errors.IsAlreadyExists(err) {
		log.Info("Secret already exists, fetching existing Secret",
			"namespace", targetNamespace,
			"name", secret.Name)

		// 기존 Secret 가져오기
		var existingSecret corev1.Secret
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: targetNamespace,
			Name:      secret.Name,
		}, &existingSecret); err != nil {
			log.Error(err, "Failed to get existing Secret",
				"namespace", targetNamespace,
				"name", secret.Name)
			return err
		}
		log.Info("Found existing Secret",
			"name", existingSecret.Name,
			"namespace", existingSecret.Namespace,
			"resourceVersion", existingSecret.ResourceVersion)

		// 메타데이터 유지하면서 업데이트
		newSecret.ResourceVersion = existingSecret.ResourceVersion
		log.Info("Updating existing Secret",
			"namespace", targetNamespace,
			"name", secret.Name,
			"newResourceVersion", newSecret.ResourceVersion)

		if err := r.Update(ctx, newSecret); err != nil {
			log.Error(err, "Failed to update Secret",
				"namespace", targetNamespace,
				"name", secret.Name)
			return err
		}
		log.Info("Successfully updated Secret",
			"namespace", targetNamespace,
			"name", secret.Name,
			"type", newSecret.Type)
		return nil
	}
	if err != nil {
		log.Error(err, "Failed to create Secret",
			"namespace", targetNamespace,
			"name", secret.Name)
		return err
	}

	log.Info("Successfully created Secret",
		"namespace", targetNamespace,
		"name", secret.Name,
		"type", newSecret.Type)
	return nil
}

func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("Starting ConfigMap sync",
		"configMap", namespaceSync.Spec.ConfigMapName,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"targetNamespace", targetNamespace)

	var configMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceSync.Spec.SourceNamespace,
		Name:      namespaceSync.Spec.ConfigMapName,
	}, &configMap); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ConfigMap not found in source namespace",
				"configMap", namespaceSync.Spec.ConfigMapName,
				"namespace", namespaceSync.Spec.SourceNamespace)
			return nil
		}
		log.Error(err, "Failed to get source ConfigMap",
			"configMap", namespaceSync.Spec.ConfigMapName,
			"namespace", namespaceSync.Spec.SourceNamespace)
		return err
	}
	log.Info("Found source ConfigMap",
		"name", configMap.Name,
		"namespace", configMap.Namespace,
		"labels", configMap.Labels,
		"annotations", configMap.Annotations)

	newConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMap.Name,
			Namespace: targetNamespace,
		},
		Data:       configMap.Data,
		BinaryData: configMap.BinaryData,
	}

	// 메타데이터 복사
	r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &newConfigMap.ObjectMeta)
	log.Info("Prepared new ConfigMap with metadata",
		"name", newConfigMap.Name,
		"namespace", newConfigMap.Namespace,
		"labels", newConfigMap.Labels,
		"annotations", newConfigMap.Annotations)

	err := r.Create(ctx, newConfigMap)
	if errors.IsAlreadyExists(err) {
		log.Info("ConfigMap already exists, fetching existing ConfigMap",
			"namespace", targetNamespace,
			"name", configMap.Name)

		// 기존 ConfigMap 가져오기
		var existingConfigMap corev1.ConfigMap
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: targetNamespace,
			Name:      configMap.Name,
		}, &existingConfigMap); err != nil {
			log.Error(err, "Failed to get existing ConfigMap",
				"namespace", targetNamespace,
				"name", configMap.Name)
			return err
		}
		log.Info("Found existing ConfigMap",
			"name", existingConfigMap.Name,
			"namespace", existingConfigMap.Namespace,
			"resourceVersion", existingConfigMap.ResourceVersion)

		// 메타데이터 유지하면서 업데이트
		newConfigMap.ResourceVersion = existingConfigMap.ResourceVersion
		log.Info("Updating existing ConfigMap",
			"namespace", targetNamespace,
			"name", configMap.Name,
			"newResourceVersion", newConfigMap.ResourceVersion)

		if err := r.Update(ctx, newConfigMap); err != nil {
			log.Error(err, "Failed to update ConfigMap",
				"namespace", targetNamespace,
				"name", configMap.Name)
			return err
		}
		log.Info("Successfully updated ConfigMap",
			"namespace", targetNamespace,
			"name", configMap.Name)
		return nil
	}
	if err != nil {
		log.Error(err, "Failed to create ConfigMap",
			"namespace", targetNamespace,
			"name", configMap.Name)
		return err
	}

	log.Info("Successfully created ConfigMap",
		"namespace", targetNamespace,
		"name", configMap.Name)
	return nil
}

func (r *NamespaceSyncReconciler) shouldSkipNamespace(namespace, sourceNamespace string) bool {
	var systemNamespaces = map[string]bool{
		"kube-system":               true,
		"kube-public":               true,
		"kube-node-lease":           true,
		"k8s-namespace-sync-system": true,
	}
	return systemNamespaces[namespace] || namespace == sourceNamespace
}

func (r *NamespaceSyncReconciler) copyLabelsAndAnnotations(src, dst *metav1.ObjectMeta) {
	if dst.Labels == nil {
		dst.Labels = make(map[string]string)
	}
	if dst.Annotations == nil {
		dst.Annotations = make(map[string]string)
	}

	// 소스에서 메타데이터 복사
	for k, v := range src.Labels {
		if !strings.HasPrefix(k, "kubernetes.io/") {
			dst.Labels[k] = v
		}
	}
	for k, v := range src.Annotations {
		if !strings.HasPrefix(k, "kubernetes.io/") {
			dst.Annotations[k] = v
		}
	}

	// 소스 추적 정보 추가
	dst.Annotations["namespacesync.nsync.dev/source-namespace"] = src.Namespace
	dst.Annotations["namespacesync.nsync.dev/source-name"] = src.Name
	dst.Annotations["namespacesync.nsync.dev/last-sync"] = time.Now().Format(time.RFC3339)
}
