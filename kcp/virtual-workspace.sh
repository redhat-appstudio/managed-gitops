#!/usr/bin/env bash

set -ex

source ./kcp/utils.sh

SERVICE_WS="service-$(echo $RANDOM)"
USER_WS="user-$(echo $RANDOM)"

export GITOPS_IN_KCP="true"

cleanup_workspace() {
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl delete workspace $SERVICE_WS || true
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl delete workspace $USER_WS || true
    pkill go
}

trap cleanup_workspace EXIT

# Read the CPS KUBECONFIG path if it's not set already
readKUBECONFIGPath

echo "Initializing service provider workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace
createAndEnterWorkspace "$SERVICE_WS"

echo "Creating APIExports and APIResourceSchemas in workspace $SERVICE_WS"
KUBECONFIG="${CPS_KUBECONFIG}" make apply-kcp-api-all

permissionToBindAPIExport 

echo "Initializing user workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace
createAndEnterWorkspace "$USER_WS"

echo "Creating APIBindings in workspace $USER_WS"
createAPIBinding gitopsrvc-backend-shared "" $SERVICE_WS
createAPIBinding gitopsrvc-appstudio-shared "" $SERVICE_WS

registerSyncTarget "user"

# Checking if the bindings are in Ready state
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-appstudio-shared
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-backend-shared

KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use $SERVICE_WS

registerSyncTarget "service"

# Install Argo CD and GitOps Service components in service provider workspace
installArgoCD

runGitOpsService