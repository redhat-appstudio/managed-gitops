NAMESPACE=gitops-service-e2e
SECRET_NAME=my-managed-env-secret
SERVER_URL=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')
KUBECONFIG_FILE=$HOME/.kube/config
if [[ ! -z "$KUBECONFIG" ]]; then
  KUBECONFIG_FILE=$KUBECONFIG
fi

kubectl create secret generic ${SECRET_NAME} --type managed-gitops.redhat.com/managed-environment --from-file=kubeconfig=${KUBECONFIG_FILE} -n ${NAMESPACE}


echo "apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  name: gitops-managedenvironment-dev
  namespace: ${NAMESPACE}
spec:
  allowInsecureSkipTLSVerify: true
  apiURL: ${SERVER_URL}
  credentialsSecret: ${SECRET_NAME}" | kubectl apply -f -
