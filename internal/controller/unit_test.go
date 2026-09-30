package controller

import (
	"context"
	"testing"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestValidateNamespaceSync(t *testing.T) {
	tests := []struct {
		name    string
		sync    *syncv1.NamespaceSync
		wantErr bool
	}{
		{
			name: "valid with secret",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"my-secret"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with configmap",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					ConfigMapName:   []string{"my-configmap"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with both",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"my-secret"},
					ConfigMapName:   []string{"my-configmap"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty sourceNamespace",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "",
					SecretName:      []string{"my-secret"},
				},
			},
			wantErr: true,
		},
		{
			name: "no secrets or configmaps",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
				},
			},
			wantErr: true,
		},
		{
			name: "empty slices for both",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{},
					ConfigMapName:   []string{},
				},
			},
			wantErr: true,
		},
		{
			name: "valid filter patterns",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"my-secret"},
					ResourceFilters: &syncv1.ResourceFilters{
						Secrets:    &syncv1.ResourceFilter{Include: []string{"my-*"}, Exclude: []string{"*-bak"}},
						ConfigMaps: &syncv1.ResourceFilter{Exclude: []string{"[a-c]*"}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty resourceFilters",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"my-secret"},
					ResourceFilters: &syncv1.ResourceFilters{},
				},
			},
			wantErr: false,
		},
		{
			name: "malformed secrets include pattern",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"my-secret"},
					ResourceFilters: &syncv1.ResourceFilters{
						Secrets: &syncv1.ResourceFilter{Include: []string{"[invalid"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "malformed configMaps exclude pattern",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					ConfigMapName:   []string{"my-configmap"},
					ResourceFilters: &syncv1.ResourceFilters{
						ConfigMaps: &syncv1.ResourceFilter{Exclude: []string{"app-[z-"}},
					},
				},
			},
			wantErr: true,
		},
		{
			// path.Match stops at the first chunk that fails; the error here is after the '*'.
			name: "malformed chunk after a star",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					SecretName:      []string{"my-secret"},
					ResourceFilters: &syncv1.ResourceFilters{
						Secrets: &syncv1.ResourceFilter{Include: []string{"app-*[0-9"}, Exclude: []string{"*a*\\"}},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamespaceSync(tt.sync)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNamespaceSync() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		item  string
		want  bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"nil slice", nil, "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.slice, tt.item); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldSyncResource(t *testing.T) {
	r := &NamespaceSyncReconciler{}

	tests := []struct {
		name    string
		resName string
		filter  *syncv1.ResourceFilter
		want    bool
	}{
		{"nil filter", "anything", nil, true},
		{"no patterns", "anything", &syncv1.ResourceFilter{}, true},
		{"exclude match", "backup-secret", &syncv1.ResourceFilter{Exclude: []string{"backup-*"}}, false},
		{"exclude no match", "app-secret", &syncv1.ResourceFilter{Exclude: []string{"backup-*"}}, true},
		{"include match", "prod-config", &syncv1.ResourceFilter{Include: []string{"prod-*"}}, true},
		{"include no match", "dev-config", &syncv1.ResourceFilter{Include: []string{"prod-*"}}, false},
		{"include empty returns true", "anything", &syncv1.ResourceFilter{Include: []string{}}, true},
		{"exclude takes priority", "backup-prod", &syncv1.ResourceFilter{
			Include: []string{"*-prod"},
			Exclude: []string{"backup-*"},
		}, false},
		{"invalid exclude pattern excludes", "test", &syncv1.ResourceFilter{Exclude: []string{"[invalid"}}, false},
		{"invalid include pattern ignored", "test", &syncv1.ResourceFilter{Include: []string{"[invalid"}}, false},
		{"exact match include", "my-config", &syncv1.ResourceFilter{Include: []string{"my-config"}}, true},
		{"exact match exclude", "my-config", &syncv1.ResourceFilter{Exclude: []string{"my-config"}}, false},
		{"wildcard all include", "anything", &syncv1.ResourceFilter{Include: []string{"*"}}, true},
		{"multiple excludes", "test-backup", &syncv1.ResourceFilter{Exclude: []string{"*-old", "*-backup"}}, false},
		{"multiple includes first match", "prod-cm", &syncv1.ResourceFilter{Include: []string{"prod-*", "staging-*"}}, true},
		{"multiple includes second match", "staging-cm", &syncv1.ResourceFilter{Include: []string{"prod-*", "staging-*"}}, true},
		{"multiple includes no match", "dev-cm", &syncv1.ResourceFilter{Include: []string{"prod-*", "staging-*"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.shouldSyncResource(tt.resName, tt.filter); got != tt.want {
				t.Errorf("shouldSyncResource(%q) = %v, want %v", tt.resName, got, tt.want)
			}
		})
	}
}

func TestIsSystemNamespace(t *testing.T) {
	r := &NamespaceSyncReconciler{}

	systemNs := []string{"kube-system", "kube-public", "kube-node-lease", "default", "k8s-namespace-sync-system"}
	for _, ns := range systemNs {
		t.Run("system_"+ns, func(t *testing.T) {
			if !r.isSystemNamespace(ns) {
				t.Errorf("isSystemNamespace(%q) = false, want true", ns)
			}
		})
	}

	nonSystemNs := []string{"my-app", "production", "staging", "test-ns"}
	for _, ns := range nonSystemNs {
		t.Run("non_system_"+ns, func(t *testing.T) {
			if r.isSystemNamespace(ns) {
				t.Errorf("isSystemNamespace(%q) = true, want false", ns)
			}
		})
	}
}

func TestShouldSyncToNamespace(t *testing.T) {
	r := &NamespaceSyncReconciler{}
	ctx := context.Background()

	tests := []struct {
		name      string
		namespace string
		sync      *syncv1.NamespaceSync
		want      bool
	}{
		{
			name:      "system namespace",
			namespace: "kube-system",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{SourceNamespace: "source-ns"},
			},
			want: false,
		},
		{
			name:      "source namespace",
			namespace: "source-ns",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{SourceNamespace: "source-ns"},
			},
			want: false,
		},
		{
			name:      "excluded namespace",
			namespace: "excluded-ns",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace: "source-ns",
					Exclude:         []string{"excluded-ns"},
				},
			},
			want: false,
		},
		{
			name:      "target namespaces specified - in list",
			namespace: "target-ns",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace:  "source-ns",
					TargetNamespaces: []string{"target-ns", "other-ns"},
				},
			},
			want: true,
		},
		{
			name:      "target namespaces specified - not in list",
			namespace: "not-target-ns",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{
					SourceNamespace:  "source-ns",
					TargetNamespaces: []string{"target-ns"},
				},
			},
			want: false,
		},
		{
			name:      "no target namespaces - should sync",
			namespace: "any-ns",
			sync: &syncv1.NamespaceSync{
				Spec: syncv1.NamespaceSyncSpec{SourceNamespace: "source-ns"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.shouldSyncToNamespace(ctx, tt.namespace, tt.sync); got != tt.want {
				t.Errorf("shouldSyncToNamespace(%q) = %v, want %v", tt.namespace, got, tt.want)
			}
		})
	}
}

