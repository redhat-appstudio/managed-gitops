#!/bin/bash

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

# A simple script for setting up Argo CD via OpenShift GitOps, on Argo CD

kubectl apply -f $SCRIPTPATH/openshift-gitops-subscription.yaml
echo -n "Waiting for default project (and namespace) to exist: "
while ! kubectl get appproject/default -n openshift-gitops &> /dev/null ; do
  echo -n .
  sleep 1
done
echo "OK"

kubectl create namespace gitops-service-argocd 2> /dev/null || true

echo "Installing Argo CD into gitops-service-argocd"
kustomize build $SCRIPTPATH/../../base/gitops-service-argocd/overlays/test-e2e | kubectl apply -f -
echo -n "Waiting for default project (and namespace) to exist: "
while ! kubectl get appproject/default -n gitops-service-argocd &> /dev/null ; do
  echo -n .
  sleep 1
done
echo "OK"
