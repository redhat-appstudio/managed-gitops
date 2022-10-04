package util

import "sigs.k8s.io/controller-runtime/pkg/client"

var (
	virtualWorkspaceClient client.Client
)

func SetVirtualWorkspaceClient(k8sClient client.Client) {
	virtualWorkspaceClient = k8sClient
}

func GetVirtualWorkspaceClient() client.Client {
	return virtualWorkspaceClient
}
