package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Shared test fixtures for the controller package tests.
const (
	// conditionTypeReady is the metav1.Condition Type that updateStatus sets on
	// the NamespaceSync status.
	conditionTypeReady = "Ready"
	// updatedValue is the post-update payload written to a source ConfigMap /
	// Secret / label, then asserted on the synced target copy.
	updatedValue = "new-value"
	// fromA and fromB mark which NamespaceSync's source a copy came from in the conflict tests.
	fromA = "from-a"
	fromB = "from-b"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = syncv1.AddToScheme(scheme)
	return scheme
}

// managedCopyMeta returns the metadata the controller stamps on a copy synced from sourceNamespace.
func managedCopyMeta(name, namespace, sourceNamespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Annotations: map[string]string{
			AnnotationSourceNamespace: sourceNamespace,
			AnnotationSourceName:      name,
		},
	}
}

func TestReconcile_NotFound(t *testing.T) {
	scheme := newTestScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	if err != nil {
		t.Errorf("expected no error for not found, got %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue")
	}
}

func TestReconcile_ValidationError_EmptySource(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-sync",
			Namespace: "test-ns",
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "",
			SecretName:      []string{"secret1"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "invalid-sync", Namespace: "test-ns"},
	})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestReconcile_ValidationError_NoResources(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-sync2",
			Namespace: "test-ns",
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "source-ns",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "invalid-sync2", Namespace: "test-ns"},
	})
	if err == nil {
		t.Error("expected validation error for no resources")
	}
}

func TestReconcile_SuccessfulSync(t *testing.T) {
	scheme := newTestScheme()

	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "src-ns"}}
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tgt-ns"}}
	sourceSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "src-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("value")},
	}
	sourceCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "src-ns"},
		Data:       map[string]string{"key": "value"},
	}

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sync",
			Namespace: "src-ns",
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "src-ns",
			TargetNamespaces: []string{"tgt-ns"},
			SecretName:       []string{"my-secret"},
			ConfigMapName:    []string{"my-cm"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sourceNs, targetNs, sourceSecret, sourceCm, ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	// The first pass adds the finalizer and syncs; the second re-syncs over existing copies.
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-sync", Namespace: "src-ns"},
	})
	if err != nil {
		t.Fatalf("first reconcile error: %v", err)
	}

	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-sync", Namespace: "src-ns"},
	})
	if err != nil {
		t.Fatalf("second reconcile error: %v", err)
	}

	var synced corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "my-secret", Namespace: "tgt-ns"}, &synced)
	if err != nil {
		t.Errorf("expected secret to be synced to target, got error: %v", err)
	}

	var syncedCm corev1.ConfigMap
	err = client.Get(context.Background(), types.NamespacedName{Name: "my-cm", Namespace: "tgt-ns"}, &syncedCm)
	if err != nil {
		t.Errorf("expected configmap to be synced to target, got error: %v", err)
	}
}

func TestReconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "del-src-ns"}}
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "del-tgt-ns"}}

	syncedSecret := &corev1.Secret{
		ObjectMeta: managedCopyMeta("del-secret", "del-tgt-ns", "del-src-ns"),
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("value")},
	}

	syncedCm := &corev1.ConfigMap{
		ObjectMeta: managedCopyMeta("del-cm", "del-tgt-ns", "del-src-ns"),
		Data:       map[string]string{"key": "value"},
	}

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-del-sync",
			Namespace:         "del-src-ns",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "del-src-ns",
			TargetNamespaces: []string{"del-tgt-ns"},
			SecretName:       []string{"del-secret"},
			ConfigMapName:    []string{"del-cm"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sourceNs, targetNs, syncedSecret, syncedCm, ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-del-sync", Namespace: "del-src-ns"},
	})
	if err != nil {
		t.Fatalf("reconcile deletion error: %v", err)
	}

	var s corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "del-secret", Namespace: "del-tgt-ns"}, &s)
	if err == nil {
		t.Error("expected secret to be deleted from target namespace")
	}

	var cm corev1.ConfigMap
	err = client.Get(context.Background(), types.NamespacedName{Name: "del-cm", Namespace: "del-tgt-ns"}, &cm)
	if err == nil {
		t.Error("expected configmap to be deleted from target namespace")
	}
}

func TestReconcile_SourceSecretNotFound_DeletesFromTarget(t *testing.T) {
	scheme := newTestScheme()

	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "snf-src-ns"}}
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "snf-tgt-ns"}}

	// Target copies exist with no source objects behind them.
	syncedSecret := &corev1.Secret{
		ObjectMeta: managedCopyMeta("missing-secret", "snf-tgt-ns", "snf-src-ns"),
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("old-value")},
	}

	syncedCm := &corev1.ConfigMap{
		ObjectMeta: managedCopyMeta("missing-cm", "snf-tgt-ns", "snf-src-ns"),
		Data:       map[string]string{"key": "old-value"},
	}

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "snf-test",
			Namespace:  "snf-src-ns",
			Finalizers: []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "snf-src-ns",
			TargetNamespaces: []string{"snf-tgt-ns"},
			SecretName:       []string{"missing-secret"},
			ConfigMapName:    []string{"missing-cm"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sourceNs, targetNs, syncedSecret, syncedCm, ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "snf-test", Namespace: "snf-src-ns"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var s corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "missing-secret", Namespace: "snf-tgt-ns"}, &s)
	if err == nil {
		t.Error("expected secret to be deleted from target namespace when source is missing")
	}

	var cm corev1.ConfigMap
	err = client.Get(context.Background(), types.NamespacedName{Name: "missing-cm", Namespace: "snf-tgt-ns"}, &cm)
	if err == nil {
		t.Error("expected configmap to be deleted from target namespace when source is missing")
	}
}

