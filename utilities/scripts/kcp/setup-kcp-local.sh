#!/usr/bin/env bash

set -o errexit
set -o nounset

# Checks if a binary is present on the local system
exit_if_binary_not_installed() {
  for binary in "$@"; do
    command -v "$binary" >/dev/null 2>&1 || {
      echo >&2 "Script requires '$binary' command-line utility to be installed on your local machine. Aborting..."
      exit 1
    }
  done
}

# Checks if the version is compatible by checking greater than or equal to in the passed arguments
version_compatibility() { 
    test "$(echo "$@" | tr " " "\n" | sort -V | head -n 1)" != "$1";
}


# Checks if the go binary exists, and if the go version is suitable to setup KCP env
check_if_go_v_compatibility() {
    exit_if_binary_not_installed "go"
    compatible_v="1.18.0"
    go_version="$(go version | cut -d " " -f3 | cut -d "o" -f2)"
    echo "For the KCP local build and setup we need go version equal or greater than: $compatible_v."
    if version_compatibility $go_version $compatible_v; then
        echo "$go_version >= $compatible_v, compatibility check success ..."
    else
        echo "$go_version is lesser than $compatible_v, exiting ..."
        exit 1
    fi
}

check_if_go_v_compatibility

# install-kcp-binaries will install and build kcp in a temporary enviroment
# since, we are working on a stable branch current KCP stable version is `release-0.5`
# make build target of KCP will build the controller of KCP
KCP_DIR="${KCP_DIR:-}"
install-build-kcp-binaries() {
  if [[ -z "${KCP_DIR}" ]]; then
    exit_if_binary_not_installed "git"
    KCP_PARENT_DIR="$(mktemp -d -t kcp.XXXXXXXXX)"
    echo $KCP_PARENT_DIR
    pushd "${KCP_PARENT_DIR}"
    git clone https://github.com/kcp-dev/kcp.git
    KCP_DIR="${KCP_PARENT_DIR}/kcp"
    pushd kcp
    KCP_STABLE_BRANCH="release-0.5"
    git checkout "${KCP_STABLE_BRANCH}"
    make build
    popd
    popd
    printf "\nThe KCP binaries are installed successfully ...\n"
  fi
}

# Initiating the KCP server, the kubectl in this case will be using the KCP's kubeconfig
kcp-init() {
  printf "\nInitiating KCP server ...\n"
  pushd "${TMP_DIR}"
  printf "Current Directory:" pwd
  "$($KCP_DIR/bin/kcp start)" &> "${TMP_DIR}/kcp.log" &
  KCP_PID=$!
  printf "KCP server started: %s\n" $KCP_PID
  touch "${TMP_DIR}/kcp-started"
  printf "\n\n----------------------------------\n\n"
  printf "\nKCP ready: %s\n" $?
  popd
}

# wait for the passed function to complete the expected result with timeout instructions
wait_for() {
    timeout=$1
    shift 1
    until [ $timeout -le 0 ] || ("$@" &> /dev/null); do
        echo waiting for "$@"
        sleep 1
        timeout=$(( timeout - 1 ))
    done
    if [ $timeout -le 0 ]; then
        return 1
    fi
}

# checks if the KCP is in ready state to proceed further with development
check_if_kcp_is_ready() {
    status=$(kubectl --kubeconfig=${TMP_DIR}/.kcp/admin.kubeconfig get --raw /readyz)
    if [ "$status" == "ok" ]; then
        return 0;
    else
        return 1;
    fi
}

# CRDs to sync to kcp and workload clusters
CR_TOSYNC=(
            # requirement of argocd installation
            deployments.apps
            pods
            services
            secrets
            statefulsets
            networkpolicies
            configmaps
            roles
            rolebindings
            clusterroles
            clusterrolebindings
            # requirement of gitops service
            gitopsdeployments
          )

