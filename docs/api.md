
# GitOps Service API

## Core GitOps Service API

### GitOpsDeployment

The `GitOpsDeployment` custom resource (CR) describes an active continous deployment from a GitOps repository, to a namespace (workspace).

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment

metadata:
  name: managed-environment-gitops-depl
  namespace: jane 

spec:

  # Application/component to deploy
  source:
    repoURL: https://github.com/redhat-appstudio/gitops-repository-template
    path: environments/overlays/dev

  destination:  # destination is user workspace if empty
    environment: my-managed-environment
    namespace: jane # NOTE: namespace must exist on remote cluster

  # Only 'automated' type is currently supported: changes to the GitOps repo immediately take effect (as soon as Argo CD detects them).
  type: automated
  # type: 'manual' # Will only deploys when a `GitOpsDeploymentSyncRun` resource is created.
```

This resource is roughly translated into an [Argo CD Application Resource](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#applications).

### GitOpsDeploymentManagedEnvironment 

The `GitOpsDeploymentManagedEnvironment` CR describes a remote cluster (or KCP workspace) which the GitOps Service will deploy to (via Argo CD). This resource references a second `Secret` resource, of type `managed-gitops.redhat.com/managed-environment`, that contains the cluster credentials.

```yaml
# Describes the API URL and credentials for the target cluster 
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  name: my-managed-environment
  namespace: jane
spec:
  apiURL: "https://api.my-cluster.dev.rhcloud.com:6443"  
  credentialsSecret: "my-managed-environment-secret"

---
# The GitOpsDeploymentManagedEnvironment references a Secret, containing the connection information
# - Kubeconfig credentials for the target cluster (as a Secret)
apiVersion: v1
kind: Secret
metadata:
  name: my-managed-environment-secret
  namespace: jane
type: managed-gitops.redhat.com/managed-environment
data:
  # Note: This would be base 64 when stored on the cluster
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        insecure-skip-tls-verify: true
        server: https://api.my-cluster.dev.rhcloud.com:6443
      name: api-my-cluster-dev-rhcloud-com:6443
    contexts:
    - context:
        cluster: api-my-cluster-dev-rhcloud-com:6443
        namespace: default
        user: kube:admin/api-my-cluster-dev-rhcloud-com:6443
      name: default/api-my-cluster-dev-rhcloud-com:6443/kube:admin
    current-context: default/api-my-cluster-dev-rhcloud-com:6443/kube:admin
    kind: Config
    preferences: {}
    users:
    - name: kube:admin/api-my-cluster-dev-rhcloud-com:6443
      user:
        token: sha256~ABCdEF1gHiJKlMnoP-Q19qrTuv1_W9X2YZABCDefGH4
```

These resources roughly translate into an [Argo CD Cluster `Secret`](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters).

### GitOpsDeploymentRepositoryCredentials (*in-progress*)

The `GitOpsDeploymentRepositoryCredentials` resource is used to provide Git credentials for a private Git repository.

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentRepositoryCredentials
metadata:
  Name: private-repo-creds
spec:
  url: https://github.com/jgwest/private-app
  secret: private-repo-creds-secret

---
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret
stringData:
  url: https://github.com/jgwest/my-private-app
  # and:
  username: jgwest
  password: (my password)
  # or:
  sshPrivateKey: (...)
```

These resources roughly translate into an [Argo CD Repository Credentials `Secret`](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repository-credentials)



### GitOpsDeploymentSyncRun (*in-progress*)

The `GitOpsDeploymentSyncRun` resource is use to manually trigger a synchronize operation (this can be roughly thought of as the equivalent to a 'deployment' operation). 

Upon creation of this resource, the GitOps Service will trigger a deployment of the Kubernetes resources describe in the GitOps repository, to the target environment (cluster/workspace).

The `GitOpsDeploymentSyncRun` resource is not required when the `GitOpsDeployment` is of type `automated`. When automated, any changes to the GitOps repository will automatically be deployed to the target environment.

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentRepositoryCredentials
metadata:
  Name: private-repo-creds
spec:
  url: https://github.com/jgwest/app
  secret: private-repo-creds-secret

---
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret
stringData:
  url: https://github.com/jgwest/app
  # and:
  username: jgwest
  password: (my password)
  # or:
  sshPrivateKey: (...)
```

Behind the scenes, this will trigger a manual sync of the corresponding Argo CD `Application`. The manual sync will cause Argo CD to ensure that the K8s resources described in the GitOps repository are consistent with what is on the target cluster.

This resource has no corresponding Argo CD CR equivalent: with Argo CD, a manual sync operation can only be triggered via the Web/GRPC API (for example, via the argocd CLI). In this case, the GitOps Service uses the Web API.

## AppStudio GitOps Service API

### Environment (*in-progress*)

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Environment
metadata:
  name: staging
spec:
  type: poc
  displayName: “Staging for Team A”
  deploymentStrategy: AppStudioAutomated
  parentEnvironment: staging
  tags:
    - staging
  configuration:
    env:
      - name: My_STG_ENV
        value: "100"
```

### ApplicationSnapshot  (*in-progress*)

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: ApplicationSnapshot
metadata:
  name: my-snapshot
spec:
  application: "new-demo-app"
  displayName: "My known good staging snapshot"
  displayDescription: "The best we've had so far!"
  components:
    - name: component-a
      containerImage: quay.io/jgwest-redhat/sample-workload:latest
```

### ApplicationSnapshotEnvironmentBinding  (*in-progress*)

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: ApplicationSnapshotEnvironmentBinding
metadata:
  name: appa-staging-binding
  labels:
    appstudio.application: new-demo-app
    appstudio.environment: staging
spec:
  application: new-demo-app
  environment: staging
  snapshot: my-snapshot
  components:
    - name: component-a
      configuration:
        env:
          - name: My_STG_ENV
            value: "200"
        replicas: 3
status:
  components:
    - name: component-a
      gitopsRepository:
        url: "https://github.com/redhat-appstudio/gitops-repository-template"
        branch: main
        path: components/componentA/overlays/staging
        generatedResources:
          - abc.yaml
``` 

### ApplicationPromotionRun  (*in-progress*)

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: ApplicationSnapshotEnvironmentBinding
metadata:
  name: appa-staging-binding
  labels:
    appstudio.application: new-demo-app
    appstudio.environment: staging
spec:
  application: new-demo-app
  environment: staging
  snapshot: my-snapshot
  components:
    - name: component-a
      configuration:
        env:
          - name: My_STG_ENV
            value: "200"
        replicas: 3
status:
  components:
    - name: component-a
      gitopsRepository:
        url: "https://github.com/redhat-appstudio/gitops-repository-template"
        branch: main
        path: components/componentA/overlays/staging
        generatedResources:
          - abc.yaml
```
