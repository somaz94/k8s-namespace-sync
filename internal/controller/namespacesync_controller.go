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

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
)

var (
	syncSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_success_total",
			Help: "Number of successful resource synchronizations",
		},
		[]string{"namespace", "resource_type"},
	)

	syncFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_sync_failure_total",
			Help: "Number of failed resource synchronizations",
		},
		[]string{"namespace", "resource_type"},
	)
)

func init() {
	metrics.Registry.MustRegister(syncSuccessCounter, syncFailureCounter)
}

//+kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=sync.nsync.dev,resources=namespacesyncs/finalizers,verbs=update
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="authentication.k8s.io",resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups="authorization.k8s.io",resources=subjectaccessreviews,verbs=create

// NamespaceSyncReconciler reconciles a NamespaceSync object
type NamespaceSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// 상수 정의 부분에 다시 추가
const finalizerName = "namespacesync.nsync.dev/finalizer"

func (r *NamespaceSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconcile function called",
		"request_name", req.Name,
		"request_namespace", req.Namespace)

	// Get the NamespaceSync resource
	var namespaceSync syncv1.NamespaceSync
	if err := r.Get(ctx, req.NamespacedName, &namespaceSync); err != nil {
		if errors.IsNotFound(err) {
			log.Info("NamespaceSync resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get NamespaceSync")
		return ctrl.Result{}, err
	}

	log.Info("Successfully retrieved NamespaceSync resource",
		"spec", namespaceSync.Spec,
		"status", namespaceSync.Status)

	// 모든 네임스페이스 목록 가져오기
	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Failed to list namespaces")
		return ctrl.Result{}, err
	}

	var syncErrors []error
	var syncedNamespaces []string
	failedNamespaces := make(map[string]string)

	// 각 네임스페이스에 대해 동기화 수행
	for _, ns := range namespaceList.Items {
		if r.shouldSkipNamespace(ns.Name, namespaceSync.Spec.SourceNamespace) {
			continue
		}

		if err := r.syncResources(ctx, &namespaceSync, ns.Name); err != nil {
			errMsg := fmt.Sprintf("failed to sync resources: %v", err)
			failedNamespaces[ns.Name] = errMsg
			syncErrors = append(syncErrors, fmt.Errorf("namespace %s: %w", ns.Name, err))
			log.Error(err, "Failed to sync resources",
				"namespace", ns.Name)
		} else {
			syncedNamespaces = append(syncedNamespaces, ns.Name)
			log.Info("Successfully synced resources",
				"namespace", ns.Name)
		}
	}

	// Update status
	log.Info("Updating NamespaceSync status",
		"syncedNamespaces", syncedNamespaces,
		"failedNamespaces", failedNamespaces)

	if err := r.updateStatus(ctx, &namespaceSync, syncedNamespaces, failedNamespaces); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}

	log.Info("Successfully updated status")

	if len(syncErrors) > 0 {
		log.Error(fmt.Errorf("sync errors occurred"),
			"errorCount", len(syncErrors),
			"errors", fmt.Sprintf("%v", syncErrors))
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// 파이널라이저 관련 로직
	if !controllerutil.ContainsFinalizer(&namespaceSync, finalizerName) {
		controllerutil.AddFinalizer(&namespaceSync, finalizerName)
		if err := r.Update(ctx, &namespaceSync); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	} else if namespaceSync.DeletionTimestamp != nil {
		// 리소스가 삭제될 때 정리 작업 수행
		if err := r.cleanupSyncedResources(ctx, &namespaceSync); err != nil {
			log.Error(err, "Failed to cleanup synced resources")
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(&namespaceSync, finalizerName)
		if err := r.Update(ctx, &namespaceSync); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// shouldSkipNamespace checks if the namespace should be skipped for synchronization
func (r *NamespaceSyncReconciler) shouldSkipNamespace(namespace, sourceNamespace string) bool {
	// Skip if it's the source namespace
	if namespace == sourceNamespace {
		return true
	}

	// Skip if it's a system namespace
	if r.isSystemNamespace(namespace) {
		return true
	}

	return false
}

// Helper function to check if namespace is a system namespace
func (r *NamespaceSyncReconciler) isSystemNamespace(namespace string) bool {
	systemNamespaces := map[string]bool{
		"kube-system":               true,
		"kube-public":               true,
		"kube-node-lease":           true,
		"k8s-namespace-sync-system": true,
	}
	return systemNamespaces[namespace]
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := log.Log.WithName("namespacesync-controller")
	logger.Info("Setting up controller manager")

	// 네임스페이스 이벤트를 위한 인덱서 설정
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &syncv1.NamespaceSync{}, ".spec.sourceNamespace", func(rawObj client.Object) []string {
		namespaceSync := rawObj.(*syncv1.NamespaceSync)
		return []string{namespaceSync.Spec.SourceNamespace}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&syncv1.NamespaceSync{}).
		// 네임스페이스 변경 감시
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncs)).
		// Secret 변경 감시 추가
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncsForSecret)).
		// ConfigMap 변경 감시 추가
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncsForConfigMap)).
		Complete(r)
}

