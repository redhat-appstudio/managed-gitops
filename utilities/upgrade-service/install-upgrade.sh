#!/usr/bin/env bash

SCRIPT_DIR="$(
  cd "$(dirname "$0")" >/dev/null
  pwd
)"

# deletes the temp directory
function cleanup() {
  rm -rf "${TEMP_DIR}"
  echo "Deleted temp working directory $WORK_DIR"
}

# installs the stable version kustomize binary if not found in PATH
function install_kustomize() {
  if [[ -z "${KUSTOMIZE}" ]]; then
    wget https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv4.5.7/kustomize_v4.5.7_$(uname | tr '[:upper:]' '[:lower:]')_$(uname -m |sed s/aarch64/arm64/ | sed s/x86_64/amd64/).tar.gz -O ${TEMP_DIR}/kustomize.tar.gz
    tar zxvf ${TEMP_DIR}/kustomize.tar.gz -C ${TEMP_DIR}
    KUSTOMIZE=${TEMP_DIR}/kustomize
    chmod +x ${TEMP_DIR}/kustomize
  fi
}

# installs the stable version of kubectl binary if not found in PATH
function install_kubectl() {
  if [[ -z "${KUBECTL}" ]]; then
    wget https://dl.k8s.io/release/v1.26.0/bin/$(uname | tr '[:upper:]' '[:lower:]')/$(uname -m | sed s/aarch64/arm64/ | sed s/x86_64/amd64/)/kubectl -O ${TEMP_DIR}/kubectl
    KUBECTL=${TEMP_DIR}/kubectl
    chmod +x ${TEMP_DIR}/kubectl
  fi
}

# Checks if a binary is present on the local system
exit_if_binary_not_installed() {
  for binary in "$@"; do
    command -v $binary >/dev/null 2>&1 || {
      echo >&2 'Script requires $binary command-line utility to be installed on your local machine. Aborting...'
      exit 1
    }
  done
}

# Check if a pod is ready, if it fails to get ready, rollback to PREV_IMAGE
check_pod_status_ready() {
  for binary in "$@"; do
    echo "Binary $binary";
    pod_name=$(kubectl get pods --no-headers -o custom-columns=":metadata.name" -n gitops | grep "$binary");
    echo "Pod name : $pod_name";
    kubectl wait pod --for=condition=Ready $pod_name -n gitops --timeout=150s;
    if [ $? -ne 0 ]; then
      echo "Pod '$pod_name' failed to become Ready in desired time. Logs from the pod:"
      kubectl logs $pod_name -n gitops;
      echo "\nInstall/Upgrade failed. Performing rollback to $PREV_IMAGE";      
      rollback
    fi
  done
}

rollback() {
  if [ ! -z "$PREV_IMAGE" ]; then
    make install-all-k8s IMG=$PREV_IMAGE
    cleanup;
    echo "Upgrade Unsuccessful!!";
  else
    echo "Installing image for the first time. Nothing to rollback. Quitting..";
  fi
  exit 1;
}

# creates a kustomization.yaml file in the temp directory pointing to the manifests available in the upstream repo.
function create_kustomization_init_file() {
  echo "apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/crd/overlays/local-dev?ref=$GIT_REVISION
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/gitops-namespace?ref=$GIT_REVISION
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/gitops-service-argocd?ref=$GIT_REVISION
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/postgresql-staging?ref=$GIT_REVISION
  - https://github.com/redhat-appstudio/managed-gitops/appstudio-controller/config/default?ref=$GIT_REVISION
  - https://github.com/redhat-appstudio/managed-gitops/backend/config/default?ref=$GIT_REVISION
  - https://github.com/redhat-appstudio/managed-gitops/cluster-agent/config/default?ref=$GIT_REVISION" > ${TEMP_DIR}/kustomization.yaml
}

# Code execution starts here
# create a temporary directory and do all the operations inside the directory.
GIT_REVISION=main
TEMP_DIR=$(mktemp -d -t managed-gitops-install-XXXXXXX)
echo "Using temp directory $TEMP_DIR"
# cleanup the temporary directory irrespective of whether the script ran successfully or failed with an error.
trap cleanup EXIT

# install kustomize in the the temp directory if its not available in the PATH
KUSTOMIZE=$(which kustomize)
install_kustomize

# install kubectl in the the temp directory if its not available in the PATH
KUBECTL=$(which kubectl)
install_kubectl


QUAY_USERNAME=redhat-appstudio

while getopts ':i:u:' OPTION; do
  case "$OPTION" in
    i) IMG=${OPTARG};;
    u) QUAY_USERNAME=${OPTARG};;
    ?) echo "Available flag options are:\n[-i] to provide gitops service image (default: quay.io/${QUAY_USERNAME}/gitops-service:latest)\n[-u] to provide QUAY registry username (default: redhat-appstudio)"; exit;
  esac
done

if [ -z $IMG ]; then
  IMG="quay.io/${QUAY_USERNAME}/gitops-service:latest"
fi

echo "IMAGE $IMG";

PREV_IMAGE=$(kubectl get deploy/gitops-appstudio-service-controller-manager -n gitops -o jsonpath='{..image}');
echo "PREV IMAGE : $PREV_IMAGE";

if [ "$PREV_IMAGE" = "$IMG" ]; then
  echo "Currently deployed image matches the new image '$IMG'. No need to upgrade. Exiting.."
  exit 0; 
fi

echo "Upgrading from $PREV_IMAGE to $IMG";

# create the required yaml files for the kustomize based install.
create_kustomization_init_file

# Set the right container image and apply the manifests
${KUSTOMIZE} build ${TEMP_DIR} |  COMMON_IMAGE=${IMG} envsubst | kubectl apply -f -

# Create Postgresql DB secret
wget -O - https://raw.githubusercontent.com/anandf/managed-gitops/main/manifests/scripts/generate-postgresql-secret.sh | bash

echo 'Wait until pods are running';

check_pod_status_ready postgres appstudio gitops-core-service-controller gitops-service-agent

cleanup

echo "Upgrade Successful!"
exit 0;
