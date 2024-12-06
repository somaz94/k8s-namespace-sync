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
	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	cleanupSuccessCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_cleanup_success_total",
			Help: "Number of successful resource cleanups",
		},
		[]string{"namespace", "resource_type"},
	)

	cleanupFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespacesync_cleanup_failure_total",
			Help: "Number of failed resource cleanups",
		},
		[]string{"namespace", "resource_type"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		syncSuccessCounter,
		syncFailureCounter,
		cleanupSuccessCounter,
		cleanupFailureCounter,
	)
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
	log := log.Log.WithValues("request_name", req.Name, "request_namespace", req.Namespace)
	log.Info("=== Reconcile function called ===")

	// Get NamespaceSync resource
	namespacesync := &syncv1.NamespaceSync{}
	err := r.Get(ctx, req.NamespacedName, namespacesync)
	if err != nil {
		if errors.IsNotFound(err) {
			// 리소스가 이미 삭제됨
			log.Info("NamespaceSync resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get NamespaceSync resource")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !namespacesync.DeletionTimestamp.IsZero() {
		return r.handleDeletionAndStatus(ctx, namespacesync)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(namespacesync, finalizerName) {
		log.Info("Adding finalizer")
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Get the latest version
			latest := &syncv1.NamespaceSync{}
			if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
				return err
			}

			if !controllerutil.ContainsFinalizer(latest, finalizerName) {
				controllerutil.AddFinalizer(latest, finalizerName)
				return r.Update(ctx, latest)
			}
			return nil
		})

		if err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Process the sync
	log.Info("Successfully retrieved NamespaceSync resource",
		"spec", namespacesync.Spec,
		"status", namespacesync.Status)

	syncedNamespaces := []string{}
	failedNamespaces := map[string]string{}

	// Get list of all namespaces
	namespaceList := &corev1.NamespaceList{}
	if err := r.List(ctx, namespaceList); err != nil {
		log.Error(err, "Failed to list namespaces")
		return ctrl.Result{}, err
	}

	// Sync to each namespace except the source and excluded ones
	for _, ns := range namespaceList.Items {
		if ns.Name != namespacesync.Spec.SourceNamespace && !r.shouldSkipNamespace(ns.Name, namespacesync.Spec.SourceNamespace, namespacesync.Spec.Exclude) {
			if err := r.syncResources(ctx, namespacesync, ns.Name); err != nil {
				log.Error(err, "Failed to sync resources", "namespace", ns.Name)
				failedNamespaces[ns.Name] = err.Error()
				continue
			}
			log.Info("Successfully synced resources", "namespace", ns.Name)
			syncedNamespaces = append(syncedNamespaces, ns.Name)
		}
	}

	// Update status only if resource exists and is not being deleted
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get latest version before updating status
		latest := &syncv1.NamespaceSync{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			return err
		}

		// Update status fields
		latest.Status.LastSyncTime = metav1.NewTime(time.Now())
		latest.Status.SyncedNamespaces = syncedNamespaces
		latest.Status.FailedNamespaces = failedNamespaces
		latest.Status.ObservedGeneration = latest.Generation

		// Update Ready condition
		readyCondition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "SyncComplete",
			Message:            fmt.Sprintf("Successfully synced to %d namespaces", len(syncedNamespaces)),
		}

		if len(failedNamespaces) > 0 {
			readyCondition.Status = metav1.ConditionFalse
			readyCondition.Reason = "SyncFailed"
			readyCondition.Message = fmt.Sprintf("Failed to sync to %d namespaces", len(failedNamespaces))
		}

		meta.SetStatusCondition(&latest.Status.Conditions, readyCondition)

		return r.Status().Update(ctx, latest)
	}); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceSyncReconciler) handleDeletionAndStatus(ctx context.Context, namespacesync *syncv1.NamespaceSync) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(namespacesync, finalizerName) {
		// 동기화된 리소스 정리
		if err := r.cleanupSyncedResources(ctx, namespacesync); err != nil {
			log.Error(err, "Failed to cleanup resources")
			return ctrl.Result{}, err
		}

		// Finalizer 제거
		controllerutil.RemoveFinalizer(namespacesync, finalizerName)
		if err := r.Update(ctx, namespacesync); err != nil {
			// 리소스가 이미 삭제된 경우 무시
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceSyncReconciler) handleDeletion(ctx context.Context, namespacesync *syncv1.NamespaceSync) error {
	log := log.Log.WithValues("resource", namespacesync.Name)
	log.Info("Starting cleanup of synced resources")

	// Get the latest version of the resource before removing finalizer
	var latestNamespaceSync syncv1.NamespaceSync
	if err := r.Get(ctx, types.NamespacedName{
		Name:      namespacesync.Name,
		Namespace: namespacesync.Namespace,
	}, &latestNamespaceSync); err != nil {
		if errors.IsNotFound(err) {
			return nil // Resource is already gone
		}
		return fmt.Errorf("failed to get latest version: %w", err)
	}

	// Get list of all namespaces
	namespaces := &corev1.NamespaceList{}
	if err := r.List(ctx, namespaces); err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	// Clean up resources from all synced namespaces
	for _, ns := range namespaces.Items {
		// Skip source namespace and system namespaces
		if ns.Name == namespacesync.Spec.SourceNamespace || r.isSystemNamespace(ns.Name) {
			continue
		}

		// Delete all synced resources from this namespace
		if err := r.deleteResourcesFromNamespace(ctx, namespacesync, ns.Name); err != nil {
			log.Error(err, "Failed to cleanup resources", "namespace", ns.Name)
			// Continue with other namespaces even if one fails
			continue
		}
	}

	log.Info("Cleanup completed successfully")

	// Remove finalizer from the latest version
	controllerutil.RemoveFinalizer(&latestNamespaceSync, finalizerName)
	if err := r.Update(ctx, &latestNamespaceSync); err != nil {
		return fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return nil
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
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				logger.Info("Create event detected", "name", e.Object.GetName())
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				logger.Info("Update event detected", "name", e.ObjectNew.GetName())
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				logger.Info("Delete event detected", "name", e.Object.GetName())
				return true
			},
			GenericFunc: func(e event.GenericEvent) bool {
				logger.Info("Generic event detected", "name", e.Object.GetName())
				return true
			},
		}).
		// 네임스페이스 변경 감시
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncs)).
		// Secret 변경 감시 추가
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncsForSecret)).
		// ConfigMap 변경 감 추가
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
		// Check if the secret name is in the SecretName list
		for _, secretName := range ns.Spec.SecretName {
			if secretName == secret.Name && ns.Spec.SourceNamespace == secret.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ns.Name,
						Namespace: ns.Namespace,
					},
				})
				break
			}
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
		// Check if the configmap name is in the ConfigMapName list
		for _, configMapName := range ns.Spec.ConfigMapName {
			if configMapName == configMap.Name && ns.Spec.SourceNamespace == configMap.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ns.Name,
						Namespace: ns.Namespace,
					},
				})
				break
			}
		}
	}
	return requests
}

