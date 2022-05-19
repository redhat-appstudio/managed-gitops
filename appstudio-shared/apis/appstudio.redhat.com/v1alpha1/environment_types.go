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
	Type string `json:"type"`

	DisplayName        string                 `json:"displayName"`
	DeploymentStrategy DeploymentStrategyType `json:"deploymentStrategy"`

	ParentEnvironment string `json:"parentEnvironment,omitempty"`

	Tags []string `json:"tags,omitempty"`

	Configuration EnvironmentConfiguration `json:"configuration,omitempty"`
}

type EnvironmentType string

const (
	EnvironmentType_POC    EnvironmentType = "POC"
	EnvironmentType_NonPOC EnvironmentType = "Non-POC"
)

type DeploymentStrategyType string

const (
	DeploymentStrategy_Manual             DeploymentStrategyType = "Manual"
	DeploymentStrategy_AppStudioAutomated DeploymentStrategyType = "AppStudioAutomated"
)

type EnvironmentConfiguration struct {
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
