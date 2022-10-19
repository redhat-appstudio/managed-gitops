#!/usr/bin/env bash

set -o errexit
set -o nounset

echo "========================================="
echo "Running setup-ckcp-on-openshift.sh script"
echo "========================================="

# Variables
# ~~~~~~~~~

SCRIPT_DIR="$(
  cd "$(dirname "$0")" >/dev/null
  pwd
)"

ARGOCD_MANIFEST="$SCRIPT_DIR/../../manifests/kcp/argocd/install-argocd.yaml"
ARGOCD_NAMESPACE="gitops-service-argocd"
TMP_DIR="$(mktemp -d -t kcp-gitops-service.XXXXXXXXX)"
export TMP_DIR
export GITOPS_IN_KCP="true"
export DISABLE_KCP_VIRTUAL_WORKSPACE="true"
export COMPATIBLE_GO_VERSION="1.18"
export OPENSHIFT_PIPELINES_REPO="https://github.com/openshift-pipelines/pipeline-service.git"
export OPENSHIFT_PIPELINES_COMMIT_HASH="edacf0348b2ae693eeb62b940dd618c05b34df62"
export OPENSHIFT_DEV_SCRIPT="${SCRIPT_DIR}/openshift_dev_setup.sh"
export CONFIG_YAML="${SCRIPT_DIR}/config.yaml"
export WORKSPACE="gitops-service-compute"
export OPENSHIFT_CI="${OPENSHIFT_CI:-false}"

[ ! -f "$ARGOCD_MANIFEST" ] && (echo "$ARGOCD_MANIFEST does not exist."; exit 1)
[ ! -d "$TMP_DIR" ] && (echo "$TMP_DIR does not exist."; exit 1)
[ ! -f "$OPENSHIFT_DEV_SCRIPT" ] && (echo "$OPENSHIFT_DEV_SCRIPT does not exist."; exit 1)
[ ! -f "$CONFIG_YAML" ] && (echo "$CONFIG_YAML does not exist."; exit 1)

echo "Temporary directory: ${TMP_DIR}"


# Helping Functions
# ~~~~~~~~~~~~~~~~~

cleanup() {
  echo "This is a best effort to stop the controllers."
  echo "Ignoring any exit codes (expected behavor)"
  echo "[INFO] Removing $TMP_DIR directory"
  rm -rf ${TMP_DIR}
  echo "Running goreman run stop-all"
  goreman run stop-all 2>&1 || true
  echo "Killing kubectl"
  killall kubectl 2>&1 || true
}

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
  echo "=== check_if_go_v_compatibility() ==="
  echo " [INFO] Checks if the go binary exists, and if the go version is suitable to setup KCP env"
  exit_if_binary_not_installed "go"
  go_version="$(go version | cut -d " " -f3 | cut -d "o" -f2)"
  echo "[INFO] For the KCP local build and setup we need go version equal or greater than: $COMPATIBLE_GO_VERSION."
  if version_compatibility $go_version $COMPATIBLE_GO_VERSION; then
    echo "[PASS] $go_version >= $COMPATIBLE_GO_VERSION, compatibility check success ..."
  else
    echo "[FAIL] $go_version is lesser than $COMPATIBLE_GO_VERSION, exiting ..."
    exit 1
  fi
}