func (r *NamespaceSyncReconciler) updateStatus(ctx context.Context, namespaceSync *syncv1.NamespaceSync, syncedNamespaces []string, failedNamespaces map[string]string) error {
	log := log.Log.WithValues("resource", namespaceSync.Name, "namespace", namespaceSync.Namespace)

	// 리소스가 삭제 중인지 확인
	if namespaceSync.DeletionTimestamp != nil {
		log.Info("Resource is being deleted, skipping status update")
		return nil
	}

	// Get the latest version of the resource
	var latest syncv1.NamespaceSync
	if err := r.Get(ctx, types.NamespacedName{
		Name:      namespaceSync.Name,
		Namespace: namespaceSync.Namespace,
	}, &latest); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Resource no longer exists, skipping status update")
			return nil
		}
		return err
	}

	// Update status fields
	latest.Status.LastSyncTime = metav1.NewTime(time.Now())
	latest.Status.SyncedNamespaces = syncedNamespaces
	latest.Status.FailedNamespaces = failedNamespaces
	latest.Status.ObservedGeneration = latest.Generation

	// Update Ready condition
	readyCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: latest.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "SyncComplete",
		Message:            fmt.Sprintf("Successfully synced to %d namespaces", len(syncedNamespaces)),
	}

	if len(failedNamespaces) > 0 {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "SyncFailed"
		readyCondition.Message = fmt.Sprintf("Failed to sync to %d namespaces", len(failedNamespaces))
	}

	meta.SetStatusCondition(&latest.Status.Conditions, readyCondition)

	// Update the status with retries
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Status().Update(ctx, &latest); err != nil {
			// Get the latest version before retrying
			if err := r.Get(ctx, types.NamespacedName{
				Name:      latest.Name,
				Namespace: latest.Namespace,
			}, &latest); err != nil {
				return err
			}
			return err
		}
		return nil
	})
}