# Adds a workload cluster to KCP
add-workload-cluster() {
  exit_if_binary_not_installed "kubectl-kcp"
  # First, we need to add a workloadcluster to the kcp in order to sync resources
  cat $(kubectl -v 99 config current-context 2>&1 | grep file: | awk -F ': ' '{print $2}' | xargs) > ${TMP_DIR}/local_kubeconfig
  printf "Current Directory:" pwd
  KUBECONFIG=${TMP_DIR}/.kcp/admin.kubeconfig sed -e 's/^/    /' ${TMP_DIR}/local_kubeconfig | cat ./utilities/scripts/kcp/manifests/workloadcluster.yaml - | kubectl apply -f -

  # Then we add a syncer to kcp to sync the given resources to sync
  cr_string="$(IFS=,; echo "${CR_TOSYNC[*]}")"
  # kubectl kcp workload sync <workload-cluster-name> --syncer-image <kcp-syncer-image>
  KUBECONFIG="$KUBECONFIG_KCP" kubectl kcp workload sync local \
  --syncer-image ghcr.io/kcp-dev/kcp/syncer:release-0.5 \ # we are using the stable release-0.5 tag for this
  --resources "$cr_string" > "${TMP_DIR}/syncer.yaml"
  kubectl apply -f "${TMP_DIR}/syncer.yaml" >/dev/null 2>&1

  printf "Add workload cluster ran successfully!"
}

# install ArgoCD manifests in the argocd namespace
install-argocd-in-kcp() {
  export KUBECONFIG=${TMP_DIR}/.kcp/admin.kubeconfig
  kubectl create ns argocd
  kubectl apply -f https://gist.githubusercontent.com/samyak-jn/192b754b8a079cca53478b487a7242fe/raw/6e48401f08357bc53fb68c86915f785e933e686e/installArgoForKCP.yaml -n argocd
}

# install gitops-service manifests using make targets
install-gitops-service-in-kcp() {
  export KUBECONFIG=${TMP_DIR}/.kcp/admin.kubeconfig
  if [[ -z "${TMP_DIR}" ]]; then
    exit_if_binary_not_installed "git"
    pushd "${TMP_DIR}"
    git clone https://github.com/redhat-appstudio/managed-gitops.git
    KCP_DIR="${TMP_DIR}/managed-gitops"
    pushd managed-gitops
    make devenv-docker
    popd
    popd
    printf "\nThe dev enviroment for gitops service is setup successfully ...\n"
  fi
}

test-gitops-service-e2e-in-kcp() {
  export KUBECONFIG=${TMP_DIR}/.kcp/admin.kubeconfig 
  make start-e2e
  make test-e2e
}

# creates a development workspace (ex: dev-work)
create-workspace() {
    printf "Creating a org dev workspace: dev-work"
    kcp_ws="dev-work"
    exit_if_binary_not_installed "kubectl-kcp"
    KUBECONFIG=${TMP_DIR}/.kcp/admin.kubeconfig kubectl kcp workspaces create $kcp_ws --enter
}

cleanup() {
  pkill kcp
  rm -rf ${TMP_DIR}
  rm -rf ${KCP_DIR}
}

# Steps to setup KCP
PARENT_PATH=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )
install-build-kcp-binaries
TMP_DIR="$(mktemp -d -t kcp-gitops-service.XXXXXXXXX)"
printf "Temporary directory created: %s\n" "${TMP_DIR}"
kcp-init $TMP_DIR $KCP_DIR
wait_for 100 check_if_kcp_is_ready $TMP_DIR

# Once, KCP is up and running add a workloadcluster to it
  add-workload-cluster $TMP_DIR
# Create a workspace for development purposes
create-workspace $TMP_DIR
# install argocd in KCP workspace
install-argocd-in-kcp $TMP_DIR
# install gitops-service in KCP workspace
install-gitops-service-in-kcp $TMP_DIR

# run the service and test e2e against the cluster
test-gitops-service-e2e-in-kcp $TMP_DIR

# cleanup directories and process once the script is ran successfully
cleanup $TMP_DIR $KCP_DIR