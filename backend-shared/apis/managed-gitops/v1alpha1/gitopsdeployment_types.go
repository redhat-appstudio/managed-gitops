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

	// SyncPolicy controls when and how a sync will be performed.
	SyncPolicy *SyncPolicy `json:"syncPolicy,omitempty"`

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
	Path string `json:"path"`
	// TargetRevision defines the revision of the source to sync the application to.
	// In case of Git, this can be commit, tag, or branch. If omitted, will equal to HEAD.
	// In case of Helm, this is a semver tag for the Chart's version.
	TargetRevision string `json:"targetRevision,omitempty"`
}

// ApplicationDestination holds information about the application's destination
type ApplicationDestination struct {
	Environment string `json:"environment,omitempty"`

	// The namespace will only be set for namespace-scoped resources that have not set a value for .metadata.namespace
	Namespace string `json:"namespace,omitempty"`
}

const (
	// DeletionFinalizer will indicate the GitOpsDeployment to wait until all its dependencies are removed.
	// In the absence of this finalizer, GitOpsDeployment will be deleted first and its dependencies will be removed in the background.
	DeletionFinalizer string = "resources-finalizer.managed-gitops.redhat.com"
)

type SyncOption string

// Supported values for SyncOptions
const (
	SyncOptions_CreateNamespace_true  SyncOption = "CreateNamespace=true"
	SyncOptions_CreateNamespace_false SyncOption = "CreateNamespace=false"
)

type SyncPolicy struct {
	// Options allow you to specify whole app sync-options.
	// This option may be empty, if and when it is empty it is considered that there are no SyncOptions present.
	SyncOptions SyncOptions `json:"syncOptions,omitempty"`
}
type SyncOptions []SyncOption

const (
	GitOpsDeploymentSpecType_Automated = "automated"
	GitOpsDeploymentSpecType_Manual    = "manual"
)

func SyncOptionToStringSlice(syncOptions SyncOptions) []string {
	if syncOptions == nil {
		return nil
	}
	var res []string
	if len(syncOptions) > 0 {
		res = []string{}
		for _, syncOption := range syncOptions {
			res = append(res, string(syncOption))
		}
	}

	return res
}

// ResourceStatus holds the current sync and health status of a resource
type ResourceStatus struct {
	Group     string         `json:"group,omitempty"`
	Version   string         `json:"version,omitempty"`
	Kind      string         `json:"kind,omitempty"`
	Namespace string         `json:"namespace,omitempty"`
	Name      string         `json:"name,omitempty"`
	Status    SyncStatusCode `json:"status,omitempty"`
	Health    *HealthStatus  `json:"health,omitempty"`
}

// ReconciledState contains the last version of the GitOpsDeployment resource that the ArgoCD Controller reconciled
type ReconciledState struct {
	Source      GitOpsDeploymentSource      `json:"source"`
	Destination GitOpsDeploymentDestination `json:"destination"`
}

// GitOpsDeploymentSource contains the information of .status.Sync.CompareTo.Source field of ArgoCD Application
type GitOpsDeploymentSource struct {
	// Path contains path from .status.Sync.CompareTo field of ArgoCD Application
	Path    string `json:"path"`
	RepoURL string `json:"repoURL"`
	Branch  string `json:"branch"`
}

// GitOpsDeploymentDestination contains the information of .status.Sync.CompareTo.Destination field of ArgoCD Application
type GitOpsDeploymentDestination struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// GitOpsDeploymentStatus defines the observed state of GitOpsDeployment
type GitOpsDeploymentStatus struct {
	Conditions []GitOpsDeploymentCondition `json:"conditions,omitempty"`
	Sync       SyncStatus                  `json:"sync,omitempty"`
	// Health contains information about the application's current health status
	Health HealthStatus `json:"health,omitempty"`

	// List of Resource created by a deployment
	Resources []ResourceStatus `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`

	// ReconciledState contains the last version of the GitOpsDeployment resource that the ArgoCD Controller reconciled
	ReconciledState ReconciledState `json:"reconciledState"`

	// OperationState contains information about any ongoing operations, such as a sync
	OperationState *OperationState `json:"operationState,omitempty"`
}

