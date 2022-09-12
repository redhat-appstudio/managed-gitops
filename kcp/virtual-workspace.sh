#!/usr/bin/env bash

set -ex

source utils.sh

SERVICE_WS="service-provider-$(echo $RANDOM)"
USER_WS="user-$(echo $RANDOM)"

export GITOPS_IN_KCP="true"

readKUBECONFIGPath

# Create and initialize server provide namespace
createAndEnterWorkspace SERVICE_WS

registerSyncTarget

# Install Argo CD and GitOps Service components in service provider workspace
installArgoCD

KUBECONFIG="${CPS_KUBECONFIG}" make apply-kcp-api-all

runGitOpsService

createAndEnterWorkspace USER_WS

switchToWorkspace() {
    kubectl config use-context kcp-stable
    kubectl kcp ws use $1
}

createAPIBinding() {
    exportName=$1
    switchToWorkspace SERVICE_WS
    url=$(kubectl get workspaces sample -o jsonpath='{.status.URL}')
    path=$($url\#\#*/)

cat <<EOF | kubectl apply -n -f -
apiVersion: apis.kcp.dev/v1alpha1
kind: APIBinding
metadata:
  name: ${exportName}
spec:
  acceptedPermissionClaims:
  - group: ""
    resource: "secrets"
  - group: ""
    resource: "namespaces"
  reference:
    workspace:
      path: ${path}
      exportName: ${exportName}
EOF

}

createAPIBinding gitopsrvc-backend-shared
createAPIBinding gitopsrvc-appstudio-shared