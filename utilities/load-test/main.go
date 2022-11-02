package main

import (
	"fmt"

	utils "github.com/redhat-appstudio/managed-gitops/utilities/load-test/loadtest"
)

const ARGO_CD_VERSION = "v2.5.1"

func main() {

	manifest := fmt.Sprintf("https://raw.githubusercontent.com/argoproj/argo-cd/%s/manifests/install.yaml", ARGO_CD_VERSION)
	// Running a kubectl apply command by passing namespace and URL for the manifests yaml(s)
	utils.KubectlApply("argocd", manifest)
}