// OperationState contains information about state of a running operation
type OperationState struct {
	// Operation is the original requested operation
	Operation ApplicationOperation `json:"operation" protobuf:"bytes,1,opt,name=operation"`
	// Phase is the current phase of the operation
	Phase OperationPhase `json:"phase" protobuf:"bytes,2,opt,name=phase"`
	// Message holds any pertinent messages when attempting to perform operation (typically errors).
	Message string `json:"message,omitempty" protobuf:"bytes,3,opt,name=message"`
	// SyncResult is the result of a Sync operation
	SyncResult *SyncOperationResult `json:"syncResult,omitempty" protobuf:"bytes,4,opt,name=syncResult"`
	// StartedAt contains time of operation start
	StartedAt metav1.Time `json:"startedAt" protobuf:"bytes,6,opt,name=startedAt"`
	// FinishedAt contains time of operation completion
	FinishedAt *metav1.Time `json:"finishedAt,omitempty" protobuf:"bytes,7,opt,name=finishedAt"`
	// RetryCount contains time of operation retries
	RetryCount int64 `json:"retryCount,omitempty" protobuf:"bytes,8,opt,name=retryCount"`
}

// Operation contains information about a requested or running operation
type ApplicationOperation struct {
	// Sync contains parameters for the operation
	Sync *SyncOperation `json:"sync,omitempty" protobuf:"bytes,1,opt,name=sync"`
	// InitiatedBy contains information about who initiated the operations
	InitiatedBy OperationInitiator `json:"initiatedBy,omitempty" protobuf:"bytes,2,opt,name=initiatedBy"`
	// Info is a list of informational items for this operation
	Info []*Info `json:"info,omitempty" protobuf:"bytes,3,name=info"`
	// Retry controls the strategy to apply if a sync fails
	Retry RetryStrategy `json:"retry,omitempty" protobuf:"bytes,4,opt,name=retry"`
}

type Info struct {
	Name  string `json:"name" protobuf:"bytes,1,name=name"`
	Value string `json:"value" protobuf:"bytes,2,name=value"`
}

// RetryStrategy contains information about the strategy to apply when a sync failed
type RetryStrategy struct {
	// Limit is the maximum number of attempts for retrying a failed sync. If set to 0, no retries will be performed.
	Limit int64 `json:"limit,omitempty" protobuf:"bytes,1,opt,name=limit"`
	// Backoff controls how to backoff on subsequent retries of failed syncs
	Backoff *Backoff `json:"backoff,omitempty" protobuf:"bytes,2,opt,name=backoff,casttype=Backoff"`
}

// Backoff is the backoff strategy to use on subsequent retries for failing syncs
type Backoff struct {
	// Duration is the amount to back off. Default unit is seconds, but could also be a duration (e.g. "2m", "1h")
	Duration string `json:"duration,omitempty" protobuf:"bytes,1,opt,name=duration"`
	// Factor is a factor to multiply the base duration after each failed retry
	Factor *int64 `json:"factor,omitempty" protobuf:"bytes,2,name=factor"`
	// MaxDuration is the maximum amount of time allowed for the backoff strategy
	MaxDuration string `json:"maxDuration,omitempty" protobuf:"bytes,3,opt,name=maxDuration"`
}

// OperationInitiator contains information about the initiator of an operation
type OperationInitiator struct {
	// Username contains the name of a user who started operation
	Username string `json:"username,omitempty" protobuf:"bytes,1,opt,name=username"`
	// Automated is set to true if operation was initiated automatically by the application controller.
	Automated bool `json:"automated,omitempty" protobuf:"bytes,2,opt,name=automated"`
}

