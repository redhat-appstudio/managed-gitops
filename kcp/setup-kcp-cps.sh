#!/usr/bin/env bash

set -ex

CPS_KUBECONFIG="${CPS_KUBECONFIG:-cps-kubeconfig}"
WORKLOAD_KUBECONFIG="${WORKLOAD_KUBECONFIG:-$HOME/.kube/config}"
WORKSPACE="gitops-service"
SYNCER_IMAGE="${SYNCER_IMAGE:-ghcr.io/kcp-dev/kcp/syncer:v0.5.0-alpha.1}"
SYNCER_MANIFESTS=$(mktemp -d)/cps-syncer.yaml

# Check if kubectl, kubelogin is installed
KUBECONFIG="$CPS_KUBECONFIG" kubectl config use-context kcp-stable

# Opens a web browser to authenticate to your RH SSO acount
KUBECONFIG="$CPS_KUBECONFIG" kubectl api-resources


# Enter a new workspace and enter that workspace
KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workspace create "$WORKSPACE" --enter --ignore-existing


# Extract the Workload Cluster name from the KUBECONFIG
WORKLOAD_CLUSTER=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl config view --minify -o jsonpath='{.clusters[].name}' | awk -F[:] '{print $1}')

KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workload sync "$WORKLOAD_CLUSTER" --syncer-image "$SYNCER_IMAGE" > "$SYNCER_MANIFESTS"


# Deploy the syncer to the Workload cluster
KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl apply -f "$SYNCER_MANIFESTS"


# Check if the cluster is already registered

# Wait until the Workload cluster is in ready state
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true workloadclusters/"${WORKLOAD_CLUSTER}"

# KUBECONFIG="${CPS_KUBECONFIG}" kubectl get workloadclusters "$WORKLOAD_CLUSTER" -owide

cleanup_workspace() {
    KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl delete -f "$SYNCER_MANIFESTS"
    # enter root workspace
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp workspace ..
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl delete workspace "$WORKSPACE"
}

cleanup_workspace