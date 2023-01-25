#!/bin/bash

# A simple script for setting up Argo CD on non-OpenShift cluster. See Panos' 'argocd.sh' script for a full-featured version of this script.

kubectl create namespace gitops-service-argocd 2> /dev/null || true
kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-cd/$ARGO_CD_VERSION/manifests/install.yaml -n gitops-service-argocd

echo "Waiting for Argo CD to start in gitops-service-argocd"
while ! kubectl get appproject/default -n gitops-service-argocd &> /dev/null ; do
	echo -n .
	sleep 1
done

