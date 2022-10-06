#!/usr/bin/env bash

set -ex

source ./kcp/utils.sh

SERVICE_WS="gitops-service-provider"
USER_WS="user-workspace"

# After setting up the ws, the kubeconfigs for respective ws will be saved here
SERVICE_WS_CONFIG="/tmp/service-provider-workspace.yaml"
USER_WS_CONFIG="/tmp/user-provider-workspace.yaml"

# Topology for the virtual workspace:
# - user root
#   - gitops-service-e2e-test
#     - gitops-service-provider
#     - user-workspace

echo "-- Setup KCP virtual workspace for e2e testing initialised --"

# Read the CPS KUBECONFIG path if it's not set already
readKUBECONFIGPath

echo "Entering the home workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace

echo "Initializing gitops-service-e2e-test workspace"
createAndEnterWorkspace "gitops-service-e2e-test"

echo "Initializing service provider workspace"
# switching to parent workspace will ensure the childrens are created properly
kubectl kcp workspace use "gitops-service-e2e-test"
createAndEnterWorkspace "${SERVICE_WS}"
cp $KUBECONFIG $SERVICE_WS_CONFIG

# Installing ArgoCD in service-provider workspace
installArgoCD

echo "Creating APIExports and APIResourceSchemas in workspace ${SERVICE_WS}"
KUBECONFIG="${CPS_KUBECONFIG}" make apply-kcp-api-all

# Copy the identity hash from the backend APIExport in the service workspace so we can reference it later in the appstudio APIBinding
identityHash=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get apiexports.apis.kcp.dev gitopsrvc-backend-shared -o jsonpath='{.status.identityHash}')

permissionToBindAPIExport

echo "Initializing user workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace use "gitops-service-e2e-test"
createAndEnterWorkspace "${USER_WS}"
cp $KUBECONFIG $USER_WS_CONFIG

echo "Creating APIBindings in workspace ${USER_WS}"
createAPIBinding gitopsrvc-backend-shared
createAPIBinding gitopsrvc-appstudio-shared

# Checking if the bindings are in Ready state
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-appstudio-shared --timeout 2m &> /dev/null
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-backend-shared --timeout 2m &> /dev/null

echo "-- Setup KCP virtual workspace for e2e testing successful --"