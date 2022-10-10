#!/usr/bin/env bash

set -ex

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

REPO_ROOT="$SCRIPTPATH/.."
echo "$SCRIPTPATH"
source "${REPO_ROOT}/../kcp/utils.sh"

SERVICE_WS="gitops-service-provider"

# Read the CPS KUBECONFIG path if it's not set already
readKUBECONFIGPath

echo "-- Running GitOps Service in service provider KCP virtual workspace --"

echo "Entering the home workspace"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace use "gitops-service-e2e-test"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace use ${SERVICE_WS}

# Running GitOps Service components in service provider workspace
runGitOpsService
