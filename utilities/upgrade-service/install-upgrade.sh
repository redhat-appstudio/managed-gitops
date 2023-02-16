#!/usr/bin/env bash

GOBIN=$(go env GOPATH)/bin

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

cleanup() {
  rm -rf /tmp/managed-gitops
}

exit_if_binary_not_installed kubectl

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

mkdir -p /tmp/managed-gitops
cd /tmp/managed-gitops

git clone git@github.com:redhat-appstudio/managed-gitops.git 
cd managed-gitops

make install-all-k8s IMG=$IMG

echo 'Wait until pods are running';

check_pod_status_ready postgres appstudio gitops-core-service-controller gitops-service-agent

cleanup

echo "Upgrade Successful!"
exit 0;
