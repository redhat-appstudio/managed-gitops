NAMESPACE=gitops-service-e2e
SECRET_NAME=no-current-context-secret
SERVER_URL=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')
KUBECONFIG_FILE=$HOME/.kube/config
if [[ ! -z "$KUBECONFIG" ]]; then
  KUBECONFIG_FILE=$KUBECONFIG
fi

cat $KUBECONFIG_FILE | yq 'del(.current-context)' > ./kubeconfig.txt

kubectl create secret generic ${SECRET_NAME} --type managed-gitops.redhat.com/managed-environment --from-file=kubeconfig=./kubeconfig.txt -n ${NAMESPACE}

rm -f ./kubeconfig.txt

echo "apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  name: gitops-managedenvironment-dev
  namespace: ${NAMESPACE}
spec:
  allowInsecureSkipTLSVerify: true
  apiURL: ${SERVER_URL}
  credentialsSecret: ${SECRET_NAME}" | kubectl apply -f -
