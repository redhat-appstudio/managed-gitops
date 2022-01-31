/*
Copyright 2021, 2022

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

// GitOpsDeploymentSpec defines the desired state of GitOpsDeployment
type GitOpsDeploymentSpec struct {
	Source ApplicationSource `json:"source"`

	// Destination is a reference to a target namespace/cluster to deploy to.
	// This field may be empty: if it is empty, it is assumed that the destination
	// is the same namespace as the GitOpsDeployment CR.
	Destination ApplicationDestination `json:"destination,omitempty"`

	// Two possible values:
	// - Automated: whenever a new commit occurs in the GitOps repository, or the Argo CD Application is out of sync, Argo CD should be told to (re)synchronize.
	// - Manual: Argo CD should never be told to resynchronize. Instead, synchronize operations will be triggered via GitOpsDeploymentSyncRun operations only.
	// - See `GitOpsDeploymentSpecType*`
	//
	// Note: This is somewhat of a placeholder for more advanced logic that can be implemented in the future.
	// For an example of this type of logic, see the 'syncPolicy' field of Argo CD Application.
	Type string `json:"type"`
}

// ApplicationSource contains all required information about the source of an application
type ApplicationSource struct {
	// RepoURL is the URL to the repository (Git or Helm) that contains the application manifests
	RepoURL string `json:"repoURL"`
	// Path is a directory path within the Git repository, and is only valid for applications sourced from Git.
	Path string `json:"path,omitempty"`
	// TargetRevision defines the revision of the source to sync the application to.
	// In case of Git, this can be commit, tag, or branch. If omitted, will equal to HEAD.
	// In case of Helm, this is a semver tag for the Chart's version.
	TargetRevision string `json:"targetRevision,omitempty"`
}

// ApplicationDestination holds information about the application's destination
type ApplicationDestination struct {

	// The namespace will only be set for namespace-scoped resources that have not set a value for .metadata.namespace
	Namespace string `json:"namespace,omitempty"`
}

const (
	GitOpsDeploymentSpecType_Automated = "automated"
	GitOpsDeploymentSpecType_Manual    = "manual"
)

// GitOpsDeploymentStatus defines the observed state of GitOpsDeployment
type GitOpsDeploymentStatus struct {
	Conditions []GitOpsDeploymentCondition `json:"conditions,omitempty"`
	Sync       SyncStatus                  `json:"sync,omitempty" protobuf:"bytes,2,opt,name=sync"`
	// Health contains information about the application's current health status
	Health HealthStatus `json:"health,omitempty" protobuf:"bytes,3,opt,name=health"`
}

// HealthStatus contains information about the currently observed health state of an application or resource
type HealthStatus struct {
	// Status holds the status code of the application or resource
	Status HealthStatusCode `json:"status,omitempty" protobuf:"bytes,1,opt,name=status"`
	// Message is a human-readable informational message describing the health status
	Message string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
}

type HealthStatusCode string

// SyncStatus contains information about the currently observed live and desired states of an application
type SyncStatus struct {
	// Status is the sync state of the comparison
	Status SyncStatusCode `json:"status" protobuf:"bytes,1,opt,name=status,casttype=SyncStatusCode"`
	// Revision contains information about the revision the comparison has been performed to
	Revision string `json:"revision,omitempty" protobuf:"bytes,3,opt,name=revision"`
}

// SyncStatusCode is a type which represents possible comparison results
type SyncStatusCode string

// Possible comparison results
const (
	// SyncStatusCodeUnknown indicates that the status of a sync could not be reliably determined
	SyncStatusCodeUnknown SyncStatusCode = "Unknown"
	// SyncStatusCodeOutOfSync indicates that desired and live states match
	SyncStatusCodeSynced SyncStatusCode = "Synced"
	// SyncStatusCodeOutOfSync indicates that there is a drift beween desired and live states
	SyncStatusCodeOutOfSync SyncStatusCode = "OutOfSync"
)

// GitOpsDeploymentCondition contains details about an applicationset condition, which is usally an error or warning
type GitOpsDeploymentCondition struct {
	// Type is an applicationset condition type
	Type GitOpsDeploymentConditionType `json:"type" protobuf:"bytes,1,opt,name=type"`

	// Message contains human-readable message indicating details about condition
	Message string `json:"message" protobuf:"bytes,2,opt,name=message"`

	// LastTransitionTime is the time the condition was last observed
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`

	// True/False/Unknown
	Status GitOpsConditionStatus `json:"status" protobuf:"bytes,4,opt,name=status"`

	//Single word camelcase representing the reason for the status eg ErrorOccurred
	Reason GitOpsDeploymentReasonType `json:"reason" protobuf:"bytes,5,opt,name=reason"`
}

// GitOpsDeploymentConditionType represents type of GitOpsDeployment condition.
type GitOpsDeploymentConditionType string

const (
	GitOpsDeploymentConditionErrorOccurred GitOpsDeploymentConditionType = "ErrorOccurred"
)

// SyncStatusCode is a type which represents possible comparison results
type GitOpsConditionStatus string

// Application Condition Status
const (
	// GitOpsConditionStatusTrue indicates that a condition type is true
	GitOpsConditionStatusTrue GitOpsConditionStatus = "True"
	// GitOpsConditionStatusFalse indicates that a condition type is false
	GitOpsConditionStatusFalse GitOpsConditionStatus = "False"
	// GitOpsConditionStatusUnknown indicates that the condition status could not be reliably determined
	GitOpsConditionStatusUnknown GitOpsConditionStatus = "Unknown"
)

type GitOpsDeploymentReasonType string

const (
	GitopsDeploymentReasonErrorOccurred GitOpsDeploymentReasonType = "ErrorOccurred"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GitOpsDeployment is the Schema for the gitopsdeployments API
type GitOpsDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitOpsDeploymentSpec   `json:"spec,omitempty"`
	Status GitOpsDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitOpsDeploymentList contains a list of GitOpsDeployment
type GitOpsDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitOpsDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitOpsDeployment{}, &GitOpsDeploymentList{})
}
