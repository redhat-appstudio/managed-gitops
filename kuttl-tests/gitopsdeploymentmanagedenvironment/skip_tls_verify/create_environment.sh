NAMESPACE=gitops-service-e2e
SECRET_NAME=my-managed-env-secret
SERVER_URL=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')
SKIP_TLS_VERIFY=${1:-false}

KUBECONFIG_FILE=$HOME/.kube/config
if [[ ! -z "$KUBECONFIG" ]]; then
  KUBECONFIG_FILE=$KUBECONFIG
fi

kubectl create secret generic ${SECRET_NAME} --type managed-gitops.redhat.com/managed-environment --from-file=kubeconfig=${KUBECONFIG_FILE} -n ${NAMESPACE}


echo "apiVersion: appstudio.redhat.com/v1alpha1
kind: Environment
metadata:
  name: staging
  namespace: ${NAMESPACE}
spec:
  type: non-poc
  displayName: “UAT for Team A”
  deploymentStrategy: AppStudioAutomated
  unstableConfigurationFields:
    kubernetesCredentials:
      allowInsecureSkipTLSVerify: ${SKIP_TLS_VERIFY}
      apiURL: ${SERVER_URL}
      targetNamespace: ${NAMESPACE}
      clusterCredentialsSecret: ${SECRET_NAME}" | kubectl apply -f -
