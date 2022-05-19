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

// EnvironmentSpec defines the desired state of Environment
type EnvironmentSpec struct {

	// Type is whether the Environment is a POC or non-POC environment
	Type EnvironmentType `json:"type"`

	// DisplayName is the user-visible, user-definable name for the environment (but not used for functional requirements)
	DisplayName string `json:"displayName"`

	// DeploymentStrategy is the promotion strategy for the Environment
	// See Environment API doc for details.
	DeploymentStrategy DeploymentStrategyType `json:"deploymentStrategy"`

	// ParentEnvironment references another Environment defined in the namespace: when automated promotion is enabled,
	// promotions to the parent environment will cause this environment to be promoted to.
	// See Environment API doc for details.
	ParentEnvironment string `json:"parentEnvironment,omitempty"`

	// Tags are a user-visisble, user-definable set of tags that can be applied to the environment
	Tags []string `json:"tags,omitempty"`

	// Configuration contains environment-specific details for Applications/Components that are deployed to
	// the Environment.
	Configuration EnvironmentConfiguration `json:"configuration,omitempty"`
}

// EnvironmentType currently indicates whether an environment is POC/Non-POC, see API doc for details.
type EnvironmentType string

const (
	EnvironmentType_POC    EnvironmentType = "POC"
	EnvironmentType_NonPOC EnvironmentType = "Non-POC"
)

// DeploymentStrategyType defines the available promotion/deployment strategies for an Environment
// See Environment API doc for details.
type DeploymentStrategyType string

const (
	// DeploymentStrategy_Manual: Promotions to an Environment with this strategy will occur due to explicit user intent
	DeploymentStrategy_Manual DeploymentStrategyType = "Manual"

	// DeploymentStrategy_AppStudioAutomated: Promotions to an Environment with this strategy will occur if a previous ("parent")
	// environment in the environment graph was successfully promoted to.
	// See Environment API doc for details.
	DeploymentStrategy_AppStudioAutomated DeploymentStrategyType = "AppStudioAutomated"
)

// EnvironmentConfiguration contains Environment-specific configurations details, to be used when generating
// Component/Application GitOps repository resources.
type EnvironmentConfiguration struct {
	// Env is an array of standard environment vairables
	Env []EnvVarPair `json:"env"`
}

// EnvironmentStatus defines the observed state of Environment
type EnvironmentStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Environment is the Schema for the environments API
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