func TestReconcile_WithResourceFilters(t *testing.T) {
	scheme := newTestScheme()

	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rf-src-ns"}}
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rf-tgt-ns"}}

	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "app-secret", Namespace: "rf-src-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("value")},
	}
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "app-secret-bak", Namespace: "rf-src-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("value")},
	}
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "rf-src-ns"},
		Data:       map[string]string{"key": "value"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config-bak", Namespace: "rf-src-ns"},
		Data:       map[string]string{"key": "value"},
	}

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "rf-test",
			Namespace:  "rf-src-ns",
			Finalizers: []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "rf-src-ns",
			TargetNamespaces: []string{"rf-tgt-ns"},
			SecretName:       []string{"app-secret", "app-secret-bak"},
			ConfigMapName:    []string{"app-config", "app-config-bak"},
			ResourceFilters: &syncv1.ResourceFilters{
				Secrets:    &syncv1.ResourceFilter{Exclude: []string{"*-bak"}},
				ConfigMaps: &syncv1.ResourceFilter{Include: []string{"app-config"}},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sourceNs, targetNs, secret1, secret2, cm1, cm2, ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "rf-test", Namespace: "rf-src-ns"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var s1 corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-secret", Namespace: "rf-tgt-ns"}, &s1); err != nil {
		t.Error("app-secret should be synced to target")
	}

	var s2 corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-secret-bak", Namespace: "rf-tgt-ns"}, &s2); err == nil {
		t.Error("app-secret-bak should NOT be synced to target")
	}

	var c1 corev1.ConfigMap
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-config", Namespace: "rf-tgt-ns"}, &c1); err != nil {
		t.Error("app-config should be synced to target")
	}

	var c2 corev1.ConfigMap
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-config-bak", Namespace: "rf-tgt-ns"}, &c2); err == nil {
		t.Error("app-config-bak should NOT be synced to target")
	}
}

func TestReconcile_UpdateExistingResources(t *testing.T) {
	scheme := newTestScheme()

	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "upd-src-ns"}}
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "upd-tgt-ns"}}

	sourceSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-secret", Namespace: "upd-src-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte(updatedValue)},
	}
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-secret", Namespace: "upd-tgt-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("old-value")},
	}

	sourceCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cm", Namespace: "upd-src-ns"},
		Data:       map[string]string{"key": updatedValue},
		BinaryData: map[string][]byte{"bin": {0x01, 0x02}},
	}
	existingCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cm", Namespace: "upd-tgt-ns"},
		Data:       map[string]string{"key": "old-value"},
	}

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "upd-test",
			Namespace:  "upd-src-ns",
			Finalizers: []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "upd-src-ns",
			TargetNamespaces: []string{"upd-tgt-ns"},
			SecretName:       []string{"upd-secret"},
			ConfigMapName:    []string{"upd-cm"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(sourceNs, targetNs, sourceSecret, existingSecret, sourceCm, existingCm, ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "upd-test", Namespace: "upd-src-ns"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var s corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: "upd-secret", Namespace: "upd-tgt-ns"}, &s); err != nil {
		t.Fatalf("failed to get updated secret: %v", err)
	}
	if string(s.Data["key"]) != updatedValue {
		t.Errorf("expected secret data 'new-value', got %q", string(s.Data["key"]))
	}

	var cm corev1.ConfigMap
	if err := client.Get(context.Background(), types.NamespacedName{Name: "upd-cm", Namespace: "upd-tgt-ns"}, &cm); err != nil {
		t.Fatalf("failed to get updated configmap: %v", err)
	}
	if cm.Data["key"] != updatedValue {
		t.Errorf("expected configmap data 'new-value', got %q", cm.Data["key"])
	}
	if string(cm.BinaryData["bin"]) != string([]byte{0x01, 0x02}) {
		t.Error("expected binary data to be synced")
	}
}

func TestReconcile_SecondPassWritesNothing(t *testing.T) {
	scheme := newTestScheme()

	sourceSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "idle-secret",
			Namespace:   "idle-src-ns",
			Labels:      map[string]string{"app": "demo"},
			Annotations: map[string]string{"note": "kept"},
		},
		Data: map[string][]byte{"key": []byte("value")},
	}
	// No labels, so the desired copy holds an empty map where the stored copy holds none.
	sourceCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "idle-cm", Namespace: "idle-src-ns"},
		Data:       map[string]string{"key": "value"},
	}
	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "idle-test",
			Namespace:  "idle-src-ns",
			Finalizers: []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "idle-src-ns",
			TargetNamespaces: []string{"idle-tgt-ns"},
			SecretName:       []string{"idle-secret"},
			ConfigMapName:    []string{"idle-cm"},
		},
	}

	var writes []string
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "idle-src-ns"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "idle-tgt-ns"}},
			sourceSecret, sourceCm, ns,
		).
		WithStatusSubresource(ns).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				writes = append(writes, "create "+obj.GetName())
				return cl.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				writes = append(writes, "update "+obj.GetName())
				return cl.Update(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				writes = append(writes, "patch "+obj.GetName())
				return cl.Patch(ctx, obj, patch, opts...)
			},
			Apply: func(ctx context.Context, cl client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
				writes = append(writes, "apply")
				return cl.Apply(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				writes = append(writes, "delete "+obj.GetName())
				return cl.Delete(ctx, obj, opts...)
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "idle-test", Namespace: "idle-src-ns"}}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if len(writes) != 2 {
		t.Fatalf("expected the first pass to create both copies, got %v", writes)
	}

	writes = nil
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if len(writes) != 0 {
		t.Errorf("expected no writes while the copies match their sources, got %v", writes)
	}
}

func TestReconcile_MetadataOnlyChangeKeepsLastSync(t *testing.T) {
	scheme := newTestScheme()
	const stamp = "2020-01-01T00:00:00Z"

	ns := newSecretSync("meta-test", "meta-src", []string{"meta-tgt"}, "shared")
	// The label a mutating admission would have added to the stored copy.
	stored := sharedSecret("meta-tgt", fromA)
	stored.ObjectMeta = managedCopyMeta("shared", "meta-tgt", "meta-src")
	stored.Labels = map[string]string{injectedLabel: labelTrue}
	stored.Annotations[AnnotationLastSync] = stamp

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("meta-src", "meta-tgt")...).
		WithObjects(sharedSecret("meta-src", fromA), stored, ns).
		WithStatusSubresource(ns).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, ns)
	s := getSharedSecret(t, c, "meta-tgt")
	if _, ok := s.Labels[injectedLabel]; ok {
		t.Errorf("expected the label the source lacks to be removed, got %v", s.Labels)
	}
	if got := s.Annotations[AnnotationLastSync]; got != stamp {
		t.Errorf("expected a metadata-only update to keep last-sync %q, got %q", stamp, got)
	}

	source := getSharedSecret(t, c, "meta-src")
	source.Data["key"] = []byte(updatedValue)
	if err := c.Update(context.Background(), source); err != nil {
		t.Fatalf("update source: %v", err)
	}
	reconcileSync(t, r, ns)
	if got := getSharedSecret(t, c, "meta-tgt").Annotations[AnnotationLastSync]; got == stamp {
		t.Errorf("expected a data change to move last-sync, got %q", got)
	}
}

