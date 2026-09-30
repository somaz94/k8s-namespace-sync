package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	finalizerName             = "namespacesync.nsync.dev/finalizer"
	AnnotationSourceNamespace = "namespacesync.nsync.dev/source-namespace"
	AnnotationSourceName      = "namespacesync.nsync.dev/source-name"
	AnnotationLastSync        = "namespacesync.nsync.dev/last-sync"
)

// handleDeletionAndStatus handles resource deletion and status updates
func (r *NamespaceSyncReconciler) handleDeletionAndStatus(ctx context.Context, namespacesync *syncv1.NamespaceSync) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(namespacesync, finalizerName) {
		if err := r.cleanupSyncedResources(ctx, namespacesync); err != nil {
			log.Error(err, "Failed to cleanup resources")
			if r.Recorder != nil {
				r.Recorder.Eventf(namespacesync, corev1.EventTypeWarning, "CleanupFailed", "Failed to clean up synced resources: %v", err)
			}
			return ctrl.Result{}, err
		}

		if r.Recorder != nil {
			r.Recorder.Event(namespacesync, corev1.EventTypeNormal, "CleanupComplete", "Successfully cleaned up synced resources")
		}

		controllerutil.RemoveFinalizer(namespacesync, finalizerName)
		if err := r.Update(ctx, namespacesync); err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// createOrUpdateResource creates desired, or updates existing to match it when anything but the last-sync stamp differs.
// guard can refuse to touch an existing object; updateFields copies resource-specific data from desired to existing.
func createOrUpdateResource[T client.Object](
	r *NamespaceSyncReconciler,
	ctx context.Context,
	desired T,
	existing T,
	resourceType string,
	guard func(client.Object) error,
	updateFields func(src, dst T),
) error {
	log := log.FromContext(ctx).WithValues(
		"namespace", desired.GetNamespace(),
		"name", desired.GetName(),
	)

	err := r.Get(ctx, types.NamespacedName{
		Namespace: desired.GetNamespace(),
		Name:      desired.GetName(),
	}, existing)

	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				log.Error(err, "Failed to create resource", "resourceType", resourceType)
				recordSyncFailure(desired.GetNamespace(), resourceType)
				return err
			}
			log.Info("Successfully created resource", "resourceType", resourceType)
			recordSyncSuccess(desired.GetNamespace(), resourceType)
			return nil
		}
		return err
	}

	if err := guard(existing); err != nil {
		recordSyncFailure(desired.GetNamespace(), resourceType)
		return err
	}

	before := existing.DeepCopyObject().(T)
	updateFields(desired, existing)

	if equalIgnoringLastSync(before, existing) {
		log.V(1).Info("Resource already in sync", "resourceType", resourceType)
		recordSyncSuccess(desired.GetNamespace(), resourceType)
		return nil
	}

	if err := r.Update(ctx, existing); err != nil {
		log.Error(err, "Failed to update resource", "resourceType", resourceType)
		recordSyncFailure(desired.GetNamespace(), resourceType)
		return err
	}

	log.Info("Successfully updated resource", "resourceType", resourceType)
	recordSyncSuccess(desired.GetNamespace(), resourceType)
	return nil
}

// equalIgnoringLastSync reports whether a and b differ at most in the last-sync stamp. Rewriting only the
// stamp fires a watch event that requeues the NamespaceSync, so every copy would be rewritten without end.
func equalIgnoringLastSync(a, b client.Object) bool {
	a, b = a.DeepCopyObject().(client.Object), b.DeepCopyObject().(client.Object)
	for _, obj := range []client.Object{a, b} {
		annotations := obj.GetAnnotations()
		delete(annotations, AnnotationLastSync)
		obj.SetAnnotations(annotations)
	}
	return equality.Semantic.DeepEqual(a, b)
}

// copyLabelsAndAnnotations copies labels and annotations from source to destination
func (r *NamespaceSyncReconciler) copyLabelsAndAnnotations(src, dst *metav1.ObjectMeta) {
	if dst.Labels == nil {
		dst.Labels = make(map[string]string)
	}
	if dst.Annotations == nil {
		dst.Annotations = make(map[string]string)
	}

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

	dst.Annotations[AnnotationSourceNamespace] = src.Namespace
	dst.Annotations[AnnotationSourceName] = src.Name
	dst.Annotations[AnnotationLastSync] = time.Now().Format(time.RFC3339)
}

