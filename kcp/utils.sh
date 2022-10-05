#!/usr/bin/env bash

set -ex

CPS_KUBECONFIG="${CPS_KUBECONFIG:-$(realpath kcp/cps-kubeconfig)}"
WORKLOAD_KUBECONFIG="${WORKLOAD_KUBECONFIG:-$HOME/.kube/config}"
SYNCER_IMAGE="${SYNCER_IMAGE:-ghcr.io/kcp-dev/kcp/syncer:v0.8.2}"
SYNCER_MANIFESTS=$(mktemp -d)/cps-syncer.yaml

ARGOCD_MANIFEST="$(realpath manifests/kcp/argocd/install-argocd.yaml)"
ARGOCD_NAMESPACE="gitops-service-argocd"


readKUBECONFIGPath() {
    if [ "$CPS_KUBECONFIG" != "" ]; then
      return
    fi
    read -p "Please enter path to CPS KUBECONFIG: " CPS_KUBECONFIG
    if [ ! -f "$CPS_KUBECONFIG" ]; then
      echo "unable to find KUBECONFIG file at path: $CPS_KUBECONFIG"
      exit 1
    fi
}

createAndEnterWorkspace() {
    WORKSPACE=$1
    KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp ws

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
}

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


installArgoCD() {
    echo "Installing Argo CD resources in workspace $WORKSPACE and namespace $ARGOCD_NAMESPACE"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl create ns $ARGOCD_NAMESPACE || true
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f $ARGOCD_MANIFEST -n $ARGOCD_NAMESPACE

    echo "Creating KUBECONFIG secrets for argocd server and argocd application controller service accounts"
    create_kubeconfig_secret "argocd-server" "kcp-kubeconfig-controller"
    create_kubeconfig_secret "argocd-application-controller" "kcp-kubeconfig-server"

    echo "Verifying if argocd components are up and running after mounting kubeconfig secrets"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=available deployments/argocd-server -n $ARGOCD_NAMESPACE --timeout 3m
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

    cat <<EOF | KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -n $ARGOCD_NAMESPACE -f -
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
}

runGitOpsService() {
    echo "Preparing to run gitops service against KCP"
    ./stop-dev-env.sh || true
    ./delete-dev-env.sh || true

    KUBECONFIG="${CPS_KUBECONFIG}" make devenv-docker

    echo "Running gitops service controllers"
    KUBECONFIG="${CPS_KUBECONFIG}" make start
}

registerSyncTarget() {
    # Extract the Workload Cluster name from the KUBECONFIG
    WORKLOAD_CLUSTER=$(KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl config view --minify -o jsonpath='{.clusters[].name}' | awk -F[:] '{print $1}')

    WORKLOAD_CLUSTER=${WORKLOAD_CLUSTER:0:22}

    if [ "${WORKLOAD_CLUSTER: -1}" == "-" ]; then
      WORKLOAD_CLUSTER="${WORKLOAD_CLUSTER%?}"
    fi

    if [ "${WORKLOAD_CLUSTER:0:1}" == "-" ]; then
      WORKLOAD_CLUSTER="${WORKLOAD_CLUSTER#?}"
    fi

    echo "Generating syncer manifests for OCP SyncTarget $WORKLOAD_CLUSTER"
    KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workload sync "$WORKLOAD_CLUSTER"  --resources "services,statefulsets.apps,deployments.apps,routes.route.openshift.io" --syncer-image "$SYNCER_IMAGE" --output-file "$SYNCER_MANIFESTS" --namespace kcp-syncer

    # Deploy the syncer to the SyncTarget
    KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl apply -f "$SYNCER_MANIFESTS"

    echo "Waiting for the SyncTarget resource to reach ready state"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true synctarget/"${WORKLOAD_CLUSTER}" --timeout 3m
}