func TestFindNamespaceSyncs(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "event-test",
			Namespace: "event-src-ns",
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "event-src-ns",
			SecretName:      []string{"my-secret"},
			Exclude:         []string{"excluded-ns"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "event-src-ns"}}
	requests := r.findNamespaceSyncs(context.Background(), sourceNs)
	if len(requests) == 0 {
		t.Error("expected reconcile request for source namespace change")
	}

	// No targetNamespaces, so any non-system, non-excluded namespace is a target.
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "some-target-ns"}}
	requests = r.findNamespaceSyncs(context.Background(), targetNs)
	if len(requests) == 0 {
		t.Error("expected reconcile request for target namespace change")
	}

	sysNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	requests = r.findNamespaceSyncs(context.Background(), sysNs)
	if len(requests) != 0 {
		t.Error("expected no reconcile request for system namespace")
	}
}

func TestFindNamespaceSyncsForSecret(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-event-test",
			Namespace: "sec-src-ns",
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "sec-src-ns",
			TargetNamespaces: []string{"sec-tgt-ns"},
			SecretName:       []string{"watched-secret"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	srcSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "watched-secret", Namespace: "sec-src-ns"}}
	requests := r.findNamespaceSyncsForSecret(context.Background(), srcSecret)
	if len(requests) == 0 {
		t.Error("expected reconcile for source secret change")
	}

	tgtSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "watched-secret", Namespace: "sec-tgt-ns"}}
	requests = r.findNamespaceSyncsForSecret(context.Background(), tgtSecret)
	if len(requests) == 0 {
		t.Error("expected reconcile for target secret change")
	}

	otherSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "other-secret", Namespace: "sec-src-ns"}}
	requests = r.findNamespaceSyncsForSecret(context.Background(), otherSecret)
	if len(requests) != 0 {
		t.Error("expected no reconcile for unrelated secret")
	}
}

func TestReconcile_GetError(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{Name: "err-sync", Namespace: "err-ns"},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "err-ns",
			SecretName:      []string{"secret1"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*syncv1.NamespaceSync); ok {
					return fmt.Errorf("api server unavailable")
				}
				return client.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "err-sync", Namespace: "err-ns"},
	})
	if err == nil {
		t.Error("expected error from Get failure")
	}
}

func TestReconcile_ListNamespacesError(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "list-err",
			Namespace:  "list-err-ns",
			Finalizers: []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "list-err-ns",
			SecretName:      []string{"secret1"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.NamespaceList); ok {
					return fmt.Errorf("list namespaces failed")
				}
				return client.List(ctx, list, opts...)
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "list-err", Namespace: "list-err-ns"},
	})
	if err == nil {
		t.Error("expected error from List namespaces failure")
	}
}

func TestUpdateStatus_DeletionTimestamp(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "del-status",
			Namespace:         "del-status-ns",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "del-status-ns",
			SecretName:      []string{"s"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	err := r.updateStatus(context.Background(), ns, []string{"ns1"}, nil)
	if err != nil {
		t.Errorf("expected no error when deletion timestamp is set, got %v", err)
	}
}

func TestCleanupSyncedResources_ListError(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{Name: "cleanup-err", Namespace: "cleanup-ns"},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "cleanup-ns",
			SecretName:      []string{"s1"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*corev1.NamespaceList); ok {
					return fmt.Errorf("list error")
				}
				return client.List(ctx, list, opts...)
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	err := r.cleanupSyncedResources(context.Background(), ns)
	if err == nil {
		t.Error("expected error from List failure during cleanup")
	}
}

func TestFindNamespaceSyncs_ListError(t *testing.T) {
	scheme := newTestScheme()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("list error")
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	requests := r.findNamespaceSyncs(context.Background(), nsObj)
	if requests != nil {
		t.Error("expected nil requests on List error")
	}
}

func TestFindNamespaceSyncsForSecret_ListError(t *testing.T) {
	scheme := newTestScheme()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("list error")
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "ns"}}
	requests := r.findNamespaceSyncsForSecret(context.Background(), secret)
	if requests != nil {
		t.Error("expected nil requests on List error")
	}
}

func TestFindNamespaceSyncsForConfigMap_ListError(t *testing.T) {
	scheme := newTestScheme()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("list error")
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"}}
	requests := r.findNamespaceSyncsForConfigMap(context.Background(), cm)
	if requests != nil {
		t.Error("expected nil requests on List error")
	}
}

