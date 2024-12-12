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

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

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

func (r *NamespaceSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.Log.WithValues("request_name", req.Name, "request_namespace", req.Namespace)
	log.Info("=== Reconcile function called ===")

	// Get NamespaceSync resource
	namespacesync := &syncv1.NamespaceSync{}
	err := r.Get(ctx, req.NamespacedName, namespacesync)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("NamespaceSync resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get NamespaceSync resource")
		return ctrl.Result{}, err
	}

	// Validate NamespaceSync resource
	if err := validateNamespaceSync(namespacesync); err != nil {
		log.Error(err, "Invalid NamespaceSync resource")
		// Update status to reflect validation error
		failedNamespaces := map[string]string{"validation": err.Error()}
		if updateErr := r.updateStatus(ctx, namespacesync, nil, failedNamespaces); updateErr != nil {
			log.Error(updateErr, "Failed to update status after validation error")
		}
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
		if r.shouldSyncToNamespace(ns.Name, namespacesync) {
			if err := r.syncResources(ctx, namespacesync, ns.Name); err != nil {
				log.Error(err, "Failed to sync resources", "namespace", ns.Name)
				failedNamespaces[ns.Name] = err.Error()
				continue
			}
			log.Info("Successfully synced resources", "namespace", ns.Name)
			syncedNamespaces = append(syncedNamespaces, ns.Name)
		}
	}

	// Update status
	if err := r.updateStatus(ctx, namespacesync, syncedNamespaces, failedNamespaces); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncs)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncsForSecret)).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.findNamespaceSyncsForConfigMap)).
		Complete(r)
}
