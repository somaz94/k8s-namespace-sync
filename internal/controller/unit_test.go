package controller

import (
	"testing"

	syncv1 "github.com/somaz94/k8s-namespace-sync/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		{"invalid exclude pattern ignored", "test", &syncv1.ResourceFilter{Exclude: []string{"[invalid"}}, true},
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

func TestShouldSkipNamespace(t *testing.T) {
	r := &NamespaceSyncReconciler{}

	tests := []struct {
		name      string
		namespace string
		sourceNs  string
		excluded  []string
		want      bool
	}{
		{"system namespace", "kube-system", "source-ns", nil, true},
		{"source namespace", "source-ns", "source-ns", nil, true},
		{"excluded namespace", "excluded-ns", "source-ns", []string{"excluded-ns"}, true},
		{"normal namespace", "target-ns", "source-ns", []string{"other-ns"}, false},
		{"not in exclude list", "target-ns", "source-ns", []string{"excluded-ns"}, false},
		{"empty exclude list", "target-ns", "source-ns", nil, false},
		{"default namespace", "default", "source-ns", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.shouldSkipNamespace(tt.namespace, tt.sourceNs, tt.excluded); got != tt.want {
				t.Errorf("shouldSkipNamespace(%q, %q, %v) = %v, want %v", tt.namespace, tt.sourceNs, tt.excluded, got, tt.want)
			}
		})
	}
}

func TestShouldSyncToNamespace(t *testing.T) {
	r := &NamespaceSyncReconciler{}

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
			if got := r.shouldSyncToNamespace(tt.namespace, tt.sync); got != tt.want {
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
		if dst.Annotations["namespacesync.nsync.dev/source-namespace"] != "source-ns" {
			t.Error("expected sync metadata annotation for source-namespace")
		}
		if dst.Annotations["namespacesync.nsync.dev/source-name"] != "source" {
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
			Labels:    map[string]string{"new-label": "new-value"},
		}
		dst := &metav1.ObjectMeta{
			Labels:      map[string]string{"existing": "keep"},
			Annotations: map[string]string{"existing-ann": "keep"},
		}

		r.copyLabelsAndAnnotations(src, dst)

		if dst.Labels["existing"] != "keep" {
			t.Error("existing label should be preserved")
		}
		if dst.Labels["new-label"] != "new-value" {
			t.Error("new label should be added")
		}
	})
}