// SyncOperation contains details about a sync operation.
type SyncOperation struct {
	// Revision is the revision (Git) or chart version (Helm) which to sync the application to
	// If omitted, will use the revision specified in app spec.
	Revision string `json:"revision,omitempty" protobuf:"bytes,1,opt,name=revision"`
	// Prune specifies to delete resources from the cluster that are no longer tracked in git
	Prune bool `json:"prune,omitempty" protobuf:"bytes,2,opt,name=prune"`
	// DryRun specifies to perform a `kubectl apply --dry-run` without actually performing the sync
	DryRun bool `json:"dryRun,omitempty" protobuf:"bytes,3,opt,name=dryRun"`
	// SyncStrategy describes how to perform the sync
	SyncStrategy *SyncStrategy `json:"syncStrategy,omitempty" protobuf:"bytes,4,opt,name=syncStrategy"`
	// Resources describes which resources shall be part of the sync
	Resources []SyncOperationResource `json:"resources,omitempty" protobuf:"bytes,6,opt,name=resources"`
	// Source overrides the source definition set in the application.
	// This is typically set in a Rollback operation and is nil during a Sync operation
	Source *ApplicationSource `json:"source,omitempty" protobuf:"bytes,7,opt,name=source"`
	// Manifests is an optional field that overrides sync source with a local directory for development
	Manifests []string `json:"manifests,omitempty" protobuf:"bytes,8,opt,name=manifests"`
	// SyncOptions provide per-sync sync-options, e.g. Validate=false
	SyncOptions SyncOptions `json:"syncOptions,omitempty" protobuf:"bytes,9,opt,name=syncOptions"`
	// Sources overrides the source definition set in the application.
	// This is typically set in a Rollback operation and is nil during a Sync operation
	Sources ApplicationSources `json:"sources,omitempty" protobuf:"bytes,10,opt,name=sources"`
	// Revisions is the list of revision (Git) or chart version (Helm) which to sync each source in sources field for the application to
	// If omitted, will use the revision specified in app spec.
	Revisions []string `json:"revisions,omitempty" protobuf:"bytes,11,opt,name=revisions"`
}

// SyncStrategy controls the manner in which a sync is performed
type SyncStrategy struct {
	// Apply will perform a `kubectl apply` to perform the sync.
	Apply *SyncStrategyApply `json:"apply,omitempty" protobuf:"bytes,1,opt,name=apply"`
	// Hook will submit any referenced resources to perform the sync. This is the default strategy
	Hook *SyncStrategyHook `json:"hook,omitempty" protobuf:"bytes,2,opt,name=hook"`
}

// SyncStrategyHook will perform a sync using hooks annotations.
// If no hook annotation is specified falls back to `kubectl apply`.
type SyncStrategyHook struct {
	// Embed SyncStrategyApply type to inherit any `apply` options
	// +optional
	SyncStrategyApply `json:",inline" protobuf:"bytes,1,opt,name=syncStrategyApply"`
}

// SyncStrategyApply uses `kubectl apply` to perform the apply
type SyncStrategyApply struct {
	// Force indicates whether or not to supply the --force flag to `kubectl apply`.
	// The --force flag deletes and re-create the resource, when PATCH encounters conflict and has
	// retried for 5 times.
	Force bool `json:"force,omitempty" protobuf:"bytes,1,opt,name=force"`
}

// SyncOperationResource contains resources to sync.
type SyncOperationResource struct {
	Group     string `json:"group,omitempty" protobuf:"bytes,1,opt,name=group"`
	Kind      string `json:"kind" protobuf:"bytes,2,opt,name=kind"`
	Name      string `json:"name" protobuf:"bytes,3,opt,name=name"`
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,4,opt,name=namespace"`
	// nolint:govet
	Exclude bool `json:"-"`
}

// SyncOperationResult represent result of sync operation
type SyncOperationResult struct {
	// Resources contains a list of sync result items for each individual resource in a sync operation
	Resources ResourceResults `json:"resources,omitempty" protobuf:"bytes,1,opt,name=resources"`
	// Revision holds the revision this sync operation was performed to
	Revision string `json:"revision" protobuf:"bytes,2,opt,name=revision"`
	// Source records the application source information of the sync, used for comparing auto-sync
	Source ApplicationSource `json:"source,omitempty" protobuf:"bytes,3,opt,name=source"`
	// Source records the application source information of the sync, used for comparing auto-sync
	Sources ApplicationSources `json:"sources,omitempty" protobuf:"bytes,4,opt,name=sources"`
	// Revisions holds the revision this sync operation was performed for respective indexed source in sources field
	Revisions []string `json:"revisions,omitempty" protobuf:"bytes,5,opt,name=revisions"`
	// ManagedNamespaceMetadata contains the current sync state of managed namespace metadata
	ManagedNamespaceMetadata *ManagedNamespaceMetadata `json:"managedNamespaceMetadata,omitempty" protobuf:"bytes,6,opt,name=managedNamespaceMetadata"`
}

