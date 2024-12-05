package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceSyncSpec defines the desired state of NamespaceSync
type NamespaceSyncSpec struct {
	// SourceNamespace is the namespace containing the resources to be synced
	// +kubebuilder:validation:Required
	SourceNamespace string `json:"sourceNamespace"`

	// SecretName is the name of the secret to be synced
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ConfigMapName is the name of the configmap to be synced
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// Exclude is a list of namespaces to exclude from synchronization
	// +optional
	Exclude []string `json:"exclude,omitempty"`
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
