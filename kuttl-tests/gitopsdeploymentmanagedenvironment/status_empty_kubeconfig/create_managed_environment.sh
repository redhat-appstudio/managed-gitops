NAMESPACE=gitops-service-e2e
SECRET_NAME=empty-kubeconfig-secret
SERVER_URL=$(kubectl config view -o jsonpath='{.clusters[0].cluster.server}')

echo "apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n" > ./kubeconfig.txt

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
