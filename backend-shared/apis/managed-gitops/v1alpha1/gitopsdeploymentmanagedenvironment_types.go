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

// The GitOpsDeploymentManagedEnvironment CR describes a remote cluster which the GitOps Service will deploy to, via Argo CD.
// This resource references a Secret resource, of type managed-gitops.redhat.com/managed-environment, that contains the cluster credentials.
// The Secret should contain credentials to a ServiceAccount/User account on the target cluster.
// This is referred to as the Argo CD 'ServiceAccount' below.
type GitOpsDeploymentManagedEnvironmentSpec struct {

	// APIURL is the URL of the cluster to connect to
	APIURL string `json:"apiURL"`

	// ClusterCredentialsSecret is a reference to a Secret that contains cluster connection details. The cluster details should be in the form of a kubeconfig file.
	ClusterCredentialsSecret string `json:"credentialsSecret"`

	// AllowInsecureSkipTLSVerify controls whether Argo CD will accept a Kubernetes API URL with untrusted-TLS certificate.
	// Optional: If true, the GitOps Service will allow Argo CD to connect to the specified cluster even if it is using an invalid or self-signed TLS certificate.
	// Defaults to false.
	AllowInsecureSkipTLSVerify bool `json:"allowInsecureSkipTLSVerify"`

	// CreateNewServiceAccount controls whether Argo CD will use the ServiceAccount provided by the user in the Secret, or if a new ServiceAccount
	// should be created.
	//
	// Optional, default to false.
	//
	// - If true, the GitOps Service will automatically create a ServiceAccount/ClusterRole/ClusterRoleBinding on the target cluster,
	//   using the credentials provided by the user in the secret.
	//   - Argo CD will then be configured to deploy with that new ServiceAccount.
	//
	// - Default: If false, it is assumed that the credentials provided by the user in the Secret are for a ServiceAccount on the cluster, and
	//   Argo CD will be configred to use the ServiceAccount referenced by the Secret of the user. No new ServiceAccount will be created.
	//   - This should be used, for example, when the ServiceAccount Argo CD does not have full cluster access (*/*/* at cluster scope)
	CreateNewServiceAccount bool `json:"createNewServiceAccount,omitempty"`

	// Namespaces allows one to indicate which Namespaces the Secret's ServiceAccount has access to.
	//
	// Optional, defaults to empty. If empty, it is assumed that the ServiceAccount has access to all Namespaces.
	//
	// The ServiceAccount that GitOps Service/Argo CD uses to deploy may not have access to all of the Namespaces on a cluster.
	// If not specified, it is assumed that the Argo CD ServiceAccount has read/write at cluster-scope.
	// - If you are familiar with Argo CD: this field is equivalent to the field of the same name in the Argo CD Cluster Secret.
	Namespaces []string `json:"namespaces,omitempty"`

	// ClusterResources is used in conjuction with the Namespace field.
	// If the .spec.namespaces field is non-empty, this field will be used to determine whether Argo CD should
	// attempt to manage cluster-scoped resources.
	// - If .spec.namespaces field is empty, this field is ignored.
	// - If you are familiar with Argo CD: this field is equivalent to the field of the same name in the Argo CD Cluster Secret.
	//
	// Optional, default to false.
	ClusterResources bool `json:"clusterResources,omitempty"`
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

type ManagedEnvironmentConditionReason string

const (
	ConditionReasonUnsupportedAPIURL                  ManagedEnvironmentConditionReason = "UnsupportedAPIURL"
	ConditionReasonSucceeded                          ManagedEnvironmentConditionReason = "Succeeded"
	ConditionReasonKubeError                          ManagedEnvironmentConditionReason = "KubernetesError"
	ConditionReasonDatabaseError                      ManagedEnvironmentConditionReason = "DatabaseError"
	ConditionReasonInvalidSecretType                  ManagedEnvironmentConditionReason = "InvalidSecretType"
	ConditionReasonMissingKubeConfigField             ManagedEnvironmentConditionReason = "MissingKubeConfigField"
	ConditionReasonUnableToCreateClient               ManagedEnvironmentConditionReason = "UnableToCreateClient"
	ConditionReasonUnableToCreateClusterCredentials   ManagedEnvironmentConditionReason = "UnableToCreateClusterCredentials"
	ConditionReasonUnableToInstallServiceAccount      ManagedEnvironmentConditionReason = "UnableToInstallServiceAccount"
	ConditionReasonUnableToValidateClusterCredentials ManagedEnvironmentConditionReason = "UnableToValidateClusterCredentials"
	ConditionReasonUnableToLocateContext              ManagedEnvironmentConditionReason = "UnableToLocateContext"
	ConditionReasonUnableToParseKubeconfigData        ManagedEnvironmentConditionReason = "UnableToParseKubeconfigData"
	ConditionReasonInvalidNamespaceList               ManagedEnvironmentConditionReason = "InvalidNamespaceList"
	ConditionReasonUnableToRetrieveRestConfig         ManagedEnvironmentConditionReason = "UnableToRetrieveRestConfig"
	ConditionReasonUnknownError                       ManagedEnvironmentConditionReason = "UnknownError"
)

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
