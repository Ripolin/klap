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

// ServerSpec defines the desired state of Server
type ServerSpec struct {

	// BaseDN is the base distinguished name for the server
	// +required
	BaseDN *string `json:"baseDN"`

	// BindDN is the distinguished name to bind as when connecting to the server
	// +required
	BindDN *string `json:"bindDN"`

	// PasswordSecretRef is a reference to a secret containing the password to bind with
	// +required
	PasswordSecretRef SecretRef `json:"passwordSecretRef"`

	// Implementation is the server implementation type (e.g., "openldap", "activedirectory")
	// +enum=openldap;activedirectory
	// +default="openldap"
	// +required
	Implementation *string `json:"implementation"`

	// TlsSecretRef is a reference to a secret containing TLS configuration for the server
	// +optional
	TlsSecretRef SecretRef `json:"tlsSecretRef,omitzero"`

	// Url is the URL of the server
	// +required
	Url *string `json:"url"`

	// StartTLS indicates whether to use StartTLS when connecting to the server
	// +default=false
	// +required
	StartTLS *bool `json:"startTLS"`
}

// SecretRef defines a reference to a kubernetes secret
type SecretRef struct {

	// Name of a secret
	// +required
	Name *string `json:"name"`

	// Namespace of a secret
	// +required
	Namespace *string `json:"namespace"`

	// Key within a secret
	// +required
	Key *string `json:"key"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="IMPLEMENTATION",type=string,JSONPath=`.spec.implementation`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Server
	// +required
	Spec ServerSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