// Secret 변경시 관련된 NamespaceSync 찾기
func (r *NamespaceSyncReconciler) findNamespaceSyncsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		if ns.Spec.SecretName == secret.Name && ns.Spec.SourceNamespace == secret.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
		}
	}
	return requests
}

// ConfigMap 변경시 관련된 NamespaceSync 찾기
func (r *NamespaceSyncReconciler) findNamespaceSyncsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap := obj.(*corev1.ConfigMap)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		if ns.Spec.ConfigMapName == configMap.Name && ns.Spec.SourceNamespace == configMap.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ns.Name,
					Namespace: ns.Namespace,
				},
			})
		}
	}
	return requests
}

func (r *NamespaceSyncReconciler) updateStatus(ctx context.Context, namespaceSync *syncv1.NamespaceSync, syncedNamespaces []string, failedNamespaces map[string]string) error {
	log := log.FromContext(ctx)
	log.Info("Updating status",
		"resource", namespaceSync.Name,
		"namespace", namespaceSync.Namespace,
		"syncedCount", len(syncedNamespaces),
		"failedCount", len(failedNamespaces))

	// Create a deep copy to avoid modifying the cache
	namespaceSyncCopy := namespaceSync.DeepCopy()

	// Update status fields
	namespaceSyncCopy.Status.LastSyncTime = metav1.NewTime(time.Now())
	namespaceSyncCopy.Status.SyncedNamespaces = syncedNamespaces
	namespaceSyncCopy.Status.FailedNamespaces = failedNamespaces

	if err := r.Status().Update(ctx, namespaceSyncCopy); err != nil {
		log.Error(err, "Failed to update NamespaceSync status")
		return err
	}

	log.Info("Successfully updated status",
		"syncedNamespaces", syncedNamespaces,
		"failedNamespaces", failedNamespaces)

	return nil
}

func (r *NamespaceSyncReconciler) handleDeletion(ctx context.Context, ns *syncv1.NamespaceSync) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Handling deletion",
		"namespace", ns.Namespace,
		"name", ns.Name)

	if controllerutil.ContainsFinalizer(ns, finalizerName) {
		log.Info("Removing finalizer")

		// Perform cleanup logic here if needed

		// Remove finalizer
		controllerutil.RemoveFinalizer(ns, finalizerName)
		if err := r.Update(ctx, ns); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		log.Info("Successfully removed finalizer")
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceSyncReconciler) syncResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("syncResources called",
		"targetNamespace", targetNamespace,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"secretName", namespaceSync.Spec.SecretName,
		"configMapName", namespaceSync.Spec.ConfigMapName)

	// Sync Secret if specified
	if namespaceSync.Spec.SecretName != "" {
		if err := r.syncSecret(ctx, namespaceSync.Spec.SourceNamespace, targetNamespace, namespaceSync.Spec.SecretName); err != nil {
			log.Error(err, "Failed to sync secret",
				"secretName", namespaceSync.Spec.SecretName,
				"targetNamespace", targetNamespace)
			return err
		}
		log.Info("Successfully synced secret",
			"secretName", namespaceSync.Spec.SecretName,
			"targetNamespace", targetNamespace)
	}

	// Sync ConfigMap if specified
	if namespaceSync.Spec.ConfigMapName != "" {
		if err := r.syncConfigMap(ctx, namespaceSync, targetNamespace); err != nil {
			log.Error(err, "Failed to sync configmap",
				"configMapName", namespaceSync.Spec.ConfigMapName,
				"targetNamespace", targetNamespace)
			return err
		}
		log.Info("Successfully synced configmap",
			"configMapName", namespaceSync.Spec.ConfigMapName,
			"targetNamespace", targetNamespace)
	}

	return nil
}

