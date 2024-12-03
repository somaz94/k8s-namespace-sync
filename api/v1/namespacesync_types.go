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
}

// NamespaceSyncStatus defines the observed state of NamespaceSync
type NamespaceSyncStatus struct {
	// LastSyncTime is the last time the sync was performed
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// SyncedNamespaces is the list of namespaces that have been synced
	// +optional
	SyncedNamespaces []string `json:"syncedNamespaces,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.sourceNamespace"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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
