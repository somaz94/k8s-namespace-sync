package controller

import (
	"context"
	"fmt"
	"testing"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = syncv1.AddToScheme(scheme)
	return scheme
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
	if result.Requeue {
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

	// First reconcile adds finalizer
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-sync", Namespace: "src-ns"},
	})
	if err != nil {
		t.Fatalf("first reconcile error: %v", err)
	}

	// Second reconcile does the sync
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-sync", Namespace: "src-ns"},
	})
	if err != nil {
		t.Fatalf("second reconcile error: %v", err)
	}

	// Verify synced secret
	var synced corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "my-secret", Namespace: "tgt-ns"}, &synced)
	if err != nil {
		t.Errorf("expected secret to be synced to target, got error: %v", err)
	}

	// Verify synced configmap
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

	// Pre-synced secret in target
	syncedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "del-secret", Namespace: "del-tgt-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("value")},
	}

	// Pre-synced configmap in target
	syncedCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "del-cm", Namespace: "del-tgt-ns"},
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

	// Verify synced secret is cleaned up
	var s corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "del-secret", Namespace: "del-tgt-ns"}, &s)
	if err == nil {
		t.Error("expected secret to be deleted from target namespace")
	}

	// Verify synced configmap is cleaned up
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

	// Target has synced secret but source doesn't have it
	syncedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-secret", Namespace: "snf-tgt-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("old-value")},
	}

	// Target has synced configmap but source doesn't have it
	syncedCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-cm", Namespace: "snf-tgt-ns"},
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

	// Verify secret deleted from target since source doesn't have it
	var s corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Name: "missing-secret", Namespace: "snf-tgt-ns"}, &s)
	if err == nil {
		t.Error("expected secret to be deleted from target namespace when source is missing")
	}

	// Verify configmap deleted from target since source doesn't have it
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

	// app-secret should be synced (not excluded)
	var s1 corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-secret", Namespace: "rf-tgt-ns"}, &s1); err != nil {
		t.Error("app-secret should be synced to target")
	}

	// app-secret-bak should NOT be synced (excluded by *-bak)
	var s2 corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-secret-bak", Namespace: "rf-tgt-ns"}, &s2); err == nil {
		t.Error("app-secret-bak should NOT be synced to target")
	}

	// app-config should be synced (included)
	var c1 corev1.ConfigMap
	if err := client.Get(context.Background(), types.NamespacedName{Name: "app-config", Namespace: "rf-tgt-ns"}, &c1); err != nil {
		t.Error("app-config should be synced to target")
	}

	// app-config-bak should NOT be synced (not in include list)
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
		Data:       map[string][]byte{"key": []byte("new-value")},
	}
	// Pre-existing secret in target with old data
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-secret", Namespace: "upd-tgt-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("old-value")},
	}

	sourceCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cm", Namespace: "upd-src-ns"},
		Data:       map[string]string{"key": "new-value"},
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

	// Verify secret updated
	var s corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Name: "upd-secret", Namespace: "upd-tgt-ns"}, &s); err != nil {
		t.Fatalf("failed to get updated secret: %v", err)
	}
	if string(s.Data["key"]) != "new-value" {
		t.Errorf("expected secret data 'new-value', got %q", string(s.Data["key"]))
	}

	// Verify configmap updated
	var cm corev1.ConfigMap
	if err := client.Get(context.Background(), types.NamespacedName{Name: "upd-cm", Namespace: "upd-tgt-ns"}, &cm); err != nil {
		t.Fatalf("failed to get updated configmap: %v", err)
	}
	if cm.Data["key"] != "new-value" {
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

	// Source namespace change triggers reconcile
	sourceNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "event-src-ns"}}
	requests := r.findNamespaceSyncs(context.Background(), sourceNs)
	if len(requests) == 0 {
		t.Error("expected reconcile request for source namespace change")
	}

	// Target namespace change triggers reconcile
	targetNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "some-target-ns"}}
	requests = r.findNamespaceSyncs(context.Background(), targetNs)
	if len(requests) == 0 {
		t.Error("expected reconcile request for target namespace change")
	}

	// System namespace should not trigger reconcile
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

	// Source secret triggers reconcile
	srcSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "watched-secret", Namespace: "sec-src-ns"}}
	requests := r.findNamespaceSyncsForSecret(context.Background(), srcSecret)
	if len(requests) == 0 {
		t.Error("expected reconcile for source secret change")
	}

	// Target secret triggers reconcile
	tgtSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "watched-secret", Namespace: "sec-tgt-ns"}}
	requests = r.findNamespaceSyncsForSecret(context.Background(), tgtSecret)
	if len(requests) == 0 {
		t.Error("expected reconcile for target secret change")
	}

	// Unrelated secret does not trigger
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

	// Source configmap triggers reconcile
	srcCm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "watched-cm", Namespace: "cm-src-ns"}}
	requests := r.findNamespaceSyncsForConfigMap(context.Background(), srcCm)
	if len(requests) == 0 {
		t.Error("expected reconcile for source configmap change")
	}

	// Target configmap triggers reconcile
	tgtCm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "watched-cm", Namespace: "cm-tgt-ns"}}
	requests = r.findNamespaceSyncsForConfigMap(context.Background(), tgtCm)
	if len(requests) == 0 {
		t.Error("expected reconcile for target configmap change")
	}

	// Unrelated configmap does not trigger
	otherCm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "other-cm", Namespace: "cm-src-ns"}}
	requests = r.findNamespaceSyncsForConfigMap(context.Background(), otherCm)
	if len(requests) != 0 {
		t.Error("expected no reconcile for unrelated configmap")
	}
}
