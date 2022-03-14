#!/bin/bash

# This script copies the all K8s resources from this repo, into a temporary directory, including substituting an image file
# into the Deployment resource.
#
# This can be used to update the https://github.com/rhd-ci-cd-sre/infra-deployments/tree/main/components/gitops/backend repo 
# with the latest version of these resources.

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

export ROOTPATH=$SCRIPTPATH/../../
export TARGET_DIR=`mktemp -d`

# Input: specify common container image for components
export COMMON_IMAGE="quay.io/redhat-appstudio/gitops-service:39630b97a060ebe2cadd2540d4675c6343f688e4"


# Output: Generate and output to tmp directory
"$SCRIPTPATH/shared-staging-config.sh"

cd $TARGET_DIR

kustomize create --autodetect

echo "* Manifest files packaged to $TARGET_DIR"

