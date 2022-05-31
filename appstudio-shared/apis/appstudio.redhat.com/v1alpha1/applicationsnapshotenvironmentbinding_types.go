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

	// Application is a reference to the Application resource (defined in the namespace) involved in the binding
	Application string `json:"application"`

	// Environment is the Environment resource (defined in the namespace) that the binding will deploy to
	Environment string `json:"environment"`

	// Snapshot is the Snapshot resource (defined in the namespae) that contains the container image versions for the components of the Application
	Snapshot string `json:"snapshot"`

	// Components contains individual component data
	Components []BindingComponent `json:"components"`
}

// BindingComponent contains individual component data
type BindingComponent struct {

	// Name is the name of the component.
	Name string `json:"name"`

	// Configuration describes GitOps repository customizations that are specific to the
	// the component-application-environment combination.
	// - Values defined in this struct will overwrite values from Application/Environment/Component
	Configuration BindingComponentConfiguration `json:"configuration,omitempty"`

	// GitOpsRepository contains the Git URL, path, and branch, for the component
	GitOpsRepository BindingComponentGitOpsRepository `json:"gitopsRepository"`
}

// BindingComponentConfiguration describes GitOps repository customizations that are specific to the
// the component-application-environment combination.
type BindingComponentConfiguration struct {
	// NOTE: The specific fields, and their form, to be included are TBD.

	// API discussion concluded with no obvious need for target port; it is thus excluded here.
	// Let us know if you have a requirement here.
	//
	// TargetPort int `json:"targetPort"`

	// Replicas defines the number of replicas to use for the component
	Replicas int `json:"replicas"`

	// Resources defines the Compute Resources required by the component
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env describes environment variables to use for the component
	Env []EnvVarPair `json:"env,omitempty"`
}

// EnvVarPair describes environment variables to use for the component
type EnvVarPair struct {

	// Name is the environment variable name
	Name string `json:"name"`

	// Value is the environment variable value
	Value string `json:"value"`
}

// BindingComponentGitOpsRepository is a reference to a GitOps repository, including path/branch
// where the application/component/environment resources can be found (usually via a kustomize overlay).
type BindingComponentGitOpsRepository struct {

	// URL is the Git repository URL
	// e.g. The Git repository that contains the K8s resources to deployment for the component of the application.
	URL string `json:"url"`

	// Branch is the branch to use when accessing the GitOps repository
	Branch string `json:"branch"`

	// Path is a pointer to a folder in the GitOps repo, containing a kustomization.yaml
	// NOTE: Each component-env combination must have it's own separate path
	Path string `json:"path"`
}

// ApplicationSnapshotEnvironmentBindingStatus defines the observed state of ApplicationSnapshotEnvironmentBinding
type ApplicationSnapshotEnvironmentBindingStatus struct {

	// GitOpsDeployments describes the set of GitOpsDeployment resources that correspond to the binding.
	// To determine the health/sync status of a binding, you can look at the GitOpsDeployments decribed here.
	GitOpsDeployments []BindingStatusGitOpsDeployment `json:"gitopsDeployments,omitempty"`
}

// BindingStatusGitOpsDeployment describes an individual reference
// to a GitOpsDeployment resources that is used to deploy this binding.
//
// To determine the health/sync status of a binding, you can look at the GitOpsDeployments decribed here.
type BindingStatusGitOpsDeployment struct {

	// ComponentName is the name of the component in the (component, gitopsdeployment) pair
	ComponentName string `json:"componentName"`

	// GitOpsDeployment is a reference to the name of a GitOpsDeployment resource which is used to deploy the binding.
	// The Health/sync status for the binding can thus be read from the references GitOpsDEployment
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