func TestUpdateStatus_AllSynced(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "status-all-synced",
			Namespace:  "status-ns",
			Generation: 1,
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "status-ns",
			SecretName:      []string{"s"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	err := r.updateStatus(context.Background(), ns, []string{"ns1", "ns2", "ns3"}, nil)
	if err != nil {
		t.Fatalf("updateStatus error: %v", err)
	}

	var updated syncv1.NamespaceSync
	err = c.Get(context.Background(), types.NamespacedName{Name: "status-all-synced", Namespace: "status-ns"}, &updated)
	if err != nil {
		t.Fatalf("failed to get updated resource: %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}

	cond := updated.Status.Conditions[0]
	if cond.Type != conditionTypeReady {
		t.Errorf("expected condition type 'Ready', got %q", cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected condition status True, got %q", cond.Status)
	}
	if cond.Reason != "SyncComplete" {
		t.Errorf("expected reason 'SyncComplete', got %q", cond.Reason)
	}
	if len(updated.Status.SyncedNamespaces) != 3 {
		t.Errorf("expected 3 synced namespaces, got %d", len(updated.Status.SyncedNamespaces))
	}
}

func TestUpdateStatus_PartialSync(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "status-partial",
			Namespace:  "status-ns",
			Generation: 1,
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "status-ns",
			SecretName:      []string{"s"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	failedNs := map[string]string{"ns2": "connection refused"}
	err := r.updateStatus(context.Background(), ns, []string{"ns1"}, failedNs)
	if err != nil {
		t.Fatalf("updateStatus error: %v", err)
	}

	var updated syncv1.NamespaceSync
	err = c.Get(context.Background(), types.NamespacedName{Name: "status-partial", Namespace: "status-ns"}, &updated)
	if err != nil {
		t.Fatalf("failed to get updated resource: %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}

	cond := updated.Status.Conditions[0]
	if cond.Type != conditionTypeReady {
		t.Errorf("expected condition type 'Ready', got %q", cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected condition status True for partial sync, got %q", cond.Status)
	}
	if cond.Reason != "PartialSync" {
		t.Errorf("expected reason 'PartialSync', got %q", cond.Reason)
	}
	if len(updated.Status.FailedNamespaces) != 1 {
		t.Errorf("expected 1 failed namespace, got %d", len(updated.Status.FailedNamespaces))
	}
}

func TestUpdateStatus_AllFailed(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "status-all-failed",
			Namespace:  "status-ns",
			Generation: 1,
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "status-ns",
			SecretName:      []string{"s"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	failedNs := map[string]string{"ns1": "error1", "ns2": "error2"}
	err := r.updateStatus(context.Background(), ns, []string{}, failedNs)
	if err != nil {
		t.Fatalf("updateStatus error: %v", err)
	}

	var updated syncv1.NamespaceSync
	err = c.Get(context.Background(), types.NamespacedName{Name: "status-all-failed", Namespace: "status-ns"}, &updated)
	if err != nil {
		t.Fatalf("failed to get updated resource: %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}

	cond := updated.Status.Conditions[0]
	if cond.Type != conditionTypeReady {
		t.Errorf("expected condition type 'Ready', got %q", cond.Type)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected condition status False for all failed, got %q", cond.Status)
	}
	if cond.Reason != "SyncFailed" {
		t.Errorf("expected reason 'SyncFailed', got %q", cond.Reason)
	}
	if len(updated.Status.FailedNamespaces) != 2 {
		t.Errorf("expected 2 failed namespaces, got %d", len(updated.Status.FailedNamespaces))
	}
}

func TestUpdateStatus_NoNamespaces(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "status-no-ns",
			Namespace:  "status-ns",
			Generation: 1,
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "status-ns",
			SecretName:      []string{"s"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	err := r.updateStatus(context.Background(), ns, []string{}, nil)
	if err != nil {
		t.Fatalf("updateStatus error: %v", err)
	}

	var updated syncv1.NamespaceSync
	err = c.Get(context.Background(), types.NamespacedName{Name: "status-no-ns", Namespace: "status-ns"}, &updated)
	if err != nil {
		t.Fatalf("failed to get updated resource: %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}

	cond := updated.Status.Conditions[0]
	if cond.Type != conditionTypeReady {
		t.Errorf("expected condition type 'Ready', got %q", cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected condition status True for no namespaces, got %q", cond.Status)
	}
	if cond.Reason != "SyncComplete" {
		t.Errorf("expected reason 'SyncComplete', got %q", cond.Reason)
	}
	if cond.Message != "No target namespaces to sync" {
		t.Errorf("expected message 'No target namespaces to sync', got %q", cond.Message)
	}
}

func TestFindNamespaceSyncsForConfigMap(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-event-test",
			Namespace: "cm-src-ns",
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "cm-src-ns",
			TargetNamespaces: []string{"cm-tgt-ns"},
			ConfigMapName:    []string{"watched-cm"},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: client, Scheme: scheme}

	srcCm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "watched-cm", Namespace: "cm-src-ns"}}
	requests := r.findNamespaceSyncsForConfigMap(context.Background(), srcCm)
	if len(requests) == 0 {
		t.Error("expected reconcile for source configmap change")
	}

	tgtCm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "watched-cm", Namespace: "cm-tgt-ns"}}
	requests = r.findNamespaceSyncsForConfigMap(context.Background(), tgtCm)
	if len(requests) == 0 {
		t.Error("expected reconcile for target configmap change")
	}

	otherCm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "other-cm", Namespace: "cm-src-ns"}}
	requests = r.findNamespaceSyncsForConfigMap(context.Background(), otherCm)
	if len(requests) != 0 {
		t.Error("expected no reconcile for unrelated configmap")
	}
}

func TestCleanupResource_DeleteError(t *testing.T) {
	scheme := newTestScheme()

	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cleanup-del-err-ns"}}
	secret1 := &corev1.Secret{ObjectMeta: managedCopyMeta("secret1", "cleanup-del-err-ns", "cleanup-src-ns")}
	secret2 := &corev1.Secret{ObjectMeta: managedCopyMeta("secret2", "cleanup-del-err-ns", "cleanup-src-ns")}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(targetNs, secret1, secret2).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				return fmt.Errorf("delete permission denied")
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	errs := r.cleanupResource(context.Background(), "cleanup-src-ns", "cleanup-del-err-ns", []string{"secret1", "secret2"}, "secret",
		func(name, namespace string) client.Object {
			return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		})
	if len(errs) != 2 {
		t.Errorf("expected 2 errors from delete failures, got %d", len(errs))
	}
}

func TestCleanupSyncedResources_DeleteError(t *testing.T) {
	scheme := newTestScheme()

	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cleanup-del-tgt"}}
	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{Name: "cleanup-del-test", Namespace: "cleanup-del-src"},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "cleanup-del-src",
			TargetNamespaces: []string{"cleanup-del-tgt"},
			SecretName:       []string{"s1"},
			ConfigMapName:    []string{"cm1"},
		},
	}

	syncedSecret := &corev1.Secret{ObjectMeta: managedCopyMeta("s1", "cleanup-del-tgt", "cleanup-del-src")}
	syncedCm := &corev1.ConfigMap{ObjectMeta: managedCopyMeta("cm1", "cleanup-del-tgt", "cleanup-del-src")}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(targetNs, ns, syncedSecret, syncedCm).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				return fmt.Errorf("delete failed")
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	err := r.cleanupSyncedResources(context.Background(), ns)
	if err == nil {
		t.Error("expected error from cleanup with delete failures")
	}
}

func TestCreateOrUpdateResource_GetError(t *testing.T) {
	scheme := newTestScheme()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					return fmt.Errorf("api server error")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "test-ns"},
	}
	err := createOrUpdateResource(r, context.Background(), secret, &corev1.Secret{}, "secret",
		func(client.Object) error { return nil },
		func(src, dst *corev1.Secret) {})
	if err == nil {
		t.Error("expected error from Get failure")
	}
}

func TestHandleDeletionAndStatus_WithRecorder(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rec-tgt-ns"}}
	syncedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rec-secret", Namespace: "rec-tgt-ns"},
	}

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rec-del-sync",
			Namespace:         "rec-src-ns",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "rec-src-ns",
			TargetNamespaces: []string{"rec-tgt-ns"},
			SecretName:       []string{"rec-secret"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(targetNs, syncedSecret, ns).
		WithStatusSubresource(ns).
		Build()

	recorder := &fakeRecorder{events: []string{}}
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme, Recorder: recorder}

	_, err := r.handleDeletionAndStatus(context.Background(), ns)
	if err != nil {
		t.Fatalf("handleDeletionAndStatus error: %v", err)
	}

	if len(recorder.events) == 0 {
		t.Error("expected at least one event recorded")
	}
}

// fakeRecorder implements record.EventRecorder for testing
type fakeRecorder struct {
	events []string
}

func (f *fakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	f.events = append(f.events, reason)
}

func (f *fakeRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	f.events = append(f.events, reason)
}

func (f *fakeRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	f.events = append(f.events, reason)
}

func TestReconcile_KeepsUnmanagedTargets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deleting bool
	}{
		{"cleanup on NamespaceSync deletion", true},
		{"source resource missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := newTestScheme()

			// Same names the spec lists, but not synced from own-src-ns.
			userSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-secret", Namespace: "own-tgt-ns"},
				Data:       map[string][]byte{"key": []byte("user-data")},
			}
			otherSourceCm := &corev1.ConfigMap{
				ObjectMeta: managedCopyMeta("shared-cm", "own-tgt-ns", "other-src-ns"),
				Data:       map[string]string{"key": "other-data"},
			}

			ns := &syncv1.NamespaceSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "own-test",
					Namespace:  "own-src-ns",
					Finalizers: []string{finalizerName},
				},
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace:  "own-src-ns",
					TargetNamespaces: []string{"own-tgt-ns"},
					SecretName:       []string{"shared-secret"},
					ConfigMapName:    []string{"shared-cm"},
				},
			}
			if tc.deleting {
				now := metav1.Now()
				ns.DeletionTimestamp = &now
			}

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "own-src-ns"}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "own-tgt-ns"}},
					userSecret, otherSourceCm, ns,
				).
				WithStatusSubresource(ns).
				Build()

			r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

			if _, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "own-test", Namespace: "own-src-ns"},
			}); err != nil {
				t.Fatalf("reconcile error: %v", err)
			}

			var s corev1.Secret
			if err := c.Get(context.Background(), types.NamespacedName{Name: "shared-secret", Namespace: "own-tgt-ns"}, &s); err != nil {
				t.Errorf("expected unannotated secret to survive, got %v", err)
			}
			var cm corev1.ConfigMap
			if err := c.Get(context.Background(), types.NamespacedName{Name: "shared-cm", Namespace: "own-tgt-ns"}, &cm); err != nil {
				t.Errorf("expected configmap synced from another source to survive, got %v", err)
			}
		})
	}
}