clone-and-setup-ckcp() {
  echo "=== clone-and-setup-ckcp() ==="
  CKCP_OPENSHIFT_DEV_SETUP_SCRIPT="./ckcp/openshift_dev_setup.sh"
  CKCP_CONFIG_YAML="./ckcp/config.yaml"
  CKCP_REGISTER_SCRIPT="./images/kcp-registrar/register.sh"
  exit_if_binary_not_installed "git"
  pushd "${TMP_DIR}"
  echo "[INFO] Clogning $OPENSHIFT_PIPELINES_REPO"
  git clone "$OPENSHIFT_PIPELINES_REPO"
  pushd pipeline-service
  echo "[INFO] Checking out a specific cherry-picked commit hash $OPENSHIFT_PIPELINES_COMMIT_HASH"
  if ! git checkout "$OPENSHIFT_PIPELINES_COMMIT_HASH"; then echo "[FAIL] Problem running git checkout $OPENSHIFT_PIPELINES_COMMIT_HASH"; exit 1; fi
 
  echo "[INFO] Copy "$OPENSHIFT_DEV_SCRIPT" to $CKCP_OPENSHIFT_DEV_SETUP_SCRIPT"
  cp "$OPENSHIFT_DEV_SCRIPT" "$CKCP_OPENSHIFT_DEV_SETUP_SCRIPT"
  [ ! -f "$CKCP_OPENSHIFT_DEV_SETUP_SCRIPT" ] && (echo "[FAIL] $CKCP_OPENSHIFT_DEV_SETUP_SCRIPT does not exist."; exit 1)


  echo "[INFO] Copy "$CONFIG_YAML" to $CKCP_CONFIG_YAML"
  cp "$CONFIG_YAML" "$CKCP_CONFIG_YAML"
  [ ! -f "$CKCP_CONFIG_YAML" ] && (echo "$CKCP_CONFIG_YAML does not exist."; exit 1)

  [ ! -f "$CKCP_REGISTER_SCRIPT" ] && (echo "[FAIL] $CKCP_REGISTER_SCRIPT does not exist."; exit 1)

  echo "[INFO] Running a crazy sed command against CKCP_REGISTER_SCRIPT"
  sed -i 's/\--resources deployments.apps,services,ingresses.networking.k8s.io,pipelines.tekton.dev,pipelineruns.tekton.dev,tasks.tekton.dev,runs.tekton.dev,networkpolicies.networking.k8s.io/\--resources deployments.apps,services,ingresses.networking.k8s.io,networkpolicies.networking.k8s.io,statefulsets.apps,routes.route.openshift.io/' "$CKCP_REGISTER_SCRIPT"
  
  # TODO Check if sed worked as expected

  echo '[PASS] Updated the resources to sync as needed for gitops-service and ckcp to run...'; echo; echo
    
  echo '[INFO] Setting up ckcp in the cluster'
  bash -x "$CKCP_OPENSHIFT_DEV_SETUP_SCRIPT"

  #TODO: Check if script worked as expected

  popd
  popd

  echo '[PASS] The pipeline-service repo consisting of ckcp is cloned and setup, ckcp is ready for use...'; echo; echo
}

create_kubeconfig_secret() {
  echo "=== create_kubeconfig_secret ==="
    sa_name="$1"
    sa_secret_name=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get sa $sa_name -n $ARGOCD_NAMESPACE -o=jsonpath='{.secrets[0].name}')

    ca=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get secret/$sa_secret_name -n $ARGOCD_NAMESPACE -o jsonpath='{.data.ca\.crt}')
    token=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get secret/$sa_secret_name -n $ARGOCD_NAMESPACE -o jsonpath='{.data.token}' | base64 --decode)
    namespace=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get secret/$sa_secret_name -n $ARGOCD_NAMESPACE -o jsonpath='{.data.namespace}' | base64 --decode)

    server=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl config view -o jsonpath='{.clusters[?(@.name == "workspace.kcp.dev/current")].cluster.server}')

    secret_name="$2"
    kubeconfig_secret="
apiVersion: v1
kind: Secret
metadata:
  creationTimestamp: null
  name: ${secret_name}
  namespace: ${ARGOCD_NAMESPACE}
stringData:
  kubeconfig: |
    apiVersion: v1
    kind: Config
    clusters:
    - name: default-cluster
      cluster:
        insecure-skip-tls-verify: true
        server: ${server}
    contexts:
    - name: default-context
      context:
        cluster: default-cluster
        namespace: ${ARGOCD_NAMESPACE}
        user: default-user
    current-context: default-context
    users:
    - name: default-user
      user:
        token: ${token}
