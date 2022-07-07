package argocd

import "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"

// GenerateArgoCDClusterSecretName generates the name of the Argo CD cluster secret (and the name of the server within Argo CD).
func GenerateArgoCDClusterSecretName(managedEnv db.ManagedEnvironment) string {
	return "managed-env-" + managedEnv.Managedenvironment_id
}

func GenerateArgoCDApplicationName(gitopsDeploymentCRUID string) string {
	return "gitopsdepl-" + string(gitopsDeploymentCRUID)
}
