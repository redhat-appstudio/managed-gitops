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

	// Snapshot refers to snapshot to promote between environments.
	Snapshot string `json:"snapshot"`

	// Application is the name of the Application to target
	Application string `json:"application"`

	// ManualPromotion is for fields specific to manual promotion.
	// Only one field should be defined: either 'manualPromotion' or 'automatedPromotion', but not both.
	ManualPromotion ApplicationPromotionRunManual `json:"manualPromotion,omitempty"`

	// AutomatedPromotion is for fields specific to automated promotion
	// Only one field should be defined: either 'manualPromotion' or 'automatedPromotion', but not both.
	AutomatedPromotion ApplicationPromotionRunAutomated `json:"automatedPromotion,omitempty"`
}

type ApplicationPromotionRunManual struct {
	// TargetEnvironment is the environment to promote to
	TargetEnvironment string `json:"targetEnvironment"`
}

type ApplicationPromotionRunAutomated struct {
	// Start iterating through the digraph, beginning with the value specified in 'initialEnvironment'
	InitialEnvironment string `json:"initialEnvironment"`
}

type ApplicationPromotionRunState string

const (
	ApplicationPromotionRunState_Active   ApplicationPromotionRunState = "Active"
	ApplicationPromotionRunState_Waiting  ApplicationPromotionRunState = "Waiting"
	ApplicationPromotionRunState_Complete ApplicationPromotionRunState = "Complete"
)

type ApplicationPromotionRunCompleteResult string

const (
	ApplicationPromotionRunCompleteResult_Success ApplicationPromotionRunCompleteResult = "Success"
	ApplicationPromotionRunCompleteResult_Failure ApplicationPromotionRunCompleteResult = "Failure"
)

// ApplicationPromotionRunStatus defines the observed state of ApplicationPromotionRun
type ApplicationPromotionRunStatus struct {

	// State indicates whether or not the overall promotion (either manual or automated is complete)
	State ApplicationPromotionRunState `json:"state"`

	// CompletionResult indicates success/failure once the promotion has completed all work.
	CompletionResult  ApplicationPromotionRunCompleteResult    `json:"completionResult,omitempty"`
	EnvironmentStatus ApplicationPromotionRunEnvironmentStatus `json:"environmentStatus,omitempty"`

	// ActiveBindings is the list of active bindings currently being promoted to:
	// - For an automated promotion, there can be multiple active bindings at a time (one for each env at a particular tree depth)
	// - For a manual promotion, there will be only one.
	ActiveBindings []string `json:"activeBindings,omitempty"`
}

type ApplicationPromotionRunEnvironmentStatusField string

const (
	ApplicationPromotionRunEnvironmentStatus_Success    ApplicationPromotionRunEnvironmentStatusField = "Success"
	ApplicationPromotionRunEnvironmentStatus_InProgress ApplicationPromotionRunEnvironmentStatusField = "In Progress"
	ApplicationPromotionRunEnvironmentStatus_Failed     ApplicationPromotionRunEnvironmentStatusField = "Failed"
)

type ApplicationPromotionRunEnvironmentStatus struct {
	Step            int                                           `json:"step"`
	EnvironmentName string                                        `json:"environmentName"`
	Status          ApplicationPromotionRunEnvironmentStatusField `json:"status"`
}

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