func (r *NamespaceSyncReconciler) syncResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace string) error {
	log := log.FromContext(ctx)
	log.Info("syncResources called",
		"targetNamespace", targetNamespace,
		"sourceNamespace", namespaceSync.Spec.SourceNamespace,
		"secretCount", len(namespaceSync.Spec.SecretName),
		"configMapCount", len(namespaceSync.Spec.ConfigMapName))

	// Sync Secrets
	for _, secretName := range namespaceSync.Spec.SecretName {
		if err := r.syncSecret(ctx, namespaceSync.Spec.SourceNamespace, targetNamespace, secretName); err != nil {
			log.Error(err, "Failed to sync secret",
				"secretName", secretName,
				"targetNamespace", targetNamespace)
			return err
		}
		log.Info("Successfully synced secret",
			"secretName", secretName,
			"targetNamespace", targetNamespace)
	}

	// Sync ConfigMaps
	for _, configMapName := range namespaceSync.Spec.ConfigMapName {
		log.Info("Starting ConfigMap sync",
			"configMap", configMapName,
			"sourceNamespace", namespaceSync.Spec.SourceNamespace,
			"targetNamespace", targetNamespace)

		if err := r.syncConfigMap(ctx, namespaceSync, targetNamespace, configMapName); err != nil {
			log.Error(err, "Failed to sync configmap",
				"configMapName", configMapName,
				"targetNamespace", targetNamespace)
			return err
		}
		log.Info("Successfully synced configmap",
			"configMapName", configMapName,
			"targetNamespace", targetNamespace)
	}

	return nil
}

func (r *NamespaceSyncReconciler) createOrUpdateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) error {
	log := log.Log.WithValues("namespace", configMap.Namespace, "name", configMap.Name)

	// Try to create or update the configmap
	var existingConfigMap corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{
		Namespace: configMap.Namespace,
		Name:      configMap.Name,
	}, &existingConfigMap)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new configmap if it doesn't exist
			if err := r.Create(ctx, configMap); err != nil {
				log.Error(err, "Failed to create ConfigMap")
				syncFailureCounter.WithLabelValues(configMap.Namespace, "configmap").Inc()
				return err
			}
			log.Info("Successfully created ConfigMap")
			syncSuccessCounter.WithLabelValues(configMap.Namespace, "configmap").Inc()
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
		log.Info("Successfully updated ConfigMap")
	}

	return nil
}

func (r *NamespaceSyncReconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	log := log.Log.WithValues("namespace", secret.Namespace, "name", secret.Name)

	// Check if secret exists in target namespace
	var existingSecret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, &existingSecret)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get target secret")
			return err
		}
		// Create new secret if it doesn't exist
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: secret.Namespace,
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
		log.Info("Successfully created Secret")
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
		log.Info("Successfully updated Secret")
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

	// 소스에서 메타데이 사
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
	log := log.Log.WithValues("namespace", obj.(*corev1.Namespace).Name)
	namespace := obj.(*corev1.Namespace)
	var namespaceSyncs syncv1.NamespaceSyncList
	if err := r.List(ctx, &namespaceSyncs); err != nil {
		log.Error(err, "Failed to list NamespaceSyncs")
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range namespaceSyncs.Items {
		// 1. 소스 네임스페이가 변경된 경우
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

		// 2. 상 네임스페이스가 생성/수정된 경우
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

// 리소스 정리를 위 헬퍼 함수
func (r *NamespaceSyncReconciler) cleanupSyncedResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync) error {
	log := log.FromContext(ctx)
	log.Info("Starting cleanup of synced resources")

	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Failed to list namespaces during cleanup")
		return err
	}

	var errs []error
	for _, ns := range namespaceList.Items {
		if r.shouldSkipNamespace(ns.Name, namespaceSync.Spec.SourceNamespace, namespaceSync.Spec.Exclude) {
			continue
		}

		// Delete Secrets
		for _, secretName := range namespaceSync.Spec.SecretName {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ns.Name,
				},
			}
			if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete synced Secret",
					"namespace", ns.Name,
					"name", secretName)
				errs = append(errs, err)
			} else {
				log.Info("Successfully deleted Secret",
					"namespace", ns.Name,
					"name", secretName)
			}
		}

		// Delete ConfigMaps
		for _, configMapName := range namespaceSync.Spec.ConfigMapName {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: ns.Name,
				},
			}
			if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete synced ConfigMap",
					"namespace", ns.Name,
					"name", configMapName)
				errs = append(errs, err)
			} else {
				log.Info("Successfully deleted ConfigMap",
					"namespace", ns.Name,
					"name", configMapName)
			}
		}
	}

	log.Info("Completed cleanup of synced resources")

	if len(errs) > 0 {
		return fmt.Errorf("encountered errors during cleanup: %v", errs)
	}
	return nil
}