func TestCopyLabelsAndAnnotations(t *testing.T) {
	r := &NamespaceSyncReconciler{}

	t.Run("copies custom labels and annotations, skips kubernetes.io", func(t *testing.T) {
		src := &metav1.ObjectMeta{
			Name:      "source",
			Namespace: "source-ns",
			Labels: map[string]string{
				"app":                      "myapp",
				"kubernetes.io/managed-by": "helm",
			},
			Annotations: map[string]string{
				"custom":                    "value",
				"kubernetes.io/description": "skip-this",
			},
		}
		dst := &metav1.ObjectMeta{}

		r.copyLabelsAndAnnotations(src, dst)

		if dst.Labels["app"] != "myapp" {
			t.Errorf("expected label 'app'='myapp', got %q", dst.Labels["app"])
		}
		if _, ok := dst.Labels["kubernetes.io/managed-by"]; ok {
			t.Error("kubernetes.io/ label should not be copied")
		}
		if dst.Annotations["custom"] != "value" {
			t.Errorf("expected annotation 'custom'='value', got %q", dst.Annotations["custom"])
		}
		if _, ok := dst.Annotations["kubernetes.io/description"]; ok {
			t.Error("kubernetes.io/ annotation should not be copied")
		}
		if dst.Annotations[AnnotationSourceNamespace] != "source-ns" {
			t.Error("expected sync metadata annotation for source-namespace")
		}
		if dst.Annotations[AnnotationSourceName] != "source" {
			t.Error("expected sync metadata annotation for source-name")
		}
	})

	t.Run("handles nil source labels and annotations", func(t *testing.T) {
		src := &metav1.ObjectMeta{
			Name:      "source",
			Namespace: "ns",
		}
		dst := &metav1.ObjectMeta{}

		r.copyLabelsAndAnnotations(src, dst)

		if dst.Labels == nil {
			t.Error("dst.Labels should be initialized")
		}
		if dst.Annotations == nil {
			t.Error("dst.Annotations should be initialized")
		}
	})

	t.Run("handles existing dst labels and annotations", func(t *testing.T) {
		src := &metav1.ObjectMeta{
			Name:      "source",
			Namespace: "ns",
			Labels:    map[string]string{"new-label": updatedValue},
		}
		dst := &metav1.ObjectMeta{
			Labels:      map[string]string{"existing": "keep"},
			Annotations: map[string]string{"existing-ann": "keep"},
		}

		r.copyLabelsAndAnnotations(src, dst)

		if dst.Labels["existing"] != "keep" {
			t.Error("existing label should be preserved")
		}
		if dst.Labels["new-label"] != updatedValue {
			t.Error("new label should be added")
		}
	})
}