func TestDeleteManagedCopy_GetError(t *testing.T) {
	scheme := newTestScheme()
	deleteCalled := false

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return fmt.Errorf("api server error")
			},
			Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				deleteCalled = true
				return cl.Delete(ctx, obj, opts...)
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	deleted, err := r.deleteManagedCopy(context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "get-err-ns"}}, "src-ns")
	if err == nil || deleted || deleteCalled {
		t.Errorf("expected the Get error and no delete, got deleted=%v err=%v deleteCalled=%v", deleted, err, deleteCalled)
	}
}

func TestCleanupResource_MissingCopyIsNotCountedAsCleanup(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	before := counterValue(t, "missing-copy-ns", "secret")
	errs := r.cleanupResource(context.Background(), "src-ns", "missing-copy-ns", []string{"absent"}, "secret",
		func(name, namespace string) client.Object {
			return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		})
	if len(errs) != 0 {
		t.Errorf("expected no errors for a missing copy, got %v", errs)
	}
	if after := counterValue(t, "missing-copy-ns", "secret"); after != before {
		t.Errorf("expected cleanup success counter unchanged, got %v -> %v", before, after)
	}
}

func counterValue(t *testing.T, namespace, resourceType string) float64 {
	t.Helper()
	var m dto.Metric
	if err := cleanupSuccessCounter.WithLabelValues(namespace, resourceType).Write(&m); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestReconcile_DeletionWithMalformedFilter(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	syncedSecret := &corev1.Secret{ObjectMeta: managedCopyMeta("bf-secret", "bf-tgt-ns", "bf-src-ns")}
	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bad-filter",
			Namespace:         "bf-src-ns",
			DeletionTimestamp: &now,
			Finalizers:        []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace:  "bf-src-ns",
			TargetNamespaces: []string{"bf-tgt-ns"},
			SecretName:       []string{"bf-secret"},
			ResourceFilters: &syncv1.ResourceFilters{
				Secrets: &syncv1.ResourceFilter{Exclude: []string{"["}},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "bf-src-ns"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "bf-tgt-ns"}},
			syncedSecret, ns,
		).
		WithStatusSubresource(ns).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-filter", Namespace: "bf-src-ns"},
	}); err != nil {
		t.Fatalf("expected deletion to proceed despite the malformed filter, got %v", err)
	}

	var s corev1.Secret
	if err := c.Get(context.Background(), types.NamespacedName{Name: "bf-secret", Namespace: "bf-tgt-ns"}, &s); !apierrors.IsNotFound(err) {
		t.Errorf("expected the synced copy to be cleaned up, got %v", err)
	}
	var got syncv1.NamespaceSync
	err := c.Get(context.Background(), types.NamespacedName{Name: "bad-filter", Namespace: "bf-src-ns"}, &got)
	if err == nil && len(got.Finalizers) > 0 {
		t.Errorf("expected the finalizer to be removed, got %v", got.Finalizers)
	}
}

func TestReconcile_MalformedFilterIsTerminal(t *testing.T) {
	scheme := newTestScheme()

	ns := &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "bad-glob",
			Namespace:  "bg-src-ns",
			Finalizers: []string{finalizerName},
		},
		Spec: syncv1.NamespaceSyncSpec{
			SourceNamespace: "bg-src-ns",
			SecretName:      []string{"s"},
			ResourceFilters: &syncv1.ResourceFilters{
				ConfigMaps: &syncv1.ResourceFilter{Include: []string{"app-*[0-9"}},
			},
		},
		// Left over from an earlier valid sync, plus the pseudo entry earlier versions wrote for a validation failure.
		Status: syncv1.NamespaceSyncStatus{
			SyncedNamespaces: []string{"old-target"},
			FailedNamespaces: map[string]string{"validation": "stale"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithStatusSubresource(ns).
		Build()

	rec := &fakeRecorder{}
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme, Recorder: rec}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "bad-glob", Namespace: "bg-src-ns"},
	})
	if !errors.Is(err, reconcile.TerminalError(nil)) {
		t.Errorf("expected a terminal error, got %v", err)
	}
	if !slices.Contains(rec.events, "ValidationFailed") {
		t.Errorf("expected a ValidationFailed event, got %v", rec.events)
	}

	var got syncv1.NamespaceSync
	if err := c.Get(context.Background(), types.NamespacedName{Name: "bad-glob", Namespace: "bg-src-ns"}, &got); err != nil {
		t.Fatalf("get NamespaceSync: %v", err)
	}
	if len(got.Status.Conditions) == 0 {
		t.Fatal("expected a Ready condition")
	}
	cond := got.Status.Conditions[0]
	if cond.Status != metav1.ConditionFalse || cond.Reason != "InvalidSpec" || !strings.Contains(cond.Message, "invalid pattern") {
		t.Errorf("expected Ready False/InvalidSpec carrying the validation error, got %s/%s %q", cond.Status, cond.Reason, cond.Message)
	}
	if len(got.Status.FailedNamespaces) != 0 || len(got.Status.SyncedNamespaces) != 0 {
		t.Errorf("expected per-namespace results to be cleared for an invalid spec, got synced %v failed %v",
			got.Status.SyncedNamespaces, got.Status.FailedNamespaces)
	}
}

