#!/usr/bin/env bash

set -ex

CPS_KUBECONFIG="${CPS_KUBECONFIG:-$(realpath kcp/cps-kubeconfig)}"
WORKLOAD_KUBECONFIG="${WORKLOAD_KUBECONFIG:-$HOME/.kube/config}"
WORKSPACE="gitops-service"
SYNCER_IMAGE="${SYNCER_IMAGE:-ghcr.io/kcp-dev/kcp/syncer:v0.7.1}"
SYNCER_MANIFESTS=$(mktemp -d)/cps-syncer.yaml

export GITOPS_IN_KCP="true"

ARGOCD_MANIFEST="https://gist.githubusercontent.com/chetan-rns/91d0b56af152f3ebb7c10df8e82b459d/raw/99429ff5ac68cb78a4fc70f1ac2d673ad7ba192a/install-argocd.yaml"
ARGOCD_NAMESPACE="gitops-service-argocd"

# Checks if a binary is present on the local system
exit_if_binary_not_installed() {
    for binary in "$@"; do
        command -v "$binary" >/dev/null 2>&1 || {
            echo >&2 "Script requires '$binary' command-line utility to be installed on your local machine. Aborting..."
            exit 1
        }
    done
}

# Check if kubectl, kubelogin and kcp plugins are installed
exit_if_binary_not_installed "kubectl" "kubectl-kcp" "kubectl-oidc_login"

KUBECONFIG="$CPS_KUBECONFIG" kubectl config use-context kcp-stable

# Opens a web browser to authenticate to your RH SSO acount
KUBECONFIG="$CPS_KUBECONFIG" kubectl api-resources

# Create a new workspace if it doesn't exist
if KUBECONFIG="$CPS_KUBECONFIG" kubectl get workspaces "${WORKSPACE}"; then
    echo "Workspace $WORKSPACE already exists"
else 
    KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp ws create "$WORKSPACE"
fi

KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp ws use "$WORKSPACE"

KUBECONFIG="$CPS_KUBECONFIG" kubectl create ns kube-system || true

# Extract the Workload Cluster name from the KUBECONFIG
WORKLOAD_CLUSTER=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl config view --minify -o jsonpath='{.clusters[].name}' | awk -F[:] '{print $1}')

WORKLOAD_CLUSTER=${WORKLOAD_CLUSTER:22}

echo "Generating syncer manifests for OCP SyncTarget $WORKLOAD_CLUSTER"
KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workload sync "$WORKLOAD_CLUSTER"  --resources "services,statefulsets.apps,deployments.apps,routes.route.openshift.io" --syncer-image "$SYNCER_IMAGE" --output-file "$SYNCER_MANIFESTS" --namespace kcp-syncer


# Deploy the syncer to the SyncTarget
KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl apply -f "$SYNCER_MANIFESTS"


echo "Waiting for the SyncTarget resource to reach ready state"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true synctarget/"${WORKLOAD_CLUSTER}"

create_kubeconfig_secret() {
    sa_name=$1
    sa_secret_name=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get sa $sa_name -n $ARGOCD_NAMESPACE -o=jsonpath='{.secrets[0].name}')

    ca=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $ARGOCD_NAMESPACE -o jsonpath='{.data.ca\.crt}')
    token=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $ARGOCD_NAMESPACE -o jsonpath='{.data.token}' | base64 --decode)
    namespace=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $ARGOCD_NAMESPACE -o jsonpath='{.data.namespace}' | base64 --decode)

    server=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl config view -o jsonpath='{.clusters[?(@.name == "workspace.kcp.dev/current")].cluster.server}')

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
        certificate-authority-data: ${ca}
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

    echo "${kubeconfig_secret}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -
}

echo "Installing Argo CD resources in workspace $WORKSPACE and namespace $ARGOCD_NAMESPACE"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl create ns $ARGOCD_NAMESPACE || true

KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f $ARGOCD_MANIFEST -n $ARGOCD_NAMESPACE

echo "Creating KUBECONFIG secrets for argocd server and argocd application controller service accounts"
create_kubeconfig_secret "argocd-server" "kcp-kubeconfig-controller"
create_kubeconfig_secret "argocd-application-controller" "kcp-kubeconfig-server"

echo "Verifying if argocd components are up and running after mounting kubeconfig secrets"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=available deployments/argocd-server -n $ARGOCD_NAMESPACE
count=0
while [ $count -lt 30 ]
do
    count=`expr $count + 1`
    replicas=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get statefulsets argocd-application-controller -n $ARGOCD_NAMESPACE -o jsonpath='{.spec.replicas}')
    ready_replicas=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get statefulsets argocd-application-controller -n $ARGOCD_NAMESPACE -o jsonpath='{.status.readyReplicas}')
    if [ "$replicas" -eq "$ready_replicas" ]; then 
        break
    fi
    if [ $count -eq 30 ]; then 
        echo "Statefulset argocd-application-controller does not have the required replicas"
        exit 1
    fi
done

cat <<EOF | kubectl apply -n $ARGOCD_NAMESPACE -f -
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

cleanup_workspace() {
    echo "Deleting argocd resources from $ARGOCD_NAMESPACE"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl delete -f $ARGOCD_MANIFEST -n gitops-service-argocd    

    echo "Deleting syncer resources from the SyncTarget cluster"
    KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl delete -f "$SYNCER_MANIFESTS"
    
    echo "Deleting KCP workspace $WORKSPACE"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws ..
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl delete workspace "$WORKSPACE"
    
    kill $E2E_SERVER_PID
} 

echo "Argo CD is successfully installed in namespace $ARGOCD_NAMESPACE"

echo "Preparing to run gitops service against KCP"
./stop-dev-env.sh
./delete-dev-env.sh
KUBECONFIG="${CPS_KUBECONFIG}" make devenv-docker

echo "Running gitops service controllers"
KUBECONFIG="${CPS_KUBECONFIG}" make start-e2e &
E2E_SERVER_PID=$!

KUBECONFIG="${CPS_KUBECONFIG}" make test-e2e

cleanup_workspace