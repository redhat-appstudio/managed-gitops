#!/usr/bin/env bash
set -ex

CPS_KUBECONFIG="${CPS_KUBECONFIG:-$(realpath kcp/cps-kubeconfig)}"
WORKLOAD_KUBECONFIG="${WORKLOAD_KUBECONFIG:-$HOME/.kube/config}"
WORKSPACE="gitops-operator"
SYNCER_IMAGE="${SYNCER_IMAGE:-ghcr.io/kcp-dev/kcp/syncer:v0.7.10}"
SYNCER_MANIFESTS=$(mktemp -d)/cps-syncer.yaml

GITOPS_OPERATOR_NAMESPACE="gitops-operator"
ARGOCD_NAMESPACE="gitops-service-argocd"

cleanup_workspace() {
    pkill go
}

trap cleanup_workspace EXIT

KUBECONFIG="$CPS_KUBECONFIG" kubectl config use-context kcp-stable

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

WORKLOAD_CLUSTER=${WORKLOAD_CLUSTER:0:18}

echo "Generating syncer manifests for OCP SyncTarget $WORKLOAD_CLUSTER"
KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workload sync "$WORKLOAD_CLUSTER"  --resources "services,pods,statefulsets.apps,deployments.apps,routes.route.openshift.io,ingresses.networking.k8s.io,servicemonitors.monitoring.coreos.com,prometheusrules.monitoring.coreos.com,prometheuses.monitoring.coreos.com" --syncer-image "$SYNCER_IMAGE" --output-file "$SYNCER_MANIFESTS" --namespace kcp-syncer


# Deploy the syncer to the SyncTarget
KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl apply -f "$SYNCER_MANIFESTS"


echo "Waiting for the SyncTarget resource to reach ready state"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true synctarget/"${WORKLOAD_CLUSTER}" --timeout=3m


KUBECONFIG="${CPS_KUBECONFIG}" kubectl create ns $GITOPS_OPERATOR_NAMESPACE || true

create_kubeconfig_secret() {
    sa_name=$1
    ns=$3
    sa_secret_name=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get sa $sa_name -n $ns -o=jsonpath='{.secrets[0].name}')

    ca=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $ns -o jsonpath='{.data.ca\.crt}')
    token=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $ns -o jsonpath='{.data.token}' | base64 --decode)
    namespace=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $ns -o jsonpath='{.data.namespace}' | base64 --decode)

    server=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl config view -o jsonpath='{.clusters[?(@.name == "workspace.kcp.dev/current")].cluster.server}')

    secret_name=$2
    kubeconfig_secret="
apiVersion: v1
kind: Secret
metadata:
  creationTimestamp: null
  name: ${secret_name}
  namespace: ${ns}
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
        namespace: ${ns}
        user: default-user
    current-context: default-context
    users:
    - name: default-user
      user:
        token: ${token}
"

    echo "${kubeconfig_secret}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -
}

KUBECONFIG="${CPS_KUBECONFIG}" kubectl create sa controller-manager -n $GITOPS_OPERATOR_NAMESPACE || true


create_kubeconfig_secret "controller-manager" "gitops-operator-kubeconfig" "$GITOPS_OPERATOR_NAMESPACE"

KUBECONFIG="${CPS_KUBECONFIG}" kubectl delete crds --all

KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -k https://github.com/redhat-developer/gitops-operator/tree/master/config/crd --server-side --force-conflicts

CLUSTER_VERSION_CRD=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl get crds clusterversions.config.openshift.io -oyaml)
echo "${CLUSTER_VERSION_CRD}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -

CLUSTER_VERSION_CR=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl get clusterversions version -oyaml)
echo "${CLUSTER_VERSION_CR}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -

CONSOLE_CLI_CRD=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl get crd consoleclidownloads.console.openshift.io -oyaml)
echo "${CONSOLE_CLI_CRD}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -

cat <<EOF | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: admin
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
EOF

echo "Installing CRDs for GitOps and Argo CD operators"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f kcp/operator.yaml -n $GITOPS_OPERATOR_NAMESPACE

echo "Installing Argo CD resources in workspace $WORKSPACE and namespace $ARGOCD_NAMESPACE"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl create ns $ARGOCD_NAMESPACE || true

KUBECONFIG="${CPS_KUBECONFIG}" kubectl create sa gitops-service-argocd-argocd-application-controller -n $ARGOCD_NAMESPACE || true
KUBECONFIG="${CPS_KUBECONFIG}" kubectl create sa gitops-service-argocd-argocd-server -n $ARGOCD_NAMESPACE || true

echo "Creating KUBECONFIG secrets for argocd server and argocd application controller service accounts"
create_kubeconfig_secret "gitops-service-argocd-argocd-server" "kcp-kubeconfig-server" "$ARGOCD_NAMESPACE"
create_kubeconfig_secret "gitops-service-argocd-argocd-application-controller" "kcp-kubeconfig-controller" "$ARGOCD_NAMESPACE"

echo "Creating an Argo CD instance"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f kcp/argocd.yaml -n $ARGOCD_NAMESPACE

## Wait for Argo CD component to be up and running

# echo "Disable Security Context for Redis"
# KUBECONFIG="${CPS_KUBECONFIG}" kubectl get deployments.apps -n $ARGOCD_NAMESPACE gitops-service-argocd-redis
# if [ $? -eq 0 ]; then
#     KUBECONFIG="${CPS_KUBECONFIG}" kubectl patch deployment gitops-service-argocd-redis -n gitops-service-argocd --type='json' --patch='[{"op": "remove", "path": "/spec/template/spec/containers/0/securityContext"}]' || true
# fi

echo "Argo CD is successfully installed in namespace $ARGOCD_NAMESPACE"

echo "Preparing to run gitops service against KCP"
./stop-dev-env.sh || true
./delete-dev-env.sh || true

export DISABLE_KCP_VIRTUAL_WORKSPACE=true
export GITOPS_IN_KCP="true"

KUBECONFIG="${CPS_KUBECONFIG}" make devenv-docker

echo "Running gitops service controllers"
KUBECONFIG="${CPS_KUBECONFIG}" make start-e2e &
E2E_SERVER_PID=$!

KUBECONFIG="${CPS_KUBECONFIG}" make test-e2e

cleanup_workspace