// isManagedCopy reports whether obj carries the source annotations stamped on every copy synced from sourceNamespace.
func isManagedCopy(obj client.Object, sourceNamespace string) bool {
	annotations := obj.GetAnnotations()
	return annotations[AnnotationSourceNamespace] == sourceNamespace &&
		annotations[AnnotationSourceName] == obj.GetName()
}

// deleteManagedCopy deletes obj only if it is a copy synced from sourceNamespace, so a same-named
// object someone else created in the target namespace is never removed. It reports whether it deleted.
func (r *NamespaceSyncReconciler) deleteManagedCopy(ctx context.Context, obj client.Object, sourceNamespace string) (bool, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := r.uncachedReader().Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get %s: %w", key, err)
	}
	if !isManagedCopy(obj, sourceNamespace) {
		log.FromContext(ctx).V(1).Info("Skipping delete of a resource this controller did not sync",
			"namespace", key.Namespace,
			"name", key.Name)
		return false, nil
	}
	// Preconditions stop the delete if the object was replaced or changed (e.g. annotations stripped) since the Get.
	uid, rv := obj.GetUID(), obj.GetResourceVersion()
	if err := r.Delete(ctx, obj, client.Preconditions{UID: &uid, ResourceVersion: &rv}); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("delete %s: %w", key, err)
	}
	return true, nil
}

// uncachedReader returns APIReader when set, falling back to the (cached) client.
func (r *NamespaceSyncReconciler) uncachedReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

// cleanupResource deletes the named copies synced from sourceNamespace out of namespace.
// newObj builds the typed stub to look up and delete.
func (r *NamespaceSyncReconciler) cleanupResource(
	ctx context.Context,
	sourceNamespace string,
	namespace string,
	names []string,
	resourceType string,
	newObj func(name, namespace string) client.Object,
) []error {
	log := log.FromContext(ctx)
	var errs []error
	for _, name := range names {
		deleted, err := r.deleteManagedCopy(ctx, newObj(name, namespace), sourceNamespace)
		switch {
		case err != nil:
			log.Error(err, "Failed to delete synced resource",
				"resourceType", resourceType,
				"namespace", namespace,
				"name", name)
			recordCleanupFailure(namespace, resourceType)
			errs = append(errs, err)
		case deleted:
			log.Info("Successfully deleted resource",
				"resourceType", resourceType,
				"namespace", namespace,
				"name", name)
			recordCleanupSuccess(namespace, resourceType)
		}
	}
	return errs
}

// cleanupSyncedResources cleans up all synced resources
func (r *NamespaceSyncReconciler) cleanupSyncedResources(ctx context.Context, namespaceSync *syncv1.NamespaceSync) error {
	log := log.FromContext(ctx)
	log.Info("Starting cleanup of synced resources")

	var namespaceList corev1.NamespaceList
	if err := r.List(ctx, &namespaceList); err != nil {
		log.Error(err, "Failed to list namespaces during cleanup")
		return err
	}

	peers, err := r.listPeers(ctx, namespaceSync)
	if err != nil {
		log.Error(err, "Failed to list NamespaceSyncs during cleanup")
		return err
	}

	source := namespaceSync.Spec.SourceNamespace
	var errs []error
	for _, ns := range namespaceList.Items {
		if !r.shouldSyncToNamespace(ctx, ns.Name, namespaceSync) {
			continue
		}

		errs = append(errs, r.cleanupResource(ctx, source, ns.Name,
			r.unclaimed(ctx, peers, "secret", source, ns.Name, namespaceSync.Spec.SecretName), "secret",
			func(name, namespace string) client.Object {
				return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
			})...)

		errs = append(errs, r.cleanupResource(ctx, source, ns.Name,
			r.unclaimed(ctx, peers, "configmap", source, ns.Name, namespaceSync.Spec.ConfigMapName), "configmap",
			func(name, namespace string) client.Object {
				return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
			})...)
	}

	log.Info("Completed cleanup of synced resources")

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