func (r *NamespaceSyncReconciler) syncSecret(ctx context.Context, sourceNamespace, targetNamespace, secretName string) error {
	log := log.FromContext(ctx)

	// Get source secret
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: sourceNamespace, Name: secretName}, &secret); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Secret not found in source namespace",
				"secret", secretName,
				"namespace", sourceNamespace)
			return nil
		}
		log.Error(err, "Failed to get source Secret")
		return err
	}

	log.Info("Found source Secret",
		"name", secret.Name,
		"namespace", secret.Namespace,
		"type", secret.Type,
		"labels", secret.Labels,
		"annotations", secret.Annotations)

	// Check if secret exists in target namespace
	var existingSecret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Namespace: targetNamespace, Name: secretName}, &existingSecret)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get target secret")
			return err
		}
		// Create new secret if it doesn't exist
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: targetNamespace,
			},
			Type:       secret.Type,
			Data:       secret.Data,
			StringData: secret.StringData,
		}
		r.copyLabelsAndAnnotations(&secret.ObjectMeta, &newSecret.ObjectMeta)

		if err := r.Create(ctx, newSecret); err != nil {
			log.Error(err, "Failed to create Secret")
			return err
		}
		log.Info("Successfully created Secret",
			"namespace", targetNamespace,
			"name", secret.Name,
			"type", newSecret.Type)
	} else {
		// Update existing secret
		existingSecret.Data = secret.Data
		existingSecret.StringData = secret.StringData
		existingSecret.Type = secret.Type
		r.copyLabelsAndAnnotations(&secret.ObjectMeta, &existingSecret.ObjectMeta)

		if err := r.Update(ctx, &existingSecret); err != nil {
			log.Error(err, "Failed to update Secret")
			return err
		}
		log.Info("Successfully updated Secret",
			"namespace", targetNamespace,
			"name", secret.Name,
			"type", existingSecret.Type)
	}

	return nil
}

func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("Starting ConfigMap sync",
		"configMap", namespaceSync.Spec.ConfigMapName,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"targetNamespace", targetNamespace)

	// Get source configmap
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
		if errors.IsForbidden(err) {
			log.Error(err, "Permission denied to get ConfigMap in source namespace. Check RBAC permissions",
				"configMap", namespaceSync.Spec.ConfigMapName,
				"namespace", namespaceSync.Spec.SourceNamespace)
			return fmt.Errorf("permission denied to get ConfigMap: %w", err)
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

	r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &newConfigMap.ObjectMeta)

	// Try to create or update the configmap
	var existingConfigMap corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{
		Namespace: targetNamespace,
		Name:      configMap.Name,
	}, &existingConfigMap)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new configmap if it doesn't exist
			if err := r.Create(ctx, newConfigMap); err != nil {
				log.Error(err, "Failed to create ConfigMap")
				syncFailureCounter.WithLabelValues(targetNamespace, "configmap").Inc()
				return err
			}
			log.Info("Successfully created ConfigMap",
				"namespace", targetNamespace,
				"name", configMap.Name)
			syncSuccessCounter.WithLabelValues(targetNamespace, "configmap").Inc()
		} else {
			return err
		}
	} else {
		// Update existing configmap
		existingConfigMap.Data = configMap.Data
		existingConfigMap.BinaryData = configMap.BinaryData
		r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &existingConfigMap.ObjectMeta)

		if err := r.Update(ctx, &existingConfigMap); err != nil {
			log.Error(err, "Failed to update ConfigMap")
			return err
		}
		log.Info("Successfully updated ConfigMap",
			"namespace", targetNamespace,
			"name", configMap.Name)
	}

	return nil
}

func (r *NamespaceSyncReconciler) copyLabelsAndAnnotations(src, dst *metav1.ObjectMeta) {
	if dst.Labels == nil {
		dst.Labels = make(map[string]string)
	}
	if dst.Annotations == nil {
		dst.Annotations = make(map[string]string)
	}

	// 소스에서 메타데이터 사
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

// 임스페이스 변경시 관련된 NamespaceSync 찾기
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
		if !r.shouldSkipNamespace(namespace.Name, ns.Spec.SourceNamespace) {
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

// 리소스 정리를 위한 헬퍼 함수
func (r *NamespaceSyncReconciler) cleanupSyncedResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync) error {
	log := log.FromContext(ctx)

	// 모든 네임스페이스 목록 가져오기
	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		return err
	}

	for _, ns := range namespaceList.Items {
		if r.shouldSkipNamespace(ns.Name, namespaceSync.Spec.SourceNamespace) {
			continue
		}

		// Secret 삭제
		if namespaceSync.Spec.SecretName != "" {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespaceSync.Spec.SecretName,
					Namespace: ns.Name,
				},
			}
			if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete synced Secret",
					"namespace", ns.Name,
					"name", namespaceSync.Spec.SecretName)
			}
		}

		// ConfigMap 삭제
		if namespaceSync.Spec.ConfigMapName != "" {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespaceSync.Spec.ConfigMapName,
					Namespace: ns.Name,
				},
			}
			if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete synced ConfigMap",
					"namespace", ns.Name,
					"name", namespaceSync.Spec.ConfigMapName)
			}
		}
	}

	return nil
}