type ManagedNamespaceMetadata struct {
	Labels      map[string]string `json:"labels,omitempty" protobuf:"bytes,1,opt,name=labels"`
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,2,opt,name=annotations"`
}

// ApplicationSources contains list of required information about the sources of an application
type ApplicationSources []ApplicationSource

// ResourceResults defines a list of resource results for a given operation
type ResourceResults []*ResourceResult

// ResourceResult holds the operation result details of a specific resource
type ResourceResult struct {
	// Group specifies the API group of the resource
	Group string `json:"group" protobuf:"bytes,1,opt,name=group"`
	// Version specifies the API version of the resource
	Version string `json:"version" protobuf:"bytes,2,opt,name=version"`
	// Kind specifies the API kind of the resource
	Kind string `json:"kind" protobuf:"bytes,3,opt,name=kind"`
	// Namespace specifies the target namespace of the resource
	Namespace string `json:"namespace" protobuf:"bytes,4,opt,name=namespace"`
	// Name specifies the name of the resource
	Name string `json:"name" protobuf:"bytes,5,opt,name=name"`
	// Status holds the final result of the sync. Will be empty if the resources is yet to be applied/pruned and is always zero-value for hooks
	Status ResultCode `json:"status,omitempty" protobuf:"bytes,6,opt,name=status"`
	// Message contains an informational or error message for the last sync OR operation
	Message string `json:"message,omitempty" protobuf:"bytes,7,opt,name=message"`
	// HookType specifies the type of the hook. Empty for non-hook resources
	HookType HookType `json:"hookType,omitempty" protobuf:"bytes,8,opt,name=hookType"`
	// HookPhase contains the state of any operation associated with this resource OR hook
	// This can also contain values for non-hook resources.
	HookPhase OperationPhase `json:"hookPhase,omitempty" protobuf:"bytes,9,opt,name=hookPhase"`
	// SyncPhase indicates the particular phase of the sync that this result was acquired in
	SyncPhase SyncPhase `json:"syncPhase,omitempty" protobuf:"bytes,10,opt,name=syncPhase"`
}

type OperationPhase string

const (
	OperationRunning     OperationPhase = "Running"
	OperationTerminating OperationPhase = "Terminating"
	OperationFailed      OperationPhase = "Failed"
	OperationError       OperationPhase = "Error"
	OperationSucceeded   OperationPhase = "Succeeded"
)

type SyncPhase string

const (
	SyncPhasePreSync  = "PreSync"
	SyncPhaseSync     = "Sync"
	SyncPhasePostSync = "PostSync"
	SyncPhaseSyncFail = "SyncFail"
)

type HookType string

const (
	HookTypePreSync  HookType = "PreSync"
	HookTypeSync     HookType = "Sync"
	HookTypePostSync HookType = "PostSync"
	HookTypeSkip     HookType = "Skip"
	HookTypeSyncFail HookType = "SyncFail"
)

// HealthStatus contains information about the currently observed health state of an application or resource
type HealthStatus struct {
	// Status holds the status code of the application or resource
	Status HealthStatusCode `json:"status,omitempty"`
	// Message is a human-readable informational message describing the health status
	Message string `json:"message,omitempty"`
}

type HealthStatusCode string

const (
	HeathStatusCodeHealthy     HealthStatusCode = "Healthy"
	HeathStatusCodeProgressing HealthStatusCode = "Progressing"
	HeathStatusCodeDegraded    HealthStatusCode = "Degraded"
	HeathStatusCodeSuspended   HealthStatusCode = "Suspended"
	HeathStatusCodeMissing     HealthStatusCode = "Missing"
	HeathStatusCodeUnknown     HealthStatusCode = "Unknown"
)

type ResultCode string

const (
	ResultCodeSynced       ResultCode = "Synced"
	ResultCodeSyncFailed   ResultCode = "SyncFailed"
	ResultCodePruned       ResultCode = "Pruned"
	ResultCodePruneSkipped ResultCode = "PruneSkipped"
)

