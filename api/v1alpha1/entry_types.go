/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EntrySpec defines the desired state of Entry
type EntrySpec struct {

	// DN (distinguished name) is a unique identifier that pinpoints the entry in an LDAP directory
	// +require
	DN *string `json:"dn"`

	// Prune indicates if entry should be delete or not
	// +default:value=true
	// +require
	Prune bool `json:"prune"`

	// Attributes associated to the entry depends its classes
	// +optional
	Attributes map[string][]string `json:"attributes,omitzero"`

	// InitAttributes unlike attributes are not remediated and are only define at entry creation
	// +optional
	InitAttributes map[string][]string `json:"initAttributes,omitzero"`

	// ServerSecretRef pinpoint a secret containing LDAP server configuration
	// +required
	ServerSecretRef SecretRef `json:"serverSecretRef"`

	// TlsSecretRef pinpoint a secret containing TLS configuration
	// +optional
	TlsSecretRef SecretRef `json:"tlsSecretRef,omitzero"`
}

// SecretRef defines a reference to a kubernetes secret
type SecretRef struct {

	// Name of a secret
	// +required
	Name *string `json:"name"`

	// Namespace of a secret
	// +optional
	Namespace *string `json:"namespace"`
}

// EntryStatus defines the observed state of Entry.
type EntryStatus struct {

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Entry resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="DN",type=string,JSONPath=`.spec.dn`
// +kubebuilder:printcolumn:name="AVAILABLE",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

// Entry is the Schema for the entries API
type Entry struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Entry
	// +required
	Spec EntrySpec `json:"spec"`

	// status defines the observed state of Entry
	// +optional
	Status EntryStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// EntryList contains a list of Entry
type EntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Entry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Entry{}, &EntryList{})
}
