set -ex

CPS_KUBECONFIG="${CPS_KUBECONFIG:-$(realpath kcp/cps-kubeconfig)}"
WORKLOAD_KUBECONFIG="${WORKLOAD_KUBECONFIG:-$HOME/.kube/config}"
WORKSPACE="gitops-operator"
SYNCER_IMAGE="${SYNCER_IMAGE:-ghcr.io/kcp-dev/kcp/syncer:v0.7.8}"
SYNCER_MANIFESTS=$(mktemp -d)/cps-syncer.yaml

GITOPS_OPERATOR_NAMESPACE="gitops-operator"


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

WORKLOAD_CLUSTER=${WORKLOAD_CLUSTER:22}

echo "Generating syncer manifests for OCP SyncTarget $WORKLOAD_CLUSTER"
KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workload sync "$WORKLOAD_CLUSTER"  --resources "services,pods,statefulsets.apps,deployments.apps,routes.route.openshift.io,ingresses.networking.k8s.io" --syncer-image "$SYNCER_IMAGE" --output-file "$SYNCER_MANIFESTS" --namespace kcp-syncer


# Deploy the syncer to the SyncTarget
KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl apply -f "$SYNCER_MANIFESTS"


echo "Waiting for the SyncTarget resource to reach ready state"
KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true synctarget/"${WORKLOAD_CLUSTER}" --timeout=3m


KUBECONFIG="${CPS_KUBECONFIG}" kubectl create ns $GITOPS_OPERATOR_NAMESPACE || true

create_kubeconfig_secret() {
    sa_name=$1
    sa_secret_name=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get sa $sa_name -n $GITOPS_OPERATOR_NAMESPACE -o=jsonpath='{.secrets[0].name}')

    ca=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $GITOPS_OPERATOR_NAMESPACE -o jsonpath='{.data.ca\.crt}')
    token=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $GITOPS_OPERATOR_NAMESPACE -o jsonpath='{.data.token}' | base64 --decode)
    namespace=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/$sa_secret_name -n $GITOPS_OPERATOR_NAMESPACE -o jsonpath='{.data.namespace}' | base64 --decode)

    server=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl config view -o jsonpath='{.clusters[?(@.name == "workspace.kcp.dev/current")].cluster.server}')

    secret_name=$2
    kubeconfig_secret="
apiVersion: v1
kind: Secret
metadata:
  creationTimestamp: null
  name: ${secret_name}
  namespace: ${GITOPS_OPERATOR_NAMESPACE}
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
        namespace: ${GITOPS_OPERATOR_NAMESPACE}
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


create_kubeconfig_secret "controller-manager" "gitops-operator-kubeconfig"

KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -k ~/gitops-operator/config/crd/

CLUSTER_VERSION_CRD=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl get crds clusterversions.config.openshift.io -oyaml)
echo "${CLUSTER_VERSION_CRD}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -

CLUSTER_VERSION_CR=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl get clusterversions version -oyaml)
echo "${CLUSTER_VERSION_CR}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -

CONSOLE_CLI_CRD=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl get crd consoleclidownloads.console.openshift.io -oyaml)
echo "${CONSOLE_CLI_CRD}" | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: admin
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
EOF

## Install CRDs for GitOps and ArgoCD operators

KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f kcp/operator.yaml -n $GITOPS_OPERATOR_NAMESPACE