// SyncStatus contains information about the currently observed live and desired states of an application
type SyncStatus struct {
	// Status is the sync state of the comparison
	Status SyncStatusCode `json:"status"`
	// Revision contains information about the revision the comparison has been performed to
	Revision string `json:"revision,omitempty"`
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

// GitOpsDeploymentCondition contains details about an GitOpsDeployment condition, which is usually an error or warning
type GitOpsDeploymentCondition struct {
	// Type is a GitOpsDeployment condition type
	Type GitOpsDeploymentConditionType `json:"type"`

	// Message contains human-readable message indicating details about the last condition.
	// +optional
	Message string `json:"message"`

	// LastProbeTime is the last time the condition was observed.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Status is the status of the condition.
	Status GitOpsConditionStatus `json:"status"`

	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason GitOpsDeploymentReasonType `json:"reason"`
}

// GitOpsDeploymentConditionType represents type of GitOpsDeployment condition.
type GitOpsDeploymentConditionType string

const (
	GitOpsDeploymentConditionSyncError     GitOpsDeploymentConditionType = "SyncError"
	GitOpsDeploymentConditionErrorOccurred GitOpsDeploymentConditionType = "ErrorOccurred"
)

// GitOpsConditionStatus is a type which represents possible comparison results
type GitOpsConditionStatus string

// GitOpsDeployment Condition Status
const (
	// GitOpsConditionStatusTrue indicates that a condition type is true
	GitOpsConditionStatusTrue GitOpsConditionStatus = "True"
	// GitOpsConditionStatusFalse indicates that a condition type is false
	GitOpsConditionStatusFalse GitOpsConditionStatus = "False"
	// GitOpsConditionStatusUnknown indicates that the condition status could not be reliably determined
	GitOpsConditionStatusUnknown GitOpsConditionStatus = "Unknown"
)

// ApplicationCondition indicates the condition reported by the Argo CD Application CR.
type ApplicationCondition struct {
	// Type is an application condition type
	Type ApplicationConditionType `json:"type"`

	// Message contains human-readable message indicating details about condition
	Message string `json:"message"`
}

// ApplicationConditionType represents type of application condition. Type name has following convention:
// prefix "Error" means error condition
// prefix "Warning" means warning condition
// prefix "Info" means informational condition
type ApplicationConditionType string

const (
	// ApplicationConditionDeletionError indicates that controller failed to delete application
	ApplicationConditionDeletionError = "DeletionError"
	// ApplicationConditionInvalidSpecError indicates that application source is invalid
	ApplicationConditionInvalidSpecError = "InvalidSpecError"
	// ApplicationConditionComparisonError indicates controller failed to compare application state
	ApplicationConditionComparisonError = "ComparisonError"
	// ApplicationConditionSyncError indicates controller failed to automatically sync the application
	ApplicationConditionSyncError = "SyncError"
	// ApplicationConditionUnknownError indicates an unknown controller error
	ApplicationConditionUnknownError = "UnknownError"
	// ApplicationConditionSharedResourceWarning indicates that controller detected resources which belongs to more than one application
	ApplicationConditionSharedResourceWarning = "SharedResourceWarning"
	// ApplicationConditionRepeatedResourceWarning indicates that application source has resource with same Group, Kind, Name, Namespace multiple times
	ApplicationConditionRepeatedResourceWarning = "RepeatedResourceWarning"
	// ApplicationConditionExcludedResourceWarning indicates that application has resource which is configured to be excluded
	ApplicationConditionExcludedResourceWarning = "ExcludedResourceWarning"
	// ApplicationConditionOrphanedResourceWarning indicates that application has orphaned resources
	ApplicationConditionOrphanedResourceWarning = "OrphanedResourceWarning"
)

type GitOpsDeploymentReasonType string

const (
	GitopsDeploymentReasonSyncError     GitOpsDeploymentReasonType = "SyncError"
	GitopsDeploymentReasonErrorOccurred GitOpsDeploymentReasonType = "ErrorOccurred"
)

const (
	GitOpsDeploymentUserError_InvalidPathSlash = "spec.source.path cannot be '/'"
	GitOpsDeploymentUserError_PathIsRequired   = "spec.source.path is a required field and it cannot be empty"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Sync Status",type=string,JSONPath=`.status.sync.status`
// +kubebuilder:printcolumn:name="Health Status",type=string,JSONPath=`.status.health.status`

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
