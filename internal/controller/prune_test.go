package controller

import (
	"context"
	"testing"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	pruneSource = "platform"
	pruneKept   = "kept"
	pruneDrop   = "dropped"
)

func pruneSync(name string, secrets []string, exclude ...string) *syncv1.NamespaceSync {
	return &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pruneSource},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: pruneSource,
			SecretName:      secrets,
			Exclude:         exclude,
		},
	}
}

func copySecret(name, namespace, source string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: managedCopyMeta(name, namespace, source)}
}

func exists(t *testing.T, c client.Client, name, namespace string) bool {
	t.Helper()
	err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &corev1.Secret{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get %s/%s: %v", namespace, name, err)
	}
	return err == nil
}

func TestPruneOrphanedCopies(t *testing.T) {
	objs := []client.Object{
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pruneKept, Namespace: pruneSource}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pruneDrop, Namespace: pruneSource}},
		copySecret(pruneKept, "team-a", pruneSource),
		copySecret(pruneDrop, "team-a", pruneSource),
		copySecret(pruneKept, "excluded", pruneSource),
		copySecret(pruneDrop, "shared", pruneSource),
		copySecret(pruneDrop, "team-b", "other-source"),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pruneDrop, Namespace: "by-hand"}},
	}
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(objs...).Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: newTestScheme()}

	self := pruneSync("self", []string{pruneKept}, "excluded")
	peer := pruneSync("peer", []string{pruneDrop})
	peer.Spec.TargetNamespaces = []string{"shared"}

	if errs := r.pruneOrphanedCopies(context.Background(), self, []syncv1.NamespaceSync{*peer}, false); len(errs) > 0 {
		t.Fatalf("pruneOrphanedCopies() errors = %v", errs)
	}

	cases := []struct {
		name, namespace string
		want            bool
		why             string
	}{
		{pruneKept, "team-a", true, "still in the spec"},
		{pruneDrop, "team-a", false, "dropped from the spec"},
		{pruneKept, "excluded", false, "namespace is now excluded"},
		{pruneDrop, "shared", true, "a peer still syncs it there"},
		{pruneDrop, "team-b", true, "copied from another source"},
		{pruneDrop, "by-hand", true, "never synced by the controller"},
		{pruneDrop, pruneSource, true, "the source object itself"},
	}
	for _, tc := range cases {
		if got := exists(t, c, tc.name, tc.namespace); got != tc.want {
			t.Errorf("%s/%s exists = %v, want %v (%s)", tc.namespace, tc.name, got, tc.want, tc.why)
		}
	}
}

func TestPruneOrphanedCopiesWhenDeleting(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pruneKept, Namespace: pruneSource}},
		copySecret(pruneKept, "team-a", pruneSource),
		copySecret(pruneDrop, "team-a", pruneSource),
	).Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: newTestScheme()}

	if errs := r.pruneOrphanedCopies(context.Background(), pruneSync("self", []string{pruneKept}), nil, true); len(errs) > 0 {
		t.Fatalf("pruneOrphanedCopies() errors = %v", errs)
	}
	if exists(t, c, pruneKept, "team-a") || exists(t, c, pruneDrop, "team-a") {
		t.Error("every copy from the source must go when the NamespaceSync is deleted")
	}
	if !exists(t, c, pruneKept, pruneSource) {
		t.Error("the source object must stay")
	}
}

func TestReconcilePrunesCopyAfterSpecChange(t *testing.T) {
	scheme := newTestScheme()
	nsync := pruneSync("sync", []string{pruneKept, pruneDrop})
	nsync.Spec.TargetNamespaces = []string{"team-a"}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: pruneSource}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pruneKept, Namespace: pruneSource}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pruneDrop, Namespace: pruneSource}},
		nsync,
	).WithStatusSubresource(nsync).Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "sync", Namespace: pruneSource}}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if !exists(t, c, pruneDrop, "team-a") {
		t.Fatal("setup: the copy was not synced")
	}

	var latest syncv1.NamespaceSync
	if err := c.Get(context.Background(), req.NamespacedName, &latest); err != nil {
		t.Fatal(err)
	}
	latest.Spec.SecretName = []string{pruneKept}
	if err := c.Update(context.Background(), &latest); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	if exists(t, c, pruneDrop, "team-a") {
		t.Error("the copy of a name dropped from the spec must be deleted")
	}
	if !exists(t, c, pruneKept, "team-a") {
		t.Error("the copy still in the spec must stay")
	}
}

func TestControllerNamespaceIsNeverATarget(t *testing.T) {
	custom := &NamespaceSyncReconciler{ControllerNamespace: "ops"}
	if !custom.isSystemNamespace("ops") {
		t.Error("the namespace the controller runs in must be excluded")
	}
	if custom.isSystemNamespace(DefaultControllerNamespace) {
		t.Error("the default namespace is only excluded when the controller runs there")
	}
	if !(&NamespaceSyncReconciler{}).isSystemNamespace(DefaultControllerNamespace) {
		t.Error("without POD_NAMESPACE the default install namespace is excluded")
	}
}
