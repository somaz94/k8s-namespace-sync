package controller

import (
	"context"
	"fmt"
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
)

// Shared test fixtures for the controller package tests.
const (
	// conditionTypeReady is the metav1.Condition Type that updateStatus sets on
	// the NamespaceSync status.
	conditionTypeReady = "Ready"
	// updatedValue is the post-update payload written to a source ConfigMap /
	// Secret / label, then asserted on the synced target copy.
	updatedValue = "new-value"
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
