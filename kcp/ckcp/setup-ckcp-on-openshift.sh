#!/usr/bin/env bash

set -o errexit
set -o nounset

SCRIPT_DIR="$(
  cd "$(dirname "$0")" >/dev/null
  pwd
)"

export GITOPS_IN_KCP="true"
export DISABLE_KCP_VIRTUAL_WORKSPACE="true"

ARGOCD_MANIFEST="$SCRIPT_DIR/../../manifests/kcp/argocd/install-argocd.yaml"
ARGOCD_NAMESPACE="gitops-service-argocd"

cleanup() {
  killall goreman
  killall kubectl
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

TMP_DIR="$(mktemp -d -t kcp-gitops-service.XXXXXXXXX)"
printf "Temporary directory created: %s\n" "${TMP_DIR}"
export TMP_DIR=${TMP_DIR}

clone-and-setup-ckcp() {
    echo "$SCRIPT_DIR"
    exit_if_binary_not_installed "git"
    pushd "${TMP_DIR}"
    git clone https://github.com/openshift-pipelines/pipeline-service.git
    pushd pipeline-service
    git checkout feb7a522e7e47af3670bde9a58ddef958e9bff1e
    cp "${SCRIPT_DIR}"/openshift_dev_setup.sh ./developer/ckcp/openshift_dev_setup.sh
    cp "${SCRIPT_DIR}"/config.yaml ./developer/ckcp/config.yaml

    sed -i 's/\--resources deployments.apps,services,ingresses.networking.k8s.io,pipelines.tekton.dev,pipelineruns.tekton.dev,tasks.tekton.dev,runs.tekton.dev,networkpolicies.networking.k8s.io/\--resources deployments.apps,services,ingresses.networking.k8s.io,networkpolicies.networking.k8s.io,statefulsets.apps,routes.route.openshift.io/' ./operator/images/kcp-registrar/bin/register.sh
    printf "\nUpdated the resources to sync as needed for gitops-service and ckcp to run...\n\n"
    
    printf "Setting up ckcp in the cluster"
    ./developer/ckcp/openshift_dev_setup.sh
    popd
    popd

    printf "\nThe pipeline-service repo consisting of ckcp is cloned and setup, ckcp is ready for use...\n\n"
}

clone-and-setup-ckcp "$TMP_DIR"
# delete openshift-gitops ns to clear out the resources
kubectl delete ns openshift-gitops

WORKSPACE="gitops-service-compute"

create_kubeconfig_secret() {
    sa_name=$1
    sa_secret_name=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get sa "$sa_name" -n $ARGOCD_NAMESPACE -o=jsonpath='{.secrets[0].name}')

    ca=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get secret/"$sa_secret_name" -n $ARGOCD_NAMESPACE -o jsonpath='{.data.ca\.crt}')
    token=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get secret/"$sa_secret_name" -n $ARGOCD_NAMESPACE -o jsonpath='{.data.token}' | base64 --decode)
    namespace=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get secret/"$sa_secret_name" -n $ARGOCD_NAMESPACE -o jsonpath='{.data.namespace}' | base64 --decode)

    server=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl config view -o jsonpath='{.clusters[?(@.name == "workspace.kcp.dev/current")].cluster.server}')

    secret_name=$2
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

echo "Installing Argo CD resources in workspace $WORKSPACE and namespace $ARGOCD_NAMESPACE"
KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl create ns $ARGOCD_NAMESPACE || true

KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl apply -f $ARGOCD_MANIFEST -n $ARGOCD_NAMESPACE

echo "Creating KUBECONFIG secrets for argocd server and argocd application controller service accounts"
create_kubeconfig_secret "argocd-server" "kcp-kubeconfig-controller"
create_kubeconfig_secret "argocd-application-controller" "kcp-kubeconfig-server"

echo "Verifying if argocd components are up and running after mounting kubeconfig secrets"
KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl wait --for=condition=available deployments/argocd-server -n $ARGOCD_NAMESPACE --timeout 3m
count=0
while [ $count -lt 30 ]
do
    count=`expr $count + 1`
    replicas=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get statefulsets argocd-application-controller -n $ARGOCD_NAMESPACE -o jsonpath='{.spec.replicas}')
    ready_replicas=$(KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get statefulsets argocd-application-controller -n $ARGOCD_NAMESPACE -o jsonpath='{.status.readyReplicas}')

    if [ "$replicas" -eq "$ready_replicas" ]; then 
        break
    fi
    if [ $count -eq 30 ]; then 
        echo "Statefulset argocd-application-controller does not have the required replicas"
        exit 1
    fi
done

cat <<EOF | KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl apply -n $ARGOCD_NAMESPACE -f -
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

echo "Argo CD is successfully installed in namespace $ARGOCD_NAMESPACE"

KUBECONFIG="${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig" kubectl get all -n $ARGOCD_NAMESPACE

test-gitops-service-e2e-in-kcp() {
  
  export KUBECONFIG=${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig
  echo "The Kubeconfig being used for this is:" "$KUBECONFIG"
  cd ${SCRIPT_DIR}/../../
  ./delete-dev-env.sh
  make devenv-docker
  kubectl create ns kube-system || true
  make start-e2e &
  make test-e2e
} 

test-gitops-service-e2e-in-kcp-in-ci() {
  
  export KUBECONFIG=${TMP_DIR}/ckcp-ckcp.default.managed-gitops-compute.kubeconfig
  echo "The Kubeconfig being used for testing e2e tests is:" "${KUBECONFIG}"
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

OPENSHIFT_CI="${OPENSHIFT_CI:-false}"

if [[ $OPENSHIFT_CI != "" ]]
then
    echo "running tests in openshift ci mode"
    test-gitops-service-e2e-in-kcp-in-ci "${TMP_DIR}" "${SCRIPT_DIR}"
else
    echo "running tests in non openshift-ci mode"
    test-gitops-service-e2e-in-kcp "${TMP_DIR}" "${SCRIPT_DIR}"
fi

# clean the tmp directory created for the local setup
echo "e2e tests on kcp ran successfully, cleanup initiated ..."
rm -rf "${TMP_DIR}"
cleanup