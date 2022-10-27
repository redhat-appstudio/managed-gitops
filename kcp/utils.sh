#!/usr/bin/env bash

set -ex

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

CPS_KUBECONFIG="${CPS_KUBECONFIG:-$(realpath kcp/cps-kubeconfig)}"
WORKLOAD_KUBECONFIG="${WORKLOAD_KUBECONFIG:-$HOME/.kube/config}"
SYNCER_IMAGE="${SYNCER_IMAGE:-ghcr.io/kcp-dev/kcp/syncer:v0.9.1}"
SYNCER_MANIFESTS=$(mktemp -d)/cps-syncer.yaml

ARGOCD_MANIFEST="$(realpath manifests/kcp/argocd/install-argocd.yaml)"
ARGOCD_NAMESPACE="gitops-service-argocd"

# Checks if a binary is present on the local system
# need yammlint utility to validate the yaml file
exit_if_binary_not_installed() {
  for binary in "$@"; do
    command -v "$binary" >/dev/null 2>&1 || {
      echo >&2 "Script requires '$binary' command-line utility to be installed on your local machine. Aborting..."
      exit 1
    }
  done
}


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
    if [ -n "$1" ]; then
      echo "Workspace request: ${1}"
      WORKSPACE=$1
    else
      echo "--> Error: Workspace name not found. Exiting ..."
      exit 1
    fi

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
    sa_secret_name=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get sa "${sa_name}" -n $ARGOCD_NAMESPACE -o=jsonpath='{.secrets[0].name}')

    ca=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/"${sa_secret_name}" -n $ARGOCD_NAMESPACE -o jsonpath='{.data.ca\.crt}')
    token=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/"${sa_secret_name}" -n $ARGOCD_NAMESPACE -o jsonpath='{.data.token}' | base64 --decode)
    # namespace=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get secret/"${sa_secret_name}" -n $ARGOCD_NAMESPACE -o jsonpath='{.data.namespace}' | base64 --decode)

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
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f "${ARGOCD_MANIFEST}" -n $ARGOCD_NAMESPACE

    echo "Creating KUBECONFIG secrets for argocd server and argocd application controller service accounts"
    create_kubeconfig_secret "argocd-server" "kcp-kubeconfig-controller"
    create_kubeconfig_secret "argocd-application-controller" "kcp-kubeconfig-server"

    echo "Verifying if argocd components are up and running after mounting kubeconfig secrets"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=available deployments/argocd-server -n $ARGOCD_NAMESPACE --timeout 3m
    count=0
    while [ $count -lt 30 ]
    do
        count=$((count + 1))
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

    if [ -n "$1" ]; then 
      exportName=$1
      WORKLOAD_CLUSTER="${WORKLOAD_CLUSTER}-$1"
    fi

    echo "Generating syncer manifests for OCP SyncTarget $WORKLOAD_CLUSTER"
    KUBECONFIG="$CPS_KUBECONFIG" kubectl kcp workload sync "$WORKLOAD_CLUSTER"  --resources "services,statefulsets.apps,deployments.apps,routes.route.openshift.io" --syncer-image "$SYNCER_IMAGE" --output-file "$SYNCER_MANIFESTS" --namespace kcp-syncer

    # Deploy the syncer to the SyncTarget
    KUBECONFIG="${WORKLOAD_KUBECONFIG}" kubectl apply -f "$SYNCER_MANIFESTS"

    echo "Waiting for the SyncTarget resource to reach ready state"
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl wait --for=condition=Ready=true synctarget/"${WORKLOAD_CLUSTER}" --timeout 3m

    echo "Checking if the CRDs are synced"
    crs_to_sync=("services" "statefulsets.apps" "deployments.apps" "routes.route.openshift.io")
    count=10
    while [ $count -gt 0 ]
    do
        api_resources=($(KUBECONFIG="${CPS_KUBECONFIG}" kubectl api-resources -oname))
        length=${#crs_to_sync[@]}
        all_synced=true
        for (( i=0; i<${length}; i++ ));
        do
            if [[ ! "${api_resources[*]}" =~ "${crs_to_sync[$i]}" ]]; then
                echo "Unable to find API: ${crs_to_sync[$i]}" 
                all_synced=false
            fi
        done

        if [ "$all_synced" = true ]; then
            echo "CRDs are synced successfully"
            break    
        fi

        sleep 10
        ((count--))
    done
    if [ $count -le 0 ]; then
      echo "CRDS are not synced from the SyncTarget"
      exit 1
    fi
}

