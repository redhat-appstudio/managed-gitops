/*
Copyright 2022.

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

const (
	ManagedEnvironmentStatusConnectionInitializationSucceeded = "ConnectionInitializationSucceeded"
)

// GitOpsDeploymentManagedEnvironmentSpec defines the desired state of GitOpsDeploymentManagedEnvironment
type GitOpsDeploymentManagedEnvironmentSpec struct {
	APIURL                     string `json:"apiURL"`
	ClusterCredentialsSecret   string `json:"credentialsSecret"`
	AllowInsecureSkipTLSVerify bool   `json:"allowInsecureSkipTLSVerify"`
}

type AllowInsecureSkipTLSVerify bool

// Insecure TLS Status types
const (
	// TLSVerifyStatusTrue indicates that a Insecure TLS verify type is true
	TLSVerifyStatusTrue AllowInsecureSkipTLSVerify = true
	// TLSVerifyStatusFalse indicates that a Insecure TLS verify type is false
	TLSVerifyStatusFalse AllowInsecureSkipTLSVerify = false
)

// GitOpsDeploymentManagedEnvironmentStatus defines the observed state of GitOpsDeploymentManagedEnvironment
type GitOpsDeploymentManagedEnvironmentStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GitOpsDeploymentManagedEnvironment is the Schema for the gitopsdeploymentmanagedenvironments API
type GitOpsDeploymentManagedEnvironment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitOpsDeploymentManagedEnvironmentSpec   `json:"spec,omitempty"`
	Status GitOpsDeploymentManagedEnvironmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitOpsDeploymentManagedEnvironmentList contains a list of GitOpsDeploymentManagedEnvironment
type GitOpsDeploymentManagedEnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitOpsDeploymentManagedEnvironment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitOpsDeploymentManagedEnvironment{}, &GitOpsDeploymentManagedEnvironmentList{})
}
