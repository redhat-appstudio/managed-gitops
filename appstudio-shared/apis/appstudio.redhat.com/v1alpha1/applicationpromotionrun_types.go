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

// ApplicationPromotionRunSpec defines the desired state of ApplicationPromotionRun
type ApplicationPromotionRunSpec struct {

	// NOTE: The name (kind) of this API, "ApplicationPromotionRun" is likely to change in the short term (Q2 2022).
	// Stay tuned for refactoring needed for your component.

	// Snapshot refers to the name of a Snapshot resource defined within the namespace, used to promote container images between Environments.
	Snapshot string `json:"snapshot"`

	// Application is the name of an Application resource defined within the namespaced, and which is the target of the promotion
	Application string `json:"application"`

	// ManualPromotion is for fields specific to manual promotion.
	// Only one field should be defined: either 'manualPromotion' or 'automatedPromotion', but not both.
	ManualPromotion ManualPromotionConfiguration `json:"manualPromotion,omitempty"`

	// AutomatedPromotion is for fields specific to automated promotion
	// Only one field should be defined: either 'manualPromotion' or 'automatedPromotion', but not both.
	AutomatedPromotion AutomatedPromotionConfiguration `json:"automatedPromotion,omitempty"`
}

// ManualPromotionConfiguration defines promotion parameters specific to manual promotion: the target environment to promote to.
type ManualPromotionConfiguration struct {
	// TargetEnvironment is the environment to promote to
	TargetEnvironment string `json:"targetEnvironment"`
}

// AutomatedPromotionConfiguration defines promotion parameters specific to automated promotion: the initial environment
// (in the promotion graph) to begin promoting on.
type AutomatedPromotionConfiguration struct {
	// InitialEnvironment: start iterating through the digraph, beginning with the value specified in 'initialEnvironment'
	InitialEnvironment string `json:"initialEnvironment"`
}

// ApplicationPromotionRunStatus defines the observed state of ApplicationPromotionRun
type ApplicationPromotionRunStatus struct {

	// State indicates whether or not the overall promotion (either manual or automated is complete)
	State PromotionRunState `json:"state"`

	// CompletionResult indicates success/failure once the promotion has completed all work.
	// CompletionResult will only have a value if State field is 'Complete'.
	CompletionResult PromotionRunCompleteResult `json:"completionResult,omitempty"`

	// EnvironmentStatus represents the set of steps taken during the  current promotion
	EnvironmentStatus []PromotionRunEnvironmentStatus `json:"environmentStatus,omitempty"`

	// ActiveBindings is the list of active bindings currently being promoted to:
	// - For an automated promotion, there can be multiple active bindings at a time (one for each env at a particular tree depth)
	// - For a manual promotion, there will be only one.
	ActiveBindings []string `json:"activeBindings,omitempty"`
}

// PromotionRunState defines the 3 states of an ApplicationPromotion resource.
type PromotionRunState string

const (
	PromotionRunState_Active   PromotionRunState = "Active"
	PromotionRunState_Waiting  PromotionRunState = "Waiting"
	PromotionRunState_Complete PromotionRunState = "Complete"
)

// PromotionRunCompleteResult defines the success/failure states if the PromotionRunState is 'Complete'.
type PromotionRunCompleteResult string

const (
	PromotionRunCompleteResult_Success PromotionRunCompleteResult = "Success"
	PromotionRunCompleteResult_Failure PromotionRunCompleteResult = "Failure"
)

// PromotionRunEnvironmentStatus represents the set of steps taken during the  current promotion:
// - manual promotions will only have a single step.
// - automated promotions may have one or more steps, depending on how many environments have been promoted to.
type PromotionRunEnvironmentStatus struct {

	// Step is the sequential number of the step in the array, starting with 1
	Step int `json:"step"`

	// EnvironmentName is the name of the environment that was promoted to in this step
	EnvironmentName string `json:"environmentName"`

	// Status is/was the result of promoting to that environment.
	Status PromotionRunEnvironmentStatusField `json:"status"`
}

// PromotionRunEnvironmentStatusField are the state values for promotion to individual enviroments, as
// used by the Status field of ApplicationPromotionRunEnvironmentStatus
type PromotionRunEnvironmentStatusField string

const (
	ApplicationPromotionRunEnvironmentStatus_Success    PromotionRunEnvironmentStatusField = "Success"
	ApplicationPromotionRunEnvironmentStatus_InProgress PromotionRunEnvironmentStatusField = "In Progress"
	ApplicationPromotionRunEnvironmentStatus_Failed     PromotionRunEnvironmentStatusField = "Failed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ApplicationPromotionRun is the Schema for the applicationpromotionruns API
type ApplicationPromotionRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationPromotionRunSpec   `json:"spec,omitempty"`
	Status ApplicationPromotionRunStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ApplicationPromotionRunList contains a list of ApplicationPromotionRun
type ApplicationPromotionRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationPromotionRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationPromotionRun{}, &ApplicationPromotionRunList{})
}