func TestIsManagedCopy(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{"no annotations", nil, false},
		{"managed", map[string]string{AnnotationSourceNamespace: "src", AnnotationSourceName: "obj"}, true},
		{"other source namespace", map[string]string{AnnotationSourceNamespace: "other", AnnotationSourceName: "obj"}, false},
		{"other source name", map[string]string{AnnotationSourceNamespace: "src", AnnotationSourceName: "renamed"}, false},
		{"namespace annotation only", map[string]string{AnnotationSourceNamespace: "src"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "obj", Namespace: "tgt", Annotations: tt.annotations}}
			if got := isManagedCopy(obj, "src"); got != tt.want {
				t.Errorf("isManagedCopy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEqualIgnoringLastSync(t *testing.T) {
	base := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "obj",
				Namespace:   "tgt",
				Labels:      map[string]string{"app": "demo"},
				Annotations: map[string]string{AnnotationLastSync: "2026-01-01T00:00:00Z", "note": "a"},
			},
			Data: map[string][]byte{"key": []byte("value")},
		}
	}

	tests := []struct {
		name   string
		mutate func(a, b *corev1.Secret)
		want   bool
	}{
		{"identical", func(a, b *corev1.Secret) {}, true},
		{"only last-sync differs", func(a, b *corev1.Secret) { b.Annotations[AnnotationLastSync] = "2026-01-01T00:00:01Z" }, true},
		{"last-sync missing on one side", func(a, b *corev1.Secret) { delete(b.Annotations, AnnotationLastSync) }, true},
		{"stamp only vs no annotations", func(a, b *corev1.Secret) {
			a.Annotations = map[string]string{AnnotationLastSync: "2026-01-01T00:00:00Z"}
			b.Annotations = nil
		}, true},
		{"nil vs empty labels", func(a, b *corev1.Secret) { a.Labels, b.Labels = nil, map[string]string{} }, true},
		{"data differs", func(a, b *corev1.Secret) { b.Data["key"] = []byte("other") }, false},
		{"label differs", func(a, b *corev1.Secret) { b.Labels["app"] = "other" }, false},
		{"other annotation differs", func(a, b *corev1.Secret) { b.Annotations["note"] = "b" }, false},
		{"type differs", func(a, b *corev1.Secret) { b.Type = corev1.SecretTypeTLS }, false},
		{"stamp and data differ", func(a, b *corev1.Secret) {
			b.Annotations[AnnotationLastSync] = "2026-01-01T00:00:01Z"
			b.Data["key"] = []byte("other")
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := base(), base()
			tt.mutate(a, b)
			if got := equalIgnoringLastSync(a, b); got != tt.want {
				t.Errorf("equalIgnoringLastSync() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("leaves its arguments untouched", func(t *testing.T) {
		a, b := base(), base()
		equalIgnoringLastSync(a, b)
		for name, s := range map[string]*corev1.Secret{"a": a, "b": b} {
			if _, ok := s.Annotations[AnnotationLastSync]; !ok {
				t.Errorf("expected the last-sync annotation on %s to survive the comparison", name)
			}
		}
	})
}

func TestSyncsName(t *testing.T) {
	r := &NamespaceSyncReconciler{}
	ns := &syncv1.NamespaceSync{Spec: syncv1.NamespaceSyncSpec{
		SecretName:      []string{"app-secret", "app-secret-bak"},
		ConfigMapName:   []string{"app-config"},
		ResourceFilters: &syncv1.ResourceFilters{Secrets: &syncv1.ResourceFilter{Exclude: []string{"*-bak"}}},
	}}

	tests := []struct {
		resourceType string
		name         string
		want         bool
	}{
		{"secret", "app-secret", true},
		{"secret", "app-secret-bak", false},
		{"secret", "app-config", false},
		{"configmap", "app-config", true},
		{"configmap", "missing", false},
		{"pod", "app-secret", false},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType+"/"+tt.name, func(t *testing.T) {
			if got := r.syncsName(ns, tt.resourceType, tt.name); got != tt.want {
				t.Errorf("syncsName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNamespaceSyncPredicate(t *testing.T) {
	old := &syncv1.NamespaceSync{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns", Generation: 1}}

	tests := []struct {
		name   string
		mutate func(*syncv1.NamespaceSync)
		want   bool
	}{
		{"status write", func(n *syncv1.NamespaceSync) { n.Status.SyncedNamespaces = []string{"tgt"} }, false},
		{"finalizer added", func(n *syncv1.NamespaceSync) { n.Finalizers = []string{finalizerName} }, false},
		{"spec edit", func(n *syncv1.NamespaceSync) { n.Generation = 2 }, true},
		{"deletion started", func(n *syncv1.NamespaceSync) {
			now := metav1.Now()
			n.DeletionTimestamp, n.Generation = &now, 2
		}, true},
		{"annotation added", func(n *syncv1.NamespaceSync) { n.Annotations = map[string]string{"resync": "1"} }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := old.DeepCopy()
			tt.mutate(updated)
			if got := namespaceSyncPredicate().Update(event.UpdateEvent{ObjectOld: old, ObjectNew: updated}); got != tt.want {
				t.Errorf("Update() = %v, want %v", got, tt.want)
			}
		})
	}

	p := namespaceSyncPredicate()
	if !p.Create(event.CreateEvent{Object: old}) || !p.Delete(event.DeleteEvent{Object: old}) {
		t.Error("expected create and delete events to pass")
	}
}
