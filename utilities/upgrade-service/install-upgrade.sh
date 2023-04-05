#!/usr/bin/env bash

KUSTOMIZE_VERSION=${KUSTOMIZE_VERSION:-"v4.5.7"}
KUBECTL_VERSION=${KUBECTL_VERSION:-"v1.26.0"}



# deletes the temp directory
function cleanup() {
  rm -rf "${TEMP_DIR}"
  echo "Deleted temp working directory ${TEMP_DIR}"
}

# installs the stable version kustomize binary if not found in PATH
function install_kustomize() {
  if [[ -z "${KUSTOMIZE}" ]]; then
    echo "[INFO] kustomize binary not found in \$PATH, installing kustomize-${KUSTOMIZE_VERSION} in ${TEMP_DIR}"
    wget https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_$(uname | tr '[:upper:]' '[:lower:]')_$(uname -m |sed s/aarch64/arm64/ | sed s/x86_64/amd64/).tar.gz -O ${TEMP_DIR}/kustomize.tar.gz
    tar zxvf ${TEMP_DIR}/kustomize.tar.gz -C ${TEMP_DIR}
    KUSTOMIZE=${TEMP_DIR}/kustomize
    chmod +x ${TEMP_DIR}/kustomize
  fi
}

# installs the stable version of kubectl binary if not found in PATH
function install_kubectl() {
  if [[ -z "${KUBECTL}" ]]; then
    echo "[INFO] kubectl binary not found in \$PATH, installing kubectl-${KUBECTL_VERSION} in ${TEMP_DIR}"
    wget https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/$(uname | tr '[:upper:]' '[:lower:]')/$(uname -m | sed s/aarch64/arm64/ | sed s/x86_64/amd64/)/kubectl -O ${TEMP_DIR}/kubectl
    KUBECTL=${TEMP_DIR}/kubectl
    chmod +x ${TEMP_DIR}/kubectl
  fi
}

# Check if a pod is ready, if it fails to get ready, rollback to PREV_IMAGE
check_pod_status_ready() {
  for binary in "$@"; do
    echo "Binary $binary";
    pod_name=$(kubectl get pods --no-headers -o custom-columns=":metadata.name" -n gitops | grep "$binary");
    echo "Pod name : $pod_name";
    kubectl wait pod --for=condition=Ready $pod_name -n gitops --timeout=300s;
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
  echo "[INFO] Creating kustomization.yaml file using manifests from revision ${GIT_REVISION}"
  echo "apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/crd/overlays/local-dev?ref=${GIT_REVISION}
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/gitops-namespace?ref=${GIT_REVISION}
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/gitops-service-argocd/base?ref=${GIT_REVISION}
  - https://github.com/redhat-appstudio/managed-gitops/manifests/base/postgresql-staging?ref=${GIT_REVISION}
  - https://github.com/redhat-appstudio/managed-gitops/appstudio-controller/config/default?ref=${GIT_REVISION}
  - https://github.com/redhat-appstudio/managed-gitops/backend/config/default?ref=${GIT_REVISION}
  - https://github.com/redhat-appstudio/managed-gitops/cluster-agent/config/default?ref=${GIT_REVISION}" > ${TEMP_DIR}/kustomization.yaml
  cat ${TEMP_DIR}/kustomization.yaml
}

# Auto-generate DB Secret if not already present
function generate_postgresql_secret {
  echo "Generating secret for postgresql-password"
  if ! kubectl get secret -n gitops gitops-postgresql-staging &>/dev/null; then
    kubectl create secret generic gitops-postgresql-staging \
    --namespace=gitops \
    --from-literal=postgresql-password=$(openssl rand -base64 20)
  fi
}

# Code execution starts here
# create a temporary directory and do all the operations inside the directory.
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/managed-gitops-install-XXXXXXX)
echo "Using temp directory ${TEMP_DIR}"
# cleanup the temporary directory irrespective of whether the script ran successfully or failed with an error.
trap cleanup EXIT

# install kustomize in the the temp directory if its not available in the PATH
KUSTOMIZE=$(which kustomize)
install_kustomize

# install kubectl in the the temp directory if its not available in the PATH
KUBECTL=$(which kubectl)
install_kubectl


QUAY_USERNAME=redhat-appstudio
# Revision of the kubernetes manifests to be used for the installation
GIT_REVISION="main"

while getopts ':i:u:r:' OPTION; do
  case "$OPTION" in
    i) IMG=${OPTARG};;
    u) QUAY_USERNAME=${OPTARG};;
    r) GIT_REVISION=${OPTARG};;
    ?) echo "Available flag options are:\n[-i] to provide gitops service image (default: quay.io/${QUAY_USERNAME}/gitops-service:latest)\n[-u] to provide QUAY registry username (default: redhat-appstudio)"; exit;
  esac
done

if [ -z $IMG ]; then
  IMG="quay.io/${QUAY_USERNAME}/gitops-service:latest"
fi

echo "IMAGE $IMG";

PREV_IMAGE=$(${KUBECTL} get deploy/gitops-appstudio-service-controller-manager -n gitops -o jsonpath='{..image}');
echo "PREV IMAGE : $PREV_IMAGE";

if [ "$PREV_IMAGE" = "$IMG" ]; then
  echo "Currently deployed image matches the new image '$IMG'. No need to upgrade. Exiting.."
  exit 0; 
fi

echo "Upgrading from $PREV_IMAGE to $IMG";

# create the required yaml files for the kustomize based install.
create_kustomization_init_file

# Set the right container image and apply the manifests
echo "[INFO] Applying the manifests generated using kustomize"
${KUSTOMIZE} build ${TEMP_DIR} |  COMMON_IMAGE=${IMG} envsubst | ${KUBECTL} apply -f -

# Create Postgresql DB password secret
generate_postgresql_secret

echo 'Wait until pods are running';

check_pod_status_ready postgres appstudio gitops-core-service-controller gitops-service-agent

cleanup

echo "Upgrade Successful!"
exit 0;
