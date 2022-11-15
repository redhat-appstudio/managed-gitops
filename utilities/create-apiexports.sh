#!/usr/bin/env bash

set -xe

APPLICATION_API_COMMIT="$1"

# Check if APPLICATION_API_COMMIT is set
if [ -z "$APPLICATION_API_COMMIT" ]; then
    echo "APPLICATION_API_COMMIT is not set"
    exit 1
fi

kubectl apply -k backend-shared/config/kcp

# Wait for the Virtual Workspace URL and Identity hash to be generated 
kubectl wait --for=condition=VirtualWorkspaceURLsReady apiexports/gitopsrvc-backend-shared --timeout=2m

identityHash=$(kubectl get apiexports.apis.kcp.dev gitopsrvc-backend-shared -o jsonpath='{.status.identityHash}')

kubectl apply -f https://raw.githubusercontent.com/redhat-appstudio/application-api/${APPLICATION_API_COMMIT}/config/kcp/apibinding.yaml
kubectl apply -f https://raw.githubusercontent.com/redhat-appstudio/application-api/${APPLICATION_API_COMMIT}/config/kcp/apiexport.yaml
kubextl apply -f https://raw.githubusercontent.com/redhat-appstudio/application-api/${APPLICATION_API_COMMIT}/config/kcp/apiresourceschema.yaml
kubectl apply -f https://raw.githubusercontent.com/redhat-appstudio/application-api/${APPLICATION_API_COMMIT}/config/kcp/kustomization.yaml


# Add the Identity hash to the Appstudio APIExport in order to claim GitOpsDeployment from Backend APIExport
patch='{"spec":{"permissionClaims": [{"group": "", "resource": "secrets"},{"group": "", "resource": "namespaces"},{"group": "managed-gitops.redhat.com", "resource": "gitopsdeployments", "identityHash": '\"${identityHash}\"'}]}}'

kubectl patch apiexports.apis.kcp.dev gitopsrvc-appstudio-shared --type=merge --patch "${patch}"