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

git checkout m2-demo-on-cluster

make install-all-k8s IMG=quay.io/jgwest-redhat/gitops-service:m2-demo

# Create demo user namespaces (workspaces) ------------------------------------

kubectl create namespace jgw
kubectl create namespace jane
# Note: any namespace will work!


# Show services resouces ------------------------------------------------------

echo ""
echo "GitOps Service K8s resources (in namespace 'gitops'):"
echo ""

kubectl get all -n gitops

