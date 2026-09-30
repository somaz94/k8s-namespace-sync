package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// errSyncConflict marks a write the controller skipped because another NamespaceSync, or the person who
// created the object, owns it.
var errSyncConflict = errors.New("sync conflict")

// listPeers returns every NamespaceSync other than self that is not being deleted. A peer with an
// invalid spec still counts: once fixed it must find its objects where it left them.
func (r *NamespaceSyncReconciler) listPeers(ctx context.Context, self *syncv1.NamespaceSync) ([]syncv1.NamespaceSync, error) {
	var list syncv1.NamespaceSyncList
	// Peers are only read, so the cache's objects can be shared instead of deep-copied.
	if err := r.List(ctx, &list, client.UnsafeDisableDeepCopy); err != nil {
		return nil, fmt.Errorf("list NamespaceSyncs: %w", err)
	}
	peers := make([]syncv1.NamespaceSync, 0, len(list.Items))
	for _, item := range list.Items {
		if (item.Namespace == self.Namespace && item.Name == self.Name) || !item.DeletionTimestamp.IsZero() {
			continue
		}
		peers = append(peers, item)
	}
	return peers, nil
}

// syncsName reports whether namespaceSync syncs the named resource of resourceType ("secret" or "configmap").
func (r *NamespaceSyncReconciler) syncsName(namespaceSync *syncv1.NamespaceSync, resourceType, name string) bool {
	var filters syncv1.ResourceFilters
	if namespaceSync.Spec.ResourceFilters != nil {
		filters = *namespaceSync.Spec.ResourceFilters
	}
	switch resourceType {
	case "secret":
		return contains(namespaceSync.Spec.SecretName, name) && r.shouldSyncResource(name, filters.Secrets)
	case "configmap":
		return contains(namespaceSync.Spec.ConfigMapName, name) && r.shouldSyncResource(name, filters.ConfigMaps)
	default:
		return false
	}
}

// sourceOwner returns the peer that reads namespace/name as its source object, or nil.
func (r *NamespaceSyncReconciler) sourceOwner(peers []syncv1.NamespaceSync, resourceType, namespace, name string) *syncv1.NamespaceSync {
	for i := range peers {
		if peers[i].Spec.SourceNamespace == namespace && r.syncsName(&peers[i], resourceType, name) {
			return &peers[i]
		}
	}
	return nil
}

// copyOwner returns the peer that syncs name from sourceNamespace into namespace, preferring one that lists
// namespace in targetNamespaces, or nil.
func (r *NamespaceSyncReconciler) copyOwner(peers []syncv1.NamespaceSync, resourceType, sourceNamespace, namespace, name string) *syncv1.NamespaceSync {
	var owner *syncv1.NamespaceSync
	for i := range peers {
		peer := &peers[i]
		if peer.Spec.SourceNamespace != sourceNamespace || !r.syncsName(peer, resourceType, name) || !r.targetsNamespace(namespace, peer) {
			continue
		}
		if contains(peer.Spec.TargetNamespaces, namespace) {
			return peer
		}
		if owner == nil {
			owner = peer
		}
	}
	return owner
}

// sourceConflict reports a write into an object that a peer reads as its source.
func (r *NamespaceSyncReconciler) sourceConflict(peers []syncv1.NamespaceSync, resourceType, namespace, name string) error {
	owner := r.sourceOwner(peers, resourceType, namespace, name)
	if owner == nil {
		return nil
	}
	return fmt.Errorf("%w: %s %s/%s is the source of NamespaceSync %s/%s",
		errSyncConflict, resourceType, namespace, name, owner.Namespace, owner.Name)
}

// copyConflictGuard decides whether an existing object may be overwritten. It never overwrites a peer's
// source unless the object is already this sync's own copy, which is how explicit targets chain. An object
// no NamespaceSync created is overwritten only in an explicit target, so a peer's hand-made source stays
// safe while that peer is deleted and recreated. A copy a live peer syncs from a different source stays
// with that peer, unless this sync lists the namespace in targetNamespaces and the peer reaches it only by
// default. An orphaned copy is taken over.
func (r *NamespaceSyncReconciler) copyConflictGuard(peers []syncv1.NamespaceSync, resourceType, sourceNamespace string, explicit bool) func(client.Object) error {
	return func(existing client.Object) error {
		namespace, name := existing.GetNamespace(), existing.GetName()
		// Checked before the annotation test below: a hand-authored source carries no annotation.
		if !isManagedCopy(existing, sourceNamespace) {
			if err := r.sourceConflict(peers, resourceType, namespace, name); err != nil {
				return err
			}
		}
		from := existing.GetAnnotations()[AnnotationSourceNamespace]
		switch {
		case from == "" && !explicit:
			return fmt.Errorf("%w: %s %s/%s was not created by a NamespaceSync; list %s in targetNamespaces to overwrite it",
				errSyncConflict, resourceType, namespace, name, namespace)
		case from == "" || from == sourceNamespace:
			return nil
		}
		owner := r.copyOwner(peers, resourceType, from, namespace, name)
		switch {
		case owner == nil:
			return nil
		case explicit && !contains(owner.Spec.TargetNamespaces, namespace):
			return nil
		}
		return fmt.Errorf("%w: %s %s/%s is synced from namespace %s by NamespaceSync %s/%s",
			errSyncConflict, resourceType, namespace, name, from, owner.Namespace, owner.Name)
	}
}

// unclaimed returns the names no peer still needs in namespace, either as a copy it syncs from the same
// source or as its own source object, so deleting one NamespaceSync never removes another's objects.
func (r *NamespaceSyncReconciler) unclaimed(ctx context.Context, peers []syncv1.NamespaceSync, resourceType, sourceNamespace, namespace string, names []string) []string {
	free := make([]string, 0, len(names))
	for _, name := range names {
		if r.copyOwner(peers, resourceType, sourceNamespace, namespace, name) != nil ||
			r.sourceOwner(peers, resourceType, namespace, name) != nil {
			log.FromContext(ctx).V(1).Info("Keeping resource another NamespaceSync still uses",
				"resourceType", resourceType,
				"namespace", namespace,
				"name", name)
			continue
		}
		free = append(free, name)
	}
	return free
}

// oneLine joins the lines errors.Join produces, since status and events read better on one line.
func oneLine(err error) string {
	return strings.ReplaceAll(err.Error(), "\n", "; ")
}
