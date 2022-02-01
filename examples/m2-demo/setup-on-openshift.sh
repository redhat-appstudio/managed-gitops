#!/bin/bash

# Setup ArgoCD on cluster, via OpenShift GitOps -------------------------------

INFRA_DEPL_TEMP_DIR=`mktemp -d`

cd "$INFRA_DEPL_TEMP_DIR"
git clone https://github.com/redhat-appstudio/infra-deployments
cd infra-deployments

# Checkout known working commit: https://github.com/redhat-appstudio/infra-deployments/commit/b176e72b5aaf0c40f1bc0abbb97e2d43207149d0
git checkout b176e72b5aaf0c40f1bc0abbb97e2d43207149d0

hack/bootstrap-cluster.sh

kubectl delete -f argo-cd-apps/app-of-apps/all-applications-staging.yaml

# Clone and start GitOps service ----------------------------------------------

GITOPS_TEMP_DIR=`mktemp -d`

cd "$GITOPS_TEMP_DIR"

git clone https://github.com/redhat-appstudio/managed-gitops
cd managed-gitops

# Checkout known working commit: https://github.com/redhat-appstudio/managed-gitops/commit/d9c002cfd5155edddfdc78f3e3c633ce3fe9746d
git checkout d9c002cfd5155edddfdc78f3e3c633ce3fe9746d

cd managed-gitops

# Apply the CRDs
kubectl apply -f "$GITOPS_TEMP_DIR"/managed-gitops/backend/config/crd/bases
kubectl apply -f "$GITOPS_TEMP_DIR"/managed-gitops/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml

# Start the local database ----------------------------------------------------

make reset-db

# Create demo user namespaces (workspaces) ------------------------------------

kubectl create namespace jgw
kubectl create namespace jane
# Note: any namespace will work!

# Start the process -----------------------------------------------------------

# Install goreman to ~/go/bin
make download-deps

ARGO_CD_NAMESPACE=openshift-gitops make start

