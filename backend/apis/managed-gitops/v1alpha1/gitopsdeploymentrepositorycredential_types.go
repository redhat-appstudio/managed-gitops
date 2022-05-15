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

// GitOpsDeploymentRepositoryCredentialSpec defines the desired state of GitOpsDeploymentRepositoryCredential
type GitOpsDeploymentRepositoryCredentialSpec struct {

	// Repository (HTTPS url, or SSH string) for accessing the Git repo
	// Required field
	// As of this writing (Mar 2022), we only support HTTPS URL
	Repository string `json:"repository"`

	// Reference to a K8s Secret in the namespace that contains repository credentials (Git username/password, as of this writing)
	// Required field
	Secret string `json:"secret"`
}

// GitOpsDeploymentRepositoryCredentialStatus defines the observed state of GitOpsDeploymentRepositoryCredential
type GitOpsDeploymentRepositoryCredentialStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GitOpsDeploymentRepositoryCredential is the Schema for the gitopsdeploymentrepositorycredentials API
type GitOpsDeploymentRepositoryCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitOpsDeploymentRepositoryCredentialSpec   `json:"spec,omitempty"`
	Status GitOpsDeploymentRepositoryCredentialStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitOpsDeploymentRepositoryCredentialList contains a list of GitOpsDeploymentRepositoryCredential
type GitOpsDeploymentRepositoryCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitOpsDeploymentRepositoryCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitOpsDeploymentRepositoryCredential{}, &GitOpsDeploymentRepositoryCredentialList{})
}
