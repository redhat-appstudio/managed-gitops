#!/bin/bash

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

# A simple script for setting up Argo CD via OpenShift GitOps, on Argo CD

SKIP_SUBSCRIPTION_INSTALL=${SKIP_SUBSCRIPTION_INSTALL:-"false"}

if [[ "$SKIP_SUBSCRIPTION_INSTALL" == "true" ]]; then
  echo "Skipping subscription install"
else
  echo "Installing subscription"
  kubectl apply -f $SCRIPTPATH/openshift-gitops-subscription.yaml

  kubectl create namespace gitops-service-argocd 2> /dev/null || true
  echo -n "Waiting for namespace to exist: "
  while ! kubectl get namespace gitops-service-argocd &> /dev/null ; do
    echo -n .
    sleep 1
  done
  echo "OK"

  echo -n "Checking for gitops operator controller pod to be created and running before proceeding with the next step:"
  while ! kubectl get pods -n openshift-operators | grep gitops-operator-controller-manager | grep Running &> /dev/null ; do
    echo -n .
    sleep 1
  done
  echo "OK"
fi

echo "Installing Argo CD into gitops-service-argocd"
kustomize build $SCRIPTPATH/../../base/gitops-service-argocd/overlays/test-e2e | kubectl apply -f -
echo -n "Waiting for default project (and namespace) to exist: "
while ! kubectl get appproject/default -n gitops-service-argocd &> /dev/null ; do
  echo -n .
  sleep 1
done
echo "OK"
