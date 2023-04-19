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
	"fmt"
	"sort"

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

// ErrorOccurred / ValidRepositoryURL / ValidRepositoryCredential
const (
	GitOpsDeploymentRepositoryCredentialConditionErrorOccurred             = "ErrorOccurred"
	GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl        = "ValidRepositoryURL"
	GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential = "ValidRepositoryCredential"
)

// GitOpsDeploymentRepositoryCredentialStatus defines the observed state of GitOpsDeploymentRepositoryCredential
type GitOpsDeploymentRepositoryCredentialStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Conditions []metav1.Condition `json:"conditions,omitempty"`
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

const (
	RepositoryCredentialReasonErrorOccurred        = "ErrorOccurred"
	RepositoryCredentialReasonCredentialsUpToDate  = "RepositoryCredentialUpToDate"
	RepositoryCredentialReasonSecretNotSpecified   = "SecretNotSpecified"
	RepositoryCredentialReasonSecretNotFound       = "SecretNotFound"
	RepositoryCredentialReasonInvalidCredentials   = "InvalidCredentials"
	RepositoryCredentialReasonInValidRepositoryUrl = "InvalidRepositoryUrl"
	RepositoryCredentialReasonValidRepositoryUrl   = "ValidRepositoryUrl"
)

// SetConditions updates the GitOpsDeploymentRepositoryCredential status conditions for a subset of evaluated types.
// If the GitOpsDeploymentRepositoryCredential has a pre-existing condition of a type that is not in the evaluated list,
// it will be preserved. If the GitOpsDeploymentRepositoryCredential has a pre-existing condition of a type, status, reason that
// is in the evaluated list, but not in the incoming conditions list, it will be removed.
func (status *GitOpsDeploymentRepositoryCredentialStatus) SetConditions(conditions []metav1.Condition) {
	repoCredConditions := make([]metav1.Condition, 0)
	now := metav1.Now()
	for i := range conditions {
		condition := conditions[i]
		eci := findConditionIndex(status.Conditions, condition.Type)
		if eci >= 0 && status.Conditions[eci].Message == condition.Message && status.Conditions[eci].Reason == condition.Reason && status.Conditions[eci].Status == condition.Status {
			// If we already have a condition of this type, status and reason, only update the timestamp if something
			// has changed.
			repoCredConditions = append(repoCredConditions, status.Conditions[eci])
		} else {
			// Otherwise we use the new incoming condition with an updated timestamp:
			condition.LastTransitionTime = now
			repoCredConditions = append(repoCredConditions, condition)
		}
	}
	sort.Slice(repoCredConditions, func(i, j int) bool {
		left := repoCredConditions[i]
		right := repoCredConditions[j]
		return fmt.Sprintf("%s/%s/%s/%s/%v", left.Type, left.Message, left.Status, left.Reason, left.LastTransitionTime) < fmt.Sprintf("%s/%s/%s/%s/%v", right.Type, right.Message, right.Status, right.Reason, right.LastTransitionTime)
	})
	status.Conditions = repoCredConditions
}

func findConditionIndex(conditions []metav1.Condition, t string) int {
	for i := range conditions {
		if conditions[i].Type == t {
			return i
		}
	}
	return -1
}
