#!/usr/bin/env bash

set -xe

kubectl apply -k backend-shared/config/kcp

# Wait for the Virtual Workspace URL and Identity hash to be generated 
kubectl wait --for=condition=VirtualWorkspaceURLsReady apiexports/gitopsrvc-backend-shared --timeout=2m

identityHash=$(kubectl get apiexports.apis.kcp.dev gitopsrvc-backend-shared -o jsonpath='{.status.identityHash}')

kubectl apply -k appstudio-shared/config/kcp

# Add the Identity hash to the Appstudio APIExport in order to claim GitOpsDeployment from Backend APIExport
patch='{"spec":{"permissionClaims": [{"group": "", "resource": "secrets"},{"group": "", "resource": "namespaces"},{"group": "managed-gitops.redhat.com", "resource": "gitopsdeployments", "identityHash": '\"${identityHash}\"'}]}}'

kubectl patch apiexports.apis.kcp.dev gitopsrvc-appstudio-shared --type=merge --patch "${patch}"