createAPIBinding() {
    exit_if_binary_not_installed "yamllint"

    if [ -n "$1" ]; then 
      exportName=$1
    else
      echo "--> Error: No APIExport found, can't create an APIBinding. Exiting ..."
      exit 1
    fi

    if [ -n "$CPS_KUBECONFIG" ]; then
      KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
      # Pass the parent workspace in which the service provider workspace is set, otherwise use root ws
      if [ -n "$2" ]; then 
        echo "Parent Workspace: " "${2}"
        KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use "${2}"
      else
        echo "Parent workspace is set in the root:org:user workspace itself..."
      fi
      
      # Copy the identity hash from the backend APIExport in the service workspace so we can reference it later in the appstudio APIBinding
      # Getting into service ws, to fetch apiexport identity hash and then switching back to parent ws
      if [ -n "$3" ]; then 
        echo "Service Provider Workspace: " "${3}"
        KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use "${3}"
        identityHash=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get apiexports.apis.kcp.dev gitopsrvc-backend-shared -o jsonpath='{.status.identityHash}')
        echo "Identity Hash for gitopsrvc-backend-shared APIExport: " "${identityHash}"
        KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws -
      else
        echo "----> Error: Can't retrieve identity hash for gitopsrvc-backend-shared"
        exit 1
      fi


      url=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get workspace "${SERVICE_WS}" -o jsonpath='{.status.URL}')
      if [ -z "$url" ]; then
        echo "url path should not be null, check service ws. Exiting ..."
        exit 1
      else
        path=$(basename "${url}")
      fi
    else 
      echo "--> Error: CPS_KUBECONFIG not found. Exiting ...s"
      exit 1
    fi

    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use "${USER_WS}" &> /dev/null
    permissionClaims='
  permissionClaims:
  - group: ""
    resource: "secrets"
    state: "Accepted"
  - group: ""
    resource: "namespaces"
    state: "Accepted"'

    if [ "${exportName}" == "gitopsrvc-appstudio-shared" ]; then
      permissionClaims="${permissionClaims}
  - group: \"managed-gitops.redhat.com\"
    resource: \"gitopsdeployments\"
    state: \"Accepted\"
    identityHash: ${identityHash}"
    fi

cat <<EOF > createAPIBinding.yaml
---
apiVersion: apis.kcp.dev/v1alpha1
kind: APIBinding
metadata:
  name: ${exportName}
spec:
${permissionClaims}
  reference:
    workspace:
      path: ${path}
      exportName: ${exportName}
EOF

    yamllint -c "${SCRIPTPATH}"/../utilities/yamllint.yaml "${SCRIPTPATH}"/../createAPIBinding.yaml

    printf "APIBinding yaml for %s \n" "${1}"
    cat createAPIBinding.yaml

    KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f "${SCRIPTPATH}"/../createAPIBinding.yaml
    rm createAPIBinding.yaml
}

permissionToBindAPIExport() {
    exit_if_binary_not_installed "yamllint"
  # Create permissions to bind APIExports. We need this workaround until KCP fixes the bug in their admission logic. Ref: https://github.com/kcp-dev/kcp/issues/1939    
    if [ -n "$CPS_KUBECONFIG" ]; then
      KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws
      # Pass the parent workspace in which the service provider workspace is set, otherwise use root ws
      if [ -n "$1" ]; then 
        KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use ${1}
      else
        echo "Service Provider workspace is set in the root:org:user workspace itself..."
      fi
      
      bindingName=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get clusterrolebinding | grep "${SERVICE_WS}" | awk '{print $1}')

      if [ -z "$bindingName" ]; then
        echo "bindingName should not be empty, check service ws. Exiting ..."
        exit 1
      else
        userName=$(KUBECONFIG="${CPS_KUBECONFIG}" kubectl get clusterrolebindings "${bindingName}" -o jsonpath='{.subjects[0].name}')
      fi
    else 
      echo "--> Error: CPS_KUBECONFIG not found. Exiting ...s"
      exit 1
    fi
    KUBECONFIG="${CPS_KUBECONFIG}" kubectl kcp ws use "${SERVICE_WS}" &> /dev/null

cat <<EOF > permissionToBindAPIExport.yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: bind-apiexport
rules:
- apiGroups:
  - apis.kcp.dev
  resourceNames:
  - gitopsrvc-backend-shared
  - gitopsrvc-appstudio-shared
  resources:
  - apiexports
  verbs:
  - bind
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: bind-apiexport
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: bind-apiexport
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: "$userName"
EOF

    yamllint -c "${SCRIPTPATH}"/../utilities/yamllint.yaml "${SCRIPTPATH}"/../permissionToBindAPIExport.yaml

    printf "permissionToBindAPIExport yaml: \n"
    cat permissionToBindAPIExport.yaml

    KUBECONFIG="${CPS_KUBECONFIG}" kubectl apply -f "${SCRIPTPATH}"/../permissionToBindAPIExport.yaml
    rm permissionToBindAPIExport.yaml
}