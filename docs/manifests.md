# Information about the Manifests

## RBAC and Deployments

Into the `Manifests` directory, you will find all the `YAML` files (**except from their CRDs**) for all the 3 main components, `backend`, `cluster-agent` and `postgres`.

```shell
manifests directory

├── backend-rbac
│   ├── managed-gitops-backend-controller-manager-metrics-service.yaml
│   ├── managed-gitops-backend-controller-manager_serviceaccount.yaml
│   ├── managed-gitops-backend-leader-election-role.yaml
│   ├── managed-gitops-backend-leader-election-rolebinding.yaml
│   ├── managed-gitops-backend-manager-config.yaml
│   ├── managed-gitops-backend-manager-role.yaml
│   ├── managed-gitops-backend-manager-rolebinding.yaml
│   ├── managed-gitops-backend-metrics-leader.yaml
│   ├── managed-gitops-backend-proxy-role.yaml
│   └── managed-gitops-backend-proxy-rolebinding.yaml

├── cluster-agent-rbac
│   ├── managed-gitops-clusteragent-controller-manager-metrics-service.yaml
│   ├── managed-gitops-clusteragent-controller-manager_serviceaccount.yaml
│   ├── managed-gitops-clusteragent-leader-election-role.yaml
│   ├── managed-gitops-clusteragent-leader-election-rolebinding.yaml
│   ├── managed-gitops-clusteragent-manager-config.yaml
│   ├── managed-gitops-clusteragent-manager-role.yaml
│   ├── managed-gitops-clusteragent-manager-rolebinding.yaml
│   ├── managed-gitops-clusteragent-metrics-leader.yaml
│   ├── managed-gitops-clusteragent-proxy-role.yaml
│   └── managed-gitops-clusteragent-proxy-rolebinding.yaml

├── postgresql-staging
│   ├── postgresql-staging.yaml

└── routes.yaml # Currently not used
```

These are automatically deployed when the `make deploy-(component)-rbac`targets are triggered as dependency of their respective targets.

## CRDs

### Backend Component

It requires 2 CRDs, into `gitops` namespace.

1. [gitopsdeployments](../backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml)
2. [gitopsdeploymentsyncruns](../backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml)

These are automatically deployed, when the `make deploy-backend-crd` is triggered as a dependency.

### Cluster Agent

It requires 2 CRDs:

1. [operations](../backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml) into `gitops` namespace.
2. [application from ArgoCD](https://raw.githubusercontent.com/argoproj/argo-cd/v2.5.1/manifests/crds/application-crd.yaml) into `argocd` namespace.

These are automatically deployed, when the `make deploy-cluster-agent-crd` is triggered as a dependency.

### AppStudio Controller

It requires 1 CRD (as of this writing):
1. [application](https://github.com/redhat-appstudio/application-service/blob/main/api/v1alpha1/application_types.go)

## How the manifests were generated

Do it the same way both Operator SDK and Kubebuiler do.
This is an example:

```shell=
cd backend
# Note: Put your own container image
cd config/manager && kustomize edit set image controller="quay.io/pgeorgia/gitops-service:latest"
cd ../../
kustomize build config/default  > deployment.yaml
```