func TestDeleteManagedCopy_StaleResourceVersion(t *testing.T) {
	scheme := newTestScheme()
	synced := &corev1.Secret{ObjectMeta: managedCopyMeta("rv-secret", "rv-ns", "rv-src")}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(synced).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := cl.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				// What a read taken before a concurrent change would have seen.
				obj.SetResourceVersion("1")
				return nil
			},
		}).
		Build()

	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	deleted, err := r.deleteManagedCopy(context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "rv-secret", Namespace: "rv-ns"}}, "rv-src")
	if deleted || !apierrors.IsConflict(err) {
		t.Errorf("expected a conflict and no delete, got deleted=%v err=%v", deleted, err)
	}
	var s corev1.Secret
	if err := c.Get(context.Background(), types.NamespacedName{Name: "rv-secret", Namespace: "rv-ns"}, &s); err != nil {
		t.Errorf("expected the copy to survive a stale delete, got %v", err)
	}
}

func TestDeleteManagedCopy_UsesAPIReader(t *testing.T) {
	scheme := newTestScheme()
	synced := &corev1.Secret{ObjectMeta: managedCopyMeta("fresh-secret", "fresh-ns", "fresh-src")}
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(synced).Build()

	// A cache that has not seen the copy yet.
	cached := interceptor.NewClient(live, interceptor.Funcs{
		Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return apierrors.NewNotFound(corev1.Resource("secrets"), key.Name)
		},
	})

	r := &NamespaceSyncReconciler{Client: cached, Scheme: scheme, APIReader: live}

	deleted, err := r.deleteManagedCopy(context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fresh-secret", Namespace: "fresh-ns"}}, "fresh-src")
	if err != nil || !deleted {
		t.Errorf("expected the copy found through APIReader to be deleted, got deleted=%v err=%v", deleted, err)
	}
}

// newSecretSync returns a NamespaceSync, finalizer already set, that syncs secrets from source into targets.
func newSecretSync(name, source string, targets []string, secrets ...string) *syncv1.NamespaceSync {
	return &syncv1.NamespaceSync{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: source, Finalizers: []string{finalizerName}},
		Spec:       syncv1.NamespaceSyncSpec{SourceNamespace: source, TargetNamespaces: targets, SecretName: secrets},
	}
}

func namespaceObjects(names ...string) []client.Object {
	objs := make([]client.Object, 0, len(names))
	for _, name := range names {
		objs = append(objs, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}
	return objs
}

// sharedSecret returns the "shared" secret the conflict tests sync, holding value under "key".
func sharedSecret(namespace, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: namespace},
		Data:       map[string][]byte{"key": []byte(value)},
	}
}

func reconcileSync(t *testing.T, r *NamespaceSyncReconciler, ns *syncv1.NamespaceSync) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: ns.Name, Namespace: ns.Namespace},
	}); err != nil {
		t.Fatalf("reconcile %s: %v", ns.Name, err)
	}
}

func getSharedSecret(t *testing.T, c client.Client, namespace string) *corev1.Secret {
	t.Helper()
	var s corev1.Secret
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: "shared"}, &s); err != nil {
		t.Fatalf("get secret %s/shared: %v", namespace, err)
	}
	return &s
}

func failedNamespaces(t *testing.T, c client.Client, ns *syncv1.NamespaceSync) map[string]string {
	t.Helper()
	var got syncv1.NamespaceSync
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(ns), &got); err != nil {
		t.Fatalf("get NamespaceSync %s: %v", ns.Name, err)
	}
	return got.Status.FailedNamespaces
}

func TestReconcile_LeavesAnotherSyncsSourceObject(t *testing.T) {
	scheme := newTestScheme()

	// sync-a targets every namespace, so it reaches b-src, where sync-b reads "shared" as its source.
	syncA := newSecretSync("sync-a", "a-src", nil, "shared")
	syncA.Spec.ConfigMapName = []string{"cfg"}
	syncB := newSecretSync("sync-b", "b-src", []string{"b-tgt"}, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a-src", "b-src", "plain-tgt")...).
		WithObjects(
			sharedSecret("a-src", fromA),
			sharedSecret("b-src", fromB),
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "a-src"}, Data: map[string]string{"key": "value"}},
			syncA, syncB,
		).
		WithStatusSubresource(syncA, syncB).
		Build()
	rec := &fakeRecorder{}
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme, Recorder: rec}

	reconcileSync(t, r, syncA)

	if got := string(getSharedSecret(t, c, "b-src").Data["key"]); got != fromB {
		t.Errorf("expected sync-b's source to keep its data, got %q", got)
	}
	if got := string(getSharedSecret(t, c, "plain-tgt").Data["key"]); got != fromA {
		t.Errorf("expected a namespace no peer claims to be synced, got %q", got)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "b-src", Name: "cfg"}, &corev1.ConfigMap{}); err != nil {
		t.Errorf("expected the rest of the conflicting namespace to sync, got %v", err)
	}
	if msg := failedNamespaces(t, c, syncA)["b-src"]; !strings.Contains(msg, "is the source of NamespaceSync b-src/sync-b") {
		t.Errorf("expected b-src to be reported as a conflict, got %q", msg)
	}
	if !slices.Contains(rec.events, "SyncConflict") {
		t.Errorf("expected a SyncConflict event, got %v", rec.events)
	}
}

func TestReconcile_LeavesCopyAnotherSyncOwns(t *testing.T) {
	scheme := newTestScheme()

	syncA := newSecretSync("sync-a", "a-src", []string{"tgt"}, "shared")
	syncB := newSecretSync("sync-b", "b-src", []string{"tgt"}, "shared")
	copyFromB := &corev1.Secret{
		ObjectMeta: managedCopyMeta("shared", "tgt", "b-src"),
		Data:       map[string][]byte{"key": []byte(fromB)},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a-src", "b-src", "tgt")...).
		WithObjects(sharedSecret("a-src", fromA), copyFromB, syncA, syncB).
		WithStatusSubresource(syncA, syncB).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncA)

	if got := string(getSharedSecret(t, c, "tgt").Data["key"]); got != fromB {
		t.Errorf("expected sync-b's copy to be left alone, got %q", got)
	}
	if msg := failedNamespaces(t, c, syncA)["tgt"]; !strings.Contains(msg, "is synced from namespace b-src by NamespaceSync b-src/sync-b") {
		t.Errorf("expected tgt to be reported as a conflict, got %q", msg)
	}
}

