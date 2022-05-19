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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationSnapshotEnvironmentBindingSpec defines the desired state of ApplicationSnapshotEnvironmentBinding
type ApplicationSnapshotEnvironmentBindingSpec struct {
	Application string                                           `json:"application"`
	Environment string                                           `json:"environment"`
	Snapshot    string                                           `json:"snapshot"`
	Components  []ApplicationSnapshotEnvironmentBindingComponent `json:"components"`
}

type ApplicationSnapshotEnvironmentBindingComponent struct {
	Name             string                                                        `json:"name"`
	Configuration    ApplicationSnapshotEnvironmentBindingComponentConfiguration   `json:"configuration"`
	GitOpsRepository ApplicationSnapshotEnvironmentBindingComponentGitOpsRepostory `json:"gitopsRepository"`
	Components       []ApplicationSnapshotEnvironmentBindingComponentConfiguration `json:"components"`
}

type ApplicationSnapshotEnvironmentBindingComponentConfiguration struct {
	// NOTE: The specific fields, and their form, to be included are TBD.

	// API discussion concluded with no obvious need for target port; it is thus excluded here.
	// Let us know if you have a requirement here.
	//
	// TargetPort int `json:"targetPort"`

	Replicas int `json:"replicas"`
	// Resources defines the Compute Resources required by the component
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	Env       []EnvVarPair                 `json:"env,omitempty"`
}

type EnvVarPair struct {
	// Name is the environment variable name
	Name string `json:"name"`
	// Value is the environment variable value
	Value string `json:"value"`
}

type ApplicationSnapshotEnvironmentBindingComponentGitOpsRepostory struct {
	// Git repository URL
	// e.g. The Git repository that contains the K8s resources to deployment for the component of the application.
	URL    string `json:"url"`
	Branch string `json:"branch"`
	// Path to a directory in the GitOps repo, containing a kustomization.yaml
	//  NOTE: Each component-env combination must have it's own separate path
	Path string `json:"path"`
}

// ApplicationSnapshotEnvironmentBindingStatus defines the observed state of ApplicationSnapshotEnvironmentBinding
type ApplicationSnapshotEnvironmentBindingStatus struct {
	GitOpsDeployments []ApplicationSnapshotEnvironmentBindingStatusGitOpsDeployment `json:"gitopsDeployments,omitempty"`
}

type ApplicationSnapshotEnvironmentBindingStatusGitOpsDeployment struct {
	// Each component would have a corresponding GitOpsDeployment, and the reference to the name of that GitOps deployment would be here. Health/sync status can be found here.
	ComponentName    string `json:"componentName"`
	GitOpsDeployment string `json:"gitopsDeployment,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ApplicationSnapshotEnvironmentBinding is the Schema for the applicationsnapshotenvironmentbindings API
type ApplicationSnapshotEnvironmentBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSnapshotEnvironmentBindingSpec   `json:"spec,omitempty"`
	Status ApplicationSnapshotEnvironmentBindingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ApplicationSnapshotEnvironmentBindingList contains a list of ApplicationSnapshotEnvironmentBinding
type ApplicationSnapshotEnvironmentBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationSnapshotEnvironmentBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationSnapshotEnvironmentBinding{}, &ApplicationSnapshotEnvironmentBindingList{})
}
