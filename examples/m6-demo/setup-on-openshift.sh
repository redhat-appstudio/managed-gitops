#!/bin/bash

echo "* Cloning GitOps Service source repository ------------------------------"
echo
M6_DEMO_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"


GITOPS_TEMP_DIR=`mktemp -d`

cd "$GITOPS_TEMP_DIR"

# NOTE: Update this to redhat-appstudio once the PR merges

git clone https://github.com/jgwest/managed-gitops
cd managed-gitops
git checkout managed-environment-87-june-2022

echo "* Installing Argo CD to OpenShift cluster, and setting up dev environment-"
echo
make install-argocd-openshift devenv-docker reset-db

echo "* Creating demo user namespaces ------------------------------------------"
echo
kubectl apply -f "$M6_DEMO_ROOT/resources/jane-namespace.yaml"
kubectl apply -f "$M6_DEMO_ROOT/resources/jgw-namespace.yaml"

echo "* Building and starting GitOps Service ----------------------------------"
echo
make start