func TestReconcile_TakesOverOrphanedCopy(t *testing.T) {
	for _, targets := range [][]string{{"tgt"}, nil} {
		t.Run(fmt.Sprintf("targetNamespaces=%v", targets), func(t *testing.T) {
			scheme := newTestScheme()

			syncA := newSecretSync("sync-a", "a-src", targets, "shared")
			// No NamespaceSync syncs from gone-src any more.
			orphan := &corev1.Secret{
				ObjectMeta: managedCopyMeta("shared", "tgt", "gone-src"),
				Data:       map[string][]byte{"key": []byte("stale")},
			}

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespaceObjects("a-src", "tgt")...).
				WithObjects(sharedSecret("a-src", fromA), orphan, syncA).
				WithStatusSubresource(syncA).
				Build()
			r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

			reconcileSync(t, r, syncA)

			s := getSharedSecret(t, c, "tgt")
			if string(s.Data["key"]) != fromA || s.Annotations[AnnotationSourceNamespace] != "a-src" {
				t.Errorf("expected the orphaned copy to be taken over, got data %q from %q", s.Data["key"], s.Annotations[AnnotationSourceNamespace])
			}
			if failed := failedNamespaces(t, c, syncA); len(failed) != 0 {
				t.Errorf("expected no conflicts, got %v", failed)
			}
		})
	}
}

func TestReconcile_DefaultTargetKeepsHandMadeObject(t *testing.T) {
	scheme := newTestScheme()

	// b-src holds the hand-made source of a NamespaceSync that is being recreated, so no peer claims it now.
	syncA := newSecretSync("sync-a", "a-src", nil, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a-src", "b-src", "plain-tgt")...).
		WithObjects(sharedSecret("a-src", fromA), sharedSecret("b-src", fromB), syncA).
		WithStatusSubresource(syncA).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncA)

	if got := string(getSharedSecret(t, c, "b-src").Data["key"]); got != fromB {
		t.Errorf("expected the hand-made object to keep its data, got %q", got)
	}
	if got := string(getSharedSecret(t, c, "plain-tgt").Data["key"]); got != fromA {
		t.Errorf("expected a namespace without the object to be synced, got %q", got)
	}
	if msg := failedNamespaces(t, c, syncA)["b-src"]; !strings.Contains(msg, "secret b-src/shared was not created by a NamespaceSync") {
		t.Errorf("expected b-src to be reported as a conflict, got %q", msg)
	}
}

func TestReconcile_DeletionKeepsCopyAnotherSyncUses(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	syncA := newSecretSync("sync-a", "src", []string{"both-tgt", "a-tgt"}, "shared")
	syncA.DeletionTimestamp = &now
	syncB := newSecretSync("sync-b", "src", []string{"both-tgt"}, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("src", "both-tgt", "a-tgt")...).
		WithObjects(
			&corev1.Secret{ObjectMeta: managedCopyMeta("shared", "both-tgt", "src")},
			&corev1.Secret{ObjectMeta: managedCopyMeta("shared", "a-tgt", "src")},
			syncA, syncB,
		).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncA)

	getSharedSecret(t, c, "both-tgt")
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "a-tgt", Name: "shared"}, &corev1.Secret{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected the copy only sync-a used to be deleted, got %v", err)
	}
	var got syncv1.NamespaceSync
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(syncA), &got); err == nil && len(got.Finalizers) > 0 {
		t.Errorf("expected the finalizer to be removed, got %v", got.Finalizers)
	}
}

func TestReconcile_PeerListError(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleting=%v", deleting), func(t *testing.T) {
			scheme := newTestScheme()
			ns := newSecretSync("peer-err", "pe-src", []string{"pe-tgt"}, "s")
			if deleting {
				now := metav1.Now()
				ns.DeletionTimestamp = &now
			}

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespaceObjects("pe-src", "pe-tgt")...).
				WithObjects(ns).
				WithStatusSubresource(ns).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*syncv1.NamespaceSyncList); ok {
							return fmt.Errorf("list failed")
						}
						return cl.List(ctx, list, opts...)
					},
				}).
				Build()
			r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

			if _, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "peer-err", Namespace: "pe-src"},
			}); err == nil {
				t.Error("expected the NamespaceSync list error to surface")
			}
			var got syncv1.NamespaceSync
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(ns), &got); err != nil || !slices.Contains(got.Finalizers, finalizerName) {
				t.Errorf("expected the finalizer to stay until cleanup can run, got %v (err %v)", got.Finalizers, err)
			}
		})
	}
}

func TestReconcile_ExplicitTargetChains(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// sync-x lists b, where sync-p reads "shared" as its source: a chain a -> b -> c.
	syncX := newSecretSync("sync-x", "a", []string{"b"}, "shared")
	syncP := newSecretSync("sync-p", "b", []string{"c"}, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a", "b", "c")...).
		WithObjects(sharedSecret("a", fromA), syncX, syncP).
		WithStatusSubresource(syncX, syncP).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncX)
	if s := getSharedSecret(t, c, "b"); string(s.Data["key"]) != fromA || s.Annotations[AnnotationSourceNamespace] != "a" {
		t.Fatalf("expected sync-x to write its copy into sync-p's source, got data %q from %q", s.Data["key"], s.Annotations[AnnotationSourceNamespace])
	}
	if failed := failedNamespaces(t, c, syncX); len(failed) != 0 {
		t.Errorf("expected no conflicts for an explicit target, got %v", failed)
	}

	upstream := getSharedSecret(t, c, "a")
	upstream.Data["key"] = []byte("rotated")
	if err := c.Update(ctx, upstream); err != nil {
		t.Fatalf("rotate upstream: %v", err)
	}
	reconcileSync(t, r, syncX)
	if got := string(getSharedSecret(t, c, "b").Data["key"]); got != "rotated" {
		t.Errorf("expected the rotation to reach the chained copy, got %q", got)
	}

	if err := c.Delete(ctx, upstream); err != nil {
		t.Fatalf("delete upstream: %v", err)
	}
	reconcileSync(t, r, syncX)
	if err := c.Get(ctx, types.NamespacedName{Namespace: "b", Name: "shared"}, &corev1.Secret{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected deleting the upstream to remove the chained copy, got %v", err)
	}
}

func TestReconcile_ExplicitTargetKeepsHandAuthoredSource(t *testing.T) {
	scheme := newTestScheme()

	syncX := newSecretSync("sync-x", "a", []string{"b"}, "shared")
	syncP := newSecretSync("sync-p", "b", []string{"c"}, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a", "b", "c")...).
		WithObjects(sharedSecret("a", fromA), sharedSecret("b", "hand-made"), syncX, syncP).
		WithStatusSubresource(syncX, syncP).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncX)

	if got := string(getSharedSecret(t, c, "b").Data["key"]); got != "hand-made" {
		t.Errorf("expected sync-p's hand-authored source to be kept, got %q", got)
	}
	if msg := failedNamespaces(t, c, syncX)["b"]; !strings.Contains(msg, "is the source of NamespaceSync b/sync-p") {
		t.Errorf("expected b to be reported as a conflict, got %q", msg)
	}
}

func TestReconcile_ExplicitTargetOutranksDefault(t *testing.T) {
	scheme := newTestScheme()

	// sync-d reaches t only because its targetNamespaces is empty; sync-x lists t.
	syncD := newSecretSync("sync-d", "d", nil, "shared")
	syncX := newSecretSync("sync-x", "x", []string{"t"}, "shared")
	copyFromD := &corev1.Secret{
		ObjectMeta: managedCopyMeta("shared", "t", "d"),
		Data:       map[string][]byte{"key": []byte("from-d")},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("d", "x", "t")...).
		WithObjects(sharedSecret("d", "from-d"), sharedSecret("x", "from-x"), copyFromD, syncD, syncX).
		WithStatusSubresource(syncD, syncX).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncX)
	if s := getSharedSecret(t, c, "t"); string(s.Data["key"]) != "from-x" || s.Annotations[AnnotationSourceNamespace] != "x" {
		t.Fatalf("expected the explicit target to take the copy over, got data %q from %q", s.Data["key"], s.Annotations[AnnotationSourceNamespace])
	}

	reconcileSync(t, r, syncD)
	if got := string(getSharedSecret(t, c, "t").Data["key"]); got != "from-x" {
		t.Errorf("expected sync-d to leave the copy with sync-x, got %q", got)
	}
	if msg := failedNamespaces(t, c, syncD)["t"]; !strings.Contains(msg, "is synced from namespace x by NamespaceSync x/sync-x") {
		t.Errorf("expected t to be reported as a conflict for sync-d, got %q", msg)
	}
}

