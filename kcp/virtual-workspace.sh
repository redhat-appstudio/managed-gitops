#!/usr/bin/env bash

set -ex

source ./kcp/utils.sh

SERVICE_WS="service-$(echo $RANDOM)"
USER_WS="user-$(echo $RANDOM)"

export GITOPS_IN_KCP="true"

cleanup_workspace() {
    kubectl kcp ws
    kubectl delete workspace $SERVICE_WS || true
    kubectl delete workspace $USER_WS || true
    pkill go
}

trap cleanup_workspace EXIT

# Read the CPS KUBECONFIG path if it's not set already
readKUBECONFIGPath

echo "Initializing service provider workspace"
createAndEnterWorkspace "$SERVICE_WS"

echo "Creating APIExports and APIResourceSchemas in workspace $SERVICE_WS"
KUBECONFIG="${CPS_KUBECONFIG}" make apply-kcp-api-all

# Copy the identity hash from the backend APIExport in the service workspace so we can reference it later in the appstudio APIBinding
identityHash=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get apiexports.apis.kcp.dev gitopsrvc-backend-shared -o jsonpath='{.status.identityHash}')

# Create permissions to bind APIExports. We need this workaround until KCP fixes the bug in their admission logic. Ref: https://github.com/kcp-dev/kcp/issues/1939  
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
bindingName=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get clusterrolebinding | grep $SERVICE_WS | awk '{print $1}')
userName=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get clusterrolebindings $bindingName -o jsonpath='{.subjects[0].name}')

KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use $SERVICE_WS
cat <<EOF | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: bind-apiexport
rules:
- apiGroups:
  - apis.kcp.dev
  resourceNames:
  - gitopsrvc-backend-shared
  - gitopsrvc-appstudio-shared
  resources:
  - apiexports
  verbs:
  - bind
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: bind-apiexport
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: bind-apiexport
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: "$userName"
EOF

echo "Initializing user workspace"
createAndEnterWorkspace "$USER_WS"

createAPIBinding() {
    exportName=$1
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
    url=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get workspace $SERVICE_WS -o jsonpath='{.status.URL}')
    path=$(basename $url)
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use $USER_WS

    permissionClaims='
  permissionClaims:
  - group: ""
    resource: "secrets"
    state: "Accepted"
  - group: ""
    resource: "namespaces"
    state: "Accepted"'

  if [ exportName == "gitopsrvc-appstudio-shared" ]; then
    permissionClaims="${acceptedPermissionClaims}
  - group: \"managed-gitops.redhat.com\"
    resource: \"gitopsdeployments\"
    state: \"Accepted\"
    identityHash: ${identityHash}"
  fi

cat <<EOF | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -
apiVersion: apis.kcp.dev/v1alpha1
kind: APIBinding
metadata:
  name: ${exportName}
spec:
${permissionClaims}
  reference:
    workspace:
      path: ${path}
      exportName: ${exportName}
EOF
}

echo "Creating APIBindings in workspace $USER_wS"
createAPIBinding gitopsrvc-backend-shared
createAPIBinding gitopsrvc-appstudio-shared

# Checking if the bindings are in Ready state
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-appstudio-shared
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready apibindings/gitopsrvc-backend-shared

KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use $SERVICE_WS

registerSyncTarget

# Install Argo CD and GitOps Service components in service provider workspace
installArgoCD

runGitOpsService