// // shouldSyncSecret checks if the secret should be synced
// func (r *NamespaceSyncReconciler) shouldSyncSecret(ns *syncv1.NamespaceSync, secret *corev1.Secret) bool {
// 	for _, secretName := range ns.Spec.SecretName {
// 		if secretName == secret.Name {
// 			return true
// 		}
// 	}
// 	return false
// }

// // shouldSyncConfigMap checks if the configmap should be synced
// func (r *NamespaceSyncReconciler) shouldSyncConfigMap(ns *syncv1.NamespaceSync, configMap *corev1.ConfigMap) bool {
// 	for _, configMapName := range ns.Spec.ConfigMapName {
// 		if configMapName == configMap.Name {
// 			return true
// 		}
// 	}
// 	return false
// }

// syncConfigMap 함수 수정 (configMapName 파라터 추가)
func (r *NamespaceSyncReconciler) syncConfigMap(ctx context.Context, namespaceSync *syncv1.NamespaceSync, targetNamespace, configMapName string) error {
	log := log.FromContext(ctx)

	// Get source configmap
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespaceSync.Spec.SourceNamespace,
		Name:      configMapName,
	}, &configMap); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ConfigMap not found in source namespace",
				"configMap", configMapName,
				"namespace", namespaceSync.Spec.SourceNamespace)
			return nil
		}
		log.Error(err, "Failed to get source ConfigMap")
		return err
	}

	// Create new ConfigMap for target namespace
	newConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: targetNamespace,
		},
		Data:       configMap.Data,
		BinaryData: configMap.BinaryData,
	}
	r.copyLabelsAndAnnotations(&configMap.ObjectMeta, &newConfigMap.ObjectMeta)

	// Create or update the configmap
	if err := r.createOrUpdateConfigMap(ctx, newConfigMap); err != nil {
		return err
	}

	return nil
}

// syncSecret 함수 수정
func (r *NamespaceSyncReconciler) syncSecret(ctx context.Context, sourceNamespace, targetNamespace, secretName string) error {
	log := log.Log.WithValues(
		"secret", secretName,
		"sourceNamespace", sourceNamespace,
		"targetNamespace", targetNamespace,
	)

	// Get source secret
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: sourceNamespace, Name: secretName}, &secret); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Secret not found in source namespace")
			return nil
		}
		log.Error(err, "Failed to get source Secret")
		return err
	}

	log.Info("Found source Secret",
		"name", secret.Name,
		"namespace", secret.Namespace,
		"type", secret.Type)

	// Create new secret for target namespace
	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespace,
		},
		Type: secret.Type,
		Data: secret.Data,
	}
	r.copyLabelsAndAnnotations(&secret.ObjectMeta, &newSecret.ObjectMeta)

	if err := r.createOrUpdateSecret(ctx, newSecret); err != nil {
		return err
	}

	return nil
}

// deleteResourcesFromNamespace deletes all synced resources from the specified namespace
func (r *NamespaceSyncReconciler) deleteResourcesFromNamespace(ctx context.Context, namespacesync *syncv1.NamespaceSync, namespace string) error {
	log := log.Log.WithValues("resource", namespacesync.Name, "namespace", namespace)

	// Delete Secrets
	for _, secretName := range namespacesync.Spec.SecretName {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
		}
		if err := r.Delete(ctx, secret); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete Secret", "name", secretName)
				return fmt.Errorf("failed to delete secret %s: %w", secretName, err)
			}
		} else {
			log.Info("Successfully deleted Secret", "name", secretName)
		}
	}

	// Delete ConfigMaps
	for _, configMapName := range namespacesync.Spec.ConfigMapName {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: namespace,
			},
		}
		if err := r.Delete(ctx, configMap); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete ConfigMap", "name", configMapName)
				return fmt.Errorf("failed to delete configmap %s: %w", configMapName, err)
			}
		} else {
			log.Info("Successfully deleted ConfigMap", "name", configMapName)
		}
	}

	return nil
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