"

    echo "${kubeconfig_secret}" | KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl apply -f -
}



test-gitops-service-e2e-in-kcp() {
  echo "=== test-gitops-service-e2e-in-kc() ==="
  export KUBECONFIG=${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig
  printf "[INFO] The Kubeconfig being used for this is:" $KUBECONFIG
  cd ${SCRIPT_DIR}/../../
  ./delete-dev-env.sh
  make devenv-docker
  kubectl create ns kube-system || true
  make start-e2e &
  make test-e2e
} 

test-gitops-service-e2e-in-kcp-in-ci() {
  echo "=== test-gitops-service-e2e-in-kcp-in-ci ==="
  export KUBECONFIG=${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig
  printf "The Kubeconfig being used for testing e2e tests is: ${KUBECONFIG}\n"
  cd ${SCRIPT_DIR}/../../
  KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" make devenv-k8s-e2e
  printf "The Kubeconfig being used for port-forwarding is: ${ADMIN_KUBECONFIG}\n"
  KUBECONFIG="${ADMIN_KUBECONFIG}" dbnamespace=$(kubectl get pods --selector=app.kubernetes.io/instance=gitops-postgresql-staging --all-namespaces -o yaml | yq e '.items[0].metadata.namespace')
  printf "Postgres pod is running in ${dbnamespace} namespace in synctarget."

  # Wait until postgres pod is running
  printf " * Wait until Postgres pod is running"
  counter=0
  sleep 2m
  until KUBECONFIG="${ADMIN_KUBECONFIG}" kubectl -n $dbnamespace get pods | grep postgres | grep '1/1' | grep 'Running' &> /dev/null
  printf "Comes here"
  do
    ((counter++))
    sleep 1
    if [ "$counter" -gt 150 ]; then
      printf " --> Error: PostgreSQL pod cannot start. Quitting ..."
      printf ""
      printf "Namespace events:"
      KUBECONFIG="${ADMIN_KUBECONFIG}" kubectl get events -n $dbnamespace
      exit 1
    fi
  done
  printf " * Postgres Pod is running."
  
  # Port forward the PostgreSQL service locally, so we can access it
	KUBECONFIG="${ADMIN_KUBECONFIG}" kubectl port-forward -n $dbnamespace svc/gitops-postgresql-staging 5432:5432 &
  echo " * Port-Forwarding worked"

  KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl create ns kube-system || true
  KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" make start-e2e &
  KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" make test-e2e
}

delete-gitops-namespace() {
  echo "=== delete-gitops-namespace() ==="
  # delete openshift-gitops ns to clear out the resources
  echo "[INFO] Delete openshift-gitops namespace to clear our the resources within 1 minute"
  timeout 1m kubectl delete ns openshift-gitops
  if kubectl get ns openshift-gitop | grep openshift-gitops; then echo '[FAIL] openshift-gitops namespace stil exists'; exit 1; else echo '[PASSED] openshift-gitops namespace has been deleted'; fi
}

install-argocd-kcp() {
  echo "=== install-argocd-kcp() ==="
  echo "[INFO] Installing Argo CD resources in workspace $WORKSPACE and namespace $ARGOCD_NAMESPACE"
  TMP_KUBE_CONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig"
 [ ! -f "$TMP_KUBE_CONFIG" ] && (echo "[FAIL] $TMP_KUBE_CONFIG does not exist."; exit 1)
  echo "[INFO] Create $$ARGOCD_NAMESPACE namespace"
  KUBECONFIG="$TMP_KUBE_CONFIG" kubectl create ns $ARGOCD_NAMESPACE || true
  if kubectl get ns $ARGOCD_NAMESPACE | grep $ARGOCD_NAMESPACE ; then echo "[PASS] Namespace created"; else echo "[FAIL] Namespace failed to be created"; exit 1; fi
  echo "[INFO] Apply the manifest $ARGOCD_MANIFEST"
  KUBECONFIG="$TMP_KUBE_CONFIG" kubectl apply -f $ARGOCD_MANIFEST -n $ARGOCD_NAMESPACE

  # TODO: Check if the manifest got applied correctly

 echo "[INFO] Creating KUBECONFIG secrets for argocd server and argocd application controller service accounts"
 create_kubeconfig_secret "argocd-server" "kcp-kubeconfig-controller"
 if kubectl -n kcp-kubeconfig-controller get secret argocd-server | grep argocd-server; then echo "[PASS] argocd-server secret applied"; else echo "[FAIL] Cannot apply argocd-server secret"; exit 1; fi

 create_kubeconfig_secret "argocd-application-controller" "kcp-kubeconfig-server"
if kubectl -n argocd-application-controller get secret argocd-application-controller | grep argocd-application-controller; then echo "[PASS] argocd-application-controller secret applied"; else echo "[FAIL] Cannot apply argocd-application-controller secret"; exit 1; fi


echo "[INFO] Verifying if argocd components are up and running after mounting kubeconfig secrets within 3 minutes"
KUBECONFIG="$TMP_KUBE_CONFIG" kubectl wait --for=condition=available deployments/argocd-server -n $ARGOCD_NAMESPACE --timeout 3m
count=0
while [ $count -lt 30 ]
do
    count=`expr $count + 1`
    replicas=$(KUBECONFIG="$TMP_KUBE_CONFIG" kubectl get statefulsets argocd-application-controller -n $ARGOCD_NAMESPACE -o jsonpath='{.spec.replicas}')
    ready_replicas=$(KUBECONFIG="$TMP_KUBE_CONFIG" kubectl get statefulsets argocd-application-controller -n $ARGOCD_NAMESPACE -o jsonpath='{.status.readyReplicas}')

    if [ "$replicas" -eq "$ready_replicas" ]; then 
        break
    fi
    if [ $count -eq 30 ]; then 
        echo "Statefulset argocd-application-controller does not have the required replicas. Expected: $replicas. Actual: $ready_replicas"
        exit 1
    fi
done

echo "[INFO]Applying Secret for argocd-cluster-config into $ARGOCD_NAMESPACE"
cat <<EOF | KUBECONFIG="$TMP_KUBE_CONFIG" kubectl apply -n $ARGOCD_NAMESPACE -f -
apiVersion: v1
kind: Secret
metadata:
  name: argocd-cluster-config
  labels:
    argocd.argoproj.io/secret-type: cluster
type: Opaque
stringData:
  name: in-cluster
  namespace: "$ARGOCD_NAMESPACE"
  server: https://kubernetes.default.svc
  config: |
    {
      "tlsClientConfig": {
        "insecure": true
      }
    }
EOF

if kubectl -n "$ARGOCD_NAMESPACE" get secret  argocd-cluster-config | grep  argocd-cluster-config; then echo "[PASS]  argocd-cluster-config secret applied"; else echo "[FAIL] Cannot apply  argocd-cluster-config secret"; exit 1; fi

echo "[PASS] Argo CD is successfully installed in namespace $ARGOCD_NAMESPACE"

KUBECONFIG="$TMP_KUBE_CONFIG" kubectl get all -n $ARGOCD_NAMESPACE
}

run-tests() {
  echo "=== run-tests() =="
  if [[ $OPENSHIFT_CI != "" ]]
  then
    echo "[INFO] Running tests in openshift ci mode"
    test-gitops-service-e2e-in-kcp-in-ci ${TMP_DIR} ${SCRIPT_DIR}
  else
    echo "[INFO] Running tests in non openshift-ci mode"
    test-gitops-service-e2e-in-kcp ${TMP_DIR} ${SCRIPT_DIR}
  fi
}


# Main
# ~~~~

check_if_go_v_compatibility
clone-and-setup-ckcp
delete-gitops-namespace
install-argocd-kcp
run-tests
cleanup
