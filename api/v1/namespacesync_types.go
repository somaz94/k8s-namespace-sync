package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceSyncSpec defines the desired state of NamespaceSync
type NamespaceSyncSpec struct {
	// SourceNamespace is the namespace to sync from
	SourceNamespace string `json:"sourceNamespace"`

	// TargetNamespaces is the list of namespaces to sync to
	// +optional
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// ConfigMapName is the name of the ConfigMap to sync
	// +optional
	ConfigMapName []string `json:"configMapName,omitempty"`

	// SecretName is the name of the Secret to sync
	// +optional
	SecretName []string `json:"secretName,omitempty"`

	// Exclude is the list of namespaces to exclude from sync
	// +optional
	Exclude []string `json:"exclude,omitempty"`

	// ResourceFilters defines filters for different resource types
	// +optional
	ResourceFilters *ResourceFilters `json:"resourceFilters,omitempty"`
}

// NamespaceSyncStatus defines the observed state of NamespaceSync
type NamespaceSyncStatus struct {
	// LastSyncTime is the last time the sync was performed
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`

	// SyncedNamespaces is a list of namespaces that were successfully synced
	SyncedNamespaces []string `json:"syncedNamespaces,omitempty"`

	// FailedNamespaces maps namespace names to error messages for failed syncs
	FailedNamespaces map[string]string `json:"failedNamespaces,omitempty"`

	// Conditions represent the latest available observations of an object's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration represents the .metadata.generation that the condition was set based upon
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ResourceFilter defines include/exclude patterns for resources
type ResourceFilter struct {
	// Include patterns for resources (if empty, all resources are included)
	// +optional
	Include []string `json:"include,omitempty"`
	// Exclude patterns for resources
	// +optional
	Exclude []string `json:"exclude,omitempty"`
}

// ResourceFilters defines filters for different resource types
type ResourceFilters struct {
	// Secrets defines filters for secrets
	// +optional
	Secrets *ResourceFilter `json:"secrets,omitempty"`
	// ConfigMaps defines filters for configmaps
	// +optional
	ConfigMaps *ResourceFilter `json:"configMaps,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.sourceNamespace"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].message"

// NamespaceSync is the Schema for the namespacesyncs API
type NamespaceSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceSyncSpec   `json:"spec,omitempty"`
	Status NamespaceSyncStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NamespaceSyncList contains a list of NamespaceSync
type NamespaceSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NamespaceSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceSync{}, &NamespaceSyncList{})
}
