NAMESPACE=gitops-service-e2e
SECRET_NAME=bad-cert-secret
SERVER_URL=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')

CONFIG_CONTENTS= $(kubectl config vew)
kubectl create secret generic ${SECRET_NAME} --type managed-gitops.redhat.com/managed-environment --from-literal=kubeconfig=$CONFIG_CONTENTS -n ${NAMESPACE}


echo "apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  name: gitops-managedenvironment-dev
  namespace: ${NAMESPACE}
spec:
  allowInsecureSkipTLSVerify: true
  apiURL: ${SERVER_URL}
  credentialsSecret: ${SECRET_NAME}" | kubectl apply -f -
