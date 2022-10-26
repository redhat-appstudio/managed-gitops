#!/usr/bin/env bash

set -ex

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

export DISABLE_KCP_VIRTUAL_WORKSPACE="false"

source "${SCRIPTPATH}/../utils.sh"

PARENT_E2E_WS="gitops-service-e2e-test"
SERVICE_WS="gitops-service-provider"
USER_WS="user-workspace"

# Topology for the virtual workspace:
# - user root ex: root:fs:fsfs:redhat-sso-samyak   <---------------- workload
#   - gitops-service-e2e-test  <---------------- workload
#     - gitops-service-provider  <---------------- workload
#     - user-workspace  <---------------- workload

echo "-- Setup KCP virtual workspace for e2e testing initialised --"

# Read the CPS KUBECONFIG path if it's not set already
readKUBECONFIGPath

echo "Entering the home workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace

echo "Initializing gitops-service-e2e-test workspace"
createAndEnterWorkspace "${PARENT_E2E_WS}"

echo "Initializing service provider workspace"
# switching to parent workspace will ensure the childrens are created properly
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace use "${PARENT_E2E_WS}"
createAndEnterWorkspace "${SERVICE_WS}"
KUBECONFIG="${CPS_KUBECONFIG}"  kubectl config view > /tmp/serviceWs.kubeconfig
export SERVICE_PROVIDER_KUBECONFIG=/tmp/serviceWs.kubeconfig

KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workload sync "service-ws-test-cluster" --resources "services,statefulsets.apps,deployments.apps,routes.route.openshift.io" --syncer-image ghcr.io/kcp-dev/kcp/syncer:v0.9.0 --output-file ./syncer-servicews.yaml --namespace kcp-syncer
kubectl apply -f ./syncer-servicews.yaml
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true synctarget/service-ws-test-cluster --timeout 3m
rm ./syncer-servicews.yaml

# echo "Creating APIExports and APIResourceSchemas in workspace ${SERVICE_WS}"
KUBECONFIG="${CPS_KUBECONFIG}" make apply-kcp-api-all

permissionToBindAPIExport "${PARENT_E2E_WS}"

# Installing ArgoCD in service-provider workspace
installArgoCD

echo "Initializing user workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace use "${PARENT_E2E_WS}"
createAndEnterWorkspace "${USER_WS}"
KUBECONFIG="${CPS_KUBECONFIG}"  kubectl config view > /tmp/userWs.kubeconfig
export USER_KUBECONFIG=/tmp/userWs.kubeconfig

echo "Creating APIBindings in workspace ${USER_WS}"
createAPIBinding gitopsrvc-backend-shared "${PARENT_E2E_WS}" "${SERVICE_WS}"
createAPIBinding gitopsrvc-appstudio-shared "${PARENT_E2E_WS}" "${SERVICE_WS}"

# Checking if the bindings are in Ready state
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-appstudio-shared --timeout 2m &> /dev/null
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-backend-shared --timeout 2m &> /dev/null

echo "-- Setup KCP virtual workspace for e2e testing successful --"