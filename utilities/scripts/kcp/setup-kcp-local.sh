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
  pwd
  exec "$($KCP_DIR/bin/kcp start)" &> "${TMP_DIR}/kcp.log" &
  KCP_PID=$!
  
  
  printf "KCP server started: %s\n" $KCP_PID

  touch "${TMP_DIR}/kcp-started"

  # wait_command "ls ${KUBECONFIG}" 30
  
  # printf "Waiting for KCP to be ready ...\n"
  
  # wait_command "kubectl --kubeconfig=${KUBECONFIG} get --raw /readyz" 30
  
  printf "KCP ready: %s\n" $?
}

create-workspace() {
    printf "Creating a org dev workspace: dev-work"
    kcp_ws="dev-work"
    exit_if_binary_not_installed "kubectl kcp"
    kubectl kcp workspaces use $kcp_ws
}

# Steps to setup KCP
PARENT_PATH=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )

install-build-kcp-binaries

TMP_DIR="$(mktemp -d -t kcp-gitops-service.XXXXXXXXX)"
printf "Temporary directory created: %s\n" "${TMP_DIR}"

kcp-init $TMP_DIR $KCP_DIR
