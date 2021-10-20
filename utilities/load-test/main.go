package main

import (
	utils "github.com/redhat-appstudio/managed-gitops/utilities/load-test/loadtest"
)

func main() {
	// Running a kubectl apply command by passing namespace and URL for the manifests yaml(s)
	utils.KubectlApply("argocd", "https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml")
}
