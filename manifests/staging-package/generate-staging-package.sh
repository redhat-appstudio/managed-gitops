#!/bin/bash

# This script copies the all K8s resources from this repo, into a temporary directory, including substituing an image file
# into the Deployment resource.
#
# This can be used to update the https://github.com/redhat-appstudio/infra-deployments/tree/main/components/gitops/backend repo 
# with the latest version of these resources.

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

ROOTPATH=$SCRIPTPATH/../../

TARGET_DIR=`mktemp -d`

cp -R $ROOTPATH/manifests/*.yaml $TARGET_DIR
cp -R $ROOTPATH/manifests/database-init/*.yaml $TARGET_DIR

cp -R $ROOTPATH/manifests/staging-cluster-resources/*.yaml $TARGET_DIR

cp -R $ROOTPATH/manifests/backend-rbac/*.yaml $TARGET_DIR
cp -R $ROOTPATH/manifests/cluster-agent-rbac/*.yaml $TARGET_DIR
cp -R $ROOTPATH/manifests/postgresql-staging/postgresql-staging.yaml $TARGET_DIR
cp -R $ROOTPATH/manifests/appstudio-controller-rbac/appstudio-controller-rbac.yaml $TARGET_DIR

export COMMON_IMAGE="quay.io/redhat-appstudio/gitops-service:39630b97a060ebe2cadd2540d4675c6343f688e4"

# NOTE: ensure you update the COMMON_IMAGE with the image to deploy to the staging cluster
ARGO_CD_NAMESPACE=gitops-service-argocd envsubst < $ROOTPATH/manifests/managed-gitops-backend-deployment.yaml > $TARGET_DIR/managed-gitops-backend-deployment.yaml
ARGO_CD_NAMESPACE=gitops-service-argocd envsubst < $ROOTPATH/manifests/managed-gitops-clusteragent-deployment.yaml > $TARGET_DIR/managed-gitops-clusteragent-deployment.yaml
envsubst < $ROOTPATH/manifests/managed-gitops-appstudio-controller-deployment.yaml > $TARGET_DIR/managed-gitops-appstudio-controller-deployment.yaml

cp -R $ROOTPATH/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml $TARGET_DIR
cp -R $ROOTPATH/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml $TARGET_DIR
cp -R $ROOTPATH/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml $TARGET_DIR


echo "* Manifest files packaged to $TARGET_DIR"

