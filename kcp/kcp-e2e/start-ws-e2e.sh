#!/usr/bin/env bash

set -ex

source ./kcp/utils.sh

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
