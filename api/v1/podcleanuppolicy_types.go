package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodCleanupPolicySpec defines the desired state of PodCleanupPolicy
type PodCleanupPolicySpec struct {
	// Schedule is a cron expression for when to run cleanup (e.g., "*/5 * * * *").
	// If not set, cleanup runs on every reconcile.
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// NamespaceSelector selects namespaces to scan for pods.
	// If not set, all namespaces are scanned.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// PodSelector selects pods to consider for cleanup.
	// If not set, all pods in the target namespaces are considered.
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`

	// PodStatuses is a list of pod phases to clean up (e.g., Failed, Succeeded).
	// If not set, all phases are eligible.
	// +optional
	PodStatuses []corev1.PodPhase `json:"podStatuses,omitempty"`

	// MaxAge is the maximum age of pods to retain (e.g., "24h", "1h30m").
	// Pods older than this will be candidates for deletion.
	// +optional
	MaxAge string `json:"maxAge,omitempty"`

	// DryRun if true, the operator logs what it would delete without actually deleting.
	// +optional
	DryRun bool `json:"dryRun,omitempty"`
}

// PodCleanupPolicyStatus defines the observed state of PodCleanupPolicy
type PodCleanupPolicyStatus struct {
	// LastRunTime is the timestamp of the last cleanup run.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// PodsDeleted is the cumulative number of pods deleted by this policy.
	// +optional
	PodsDeleted int64 `json:"podsDeleted,omitempty"`

	// LastRunPodsDeleted is the number of pods deleted (or would-be deleted) in the last run.
	// +optional
	LastRunPodsDeleted int32 `json:"lastRunPodsDeleted,omitempty"`

	// Conditions represents the latest available observations of the policy's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=pcp
//+kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
//+kubebuilder:printcolumn:name="DryRun",type=boolean,JSONPath=`.spec.dryRun`
//+kubebuilder:printcolumn:name="LastRun",type=string,JSONPath=`.status.lastRunTime`
//+kubebuilder:printcolumn:name="PodsDeleted",type=integer,JSONPath=`.status.podsDeleted`

// PodCleanupPolicy is the Schema for the podcleanuppolicies API.
// It defines rules for automatically cleaning up pods based on their state and age.
type PodCleanupPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodCleanupPolicySpec   `json:"spec,omitempty"`
	Status PodCleanupPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodCleanupPolicyList contains a list of PodCleanupPolicy
type PodCleanupPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodCleanupPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodCleanupPolicy{}, &PodCleanupPolicyList{})
}
