package v1alpha1

type GitOpsResourceType string

const (
	GitOpsDeploymentTypeName        GitOpsResourceType = "GitOpsDeployment"
	GitOpsDeploymentSyncRunTypeName GitOpsResourceType = "GitOpsDeploymentSyncRun"
)
