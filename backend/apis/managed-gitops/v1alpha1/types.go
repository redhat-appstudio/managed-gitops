package v1alpha1

type GitOpsResourceType string

const (
	GitOpsDeploymentRepositoryCredentialTypeName GitOpsResourceType = "GitOpsDeploymentRepositoryCredential"
	GitOpsDeploymentTypeName                     GitOpsResourceType = "GitOpsDeployment"
	GitOpsDeploymentSyncRunTypeName              GitOpsResourceType = "GitOpsDeploymentSyncRun"
)
