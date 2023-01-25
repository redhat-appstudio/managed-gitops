#!/bin/bash

# A simple script for setting up Argo CD via OpenShift GitOps, on Argo CD

kubectl apply -f manifests/openshift-argo-deploy/openshift-gitops-subscription.yaml
echo -n "Waiting for default project (and namespace) to exist: "
while ! kubectl get appproject/default -n openshift-gitops &> /dev/null ; do
  echo -n .
  sleep 1
done
echo "OK"

kubectl create namespace gitops-service-argocd 2> /dev/null || true

echo "Installing Argo CD into gitops-service-argocd"
kubectl apply -f manifests/staging-cluster-resources/argo-cd.yaml
echo -n "Waiting for default project (and namespace) to exist: "
while ! kubectl get appproject/default -n gitops-service-argocd &> /dev/null ; do
  echo -n .
  sleep 1
done
echo "OK"