func TestReconcile_MissingSourceReportsNoConflict(t *testing.T) {
	scheme := newTestScheme()

	// sync-d's own source secret is gone, so it has nothing to write into sync-p's source.
	syncD := newSecretSync("sync-d", "d", nil, "shared")
	syncP := newSecretSync("sync-p", "b", []string{"c"}, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("d", "b")...).
		WithObjects(sharedSecret("b", fromB), syncD, syncP).
		WithStatusSubresource(syncD, syncP).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncD)

	if failed := failedNamespaces(t, c, syncD); len(failed) != 0 {
		t.Errorf("expected no conflicts while the source is missing, got %v", failed)
	}
	if got := string(getSharedSecret(t, c, "b").Data["key"]); got != fromB {
		t.Errorf("expected sync-p's source to be left alone, got %q", got)
	}
}

func TestReconcile_ReportsEveryConflictInANamespace(t *testing.T) {
	scheme := newTestScheme()

	syncA := newSecretSync("sync-a", "a-src", nil, "shared")
	syncA.Spec.ConfigMapName = []string{"cfg"}
	syncB := newSecretSync("sync-b", "b-src", []string{"b-tgt"}, "shared")
	syncB.Spec.ConfigMapName = []string{"cfg"}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a-src", "b-src")...).
		WithObjects(
			sharedSecret("a-src", fromA),
			sharedSecret("b-src", fromB),
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "a-src"}, Data: map[string]string{"key": fromA}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "b-src"}, Data: map[string]string{"key": fromB}},
			syncA, syncB,
		).
		WithStatusSubresource(syncA, syncB).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncA)

	msg := failedNamespaces(t, c, syncA)["b-src"]
	for _, want := range []string{"secret b-src/shared", "configmap b-src/cfg", "; "} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected the b-src report to contain %q, got %q", want, msg)
		}
	}
	var cfg corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "b-src", Name: "cfg"}, &cfg); err != nil || cfg.Data["key"] != fromB {
		t.Errorf("expected sync-b's configmap source to be left alone, got %v (err %v)", cfg.Data, err)
	}
}

func TestReconcile_DeletionKeepsChainedCopy(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	syncX := newSecretSync("sync-x", "a", []string{"b"}, "shared")
	syncX.DeletionTimestamp = &now
	syncP := newSecretSync("sync-p", "b", []string{"c"}, "shared")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("a", "b", "c")...).
		WithObjects(&corev1.Secret{ObjectMeta: managedCopyMeta("shared", "b", "a")}, syncX, syncP).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncX)

	getSharedSecret(t, c, "b")
}

func TestReconcile_DeletionWithDeletingPeerRemovesSharedCopy(t *testing.T) {
	scheme := newTestScheme()
	now := metav1.Now()

	// Both are being deleted, as with kubectl delete --all or a namespace deletion.
	syncA := newSecretSync("sync-a", "src", []string{"tgt"}, "shared")
	syncB := newSecretSync("sync-b", "src", []string{"tgt"}, "shared")
	syncA.DeletionTimestamp, syncB.DeletionTimestamp = &now, &now

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("src", "tgt")...).
		WithObjects(&corev1.Secret{ObjectMeta: managedCopyMeta("shared", "tgt", "src")}, syncA, syncB).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncA)

	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "tgt", Name: "shared"}, &corev1.Secret{}); !apierrors.IsNotFound(err) {
		t.Errorf("expected a copy shared only with a deleting peer to be removed, got %v", err)
	}
}

func TestReconcile_SameSourcePeersShareACopy(t *testing.T) {
	scheme := newTestScheme()

	syncA := newSecretSync("sync-a", "src", []string{"tgt"}, "shared")
	syncB := newSecretSync("sync-b", "src", []string{"tgt"}, "shared")
	stale := &corev1.Secret{
		ObjectMeta: managedCopyMeta("shared", "tgt", "src"),
		Data:       map[string][]byte{"key": []byte("stale")},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObjects("src", "tgt")...).
		WithObjects(sharedSecret("src", "fresh"), stale, syncA, syncB).
		WithStatusSubresource(syncA, syncB).
		Build()
	r := &NamespaceSyncReconciler{Client: c, Scheme: scheme}

	reconcileSync(t, r, syncA)

	if got := string(getSharedSecret(t, c, "tgt").Data["key"]); got != "fresh" {
		t.Errorf("expected the shared copy to be updated, got %q", got)
	}
	if failed := failedNamespaces(t, c, syncA); len(failed) != 0 {
		t.Errorf("expected peers with the same source not to conflict, got %v", failed)
	}
}
