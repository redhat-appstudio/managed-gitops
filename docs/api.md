
# GitOps Service API Examples

This document describes the APIs supported by the GitOps Service. The examples provided here *might* differ slightly from their current form (but let us know if they do, so we can update this document).

The K8s resource *Go structs* defined in the `managed-gitops` repository, the `application-api` repository, and the corresponding CustomResourceDefinitions (CRDs), are the canonical representations of the current state of this API. This document flows downstream from those.

## Core vs App Studio Environment APIs

The App Studio Environment APIs implement (opinionated) concepts and APIs that are specific to App Studio, including `Environment`s, `Application`s, and `Component`s.
- Applications and Components are examples of App-Studio-specific concepts that relate to how an application is composed, and are primarily reconciled by the [application-service](https://github.com/redhat-appstudio/application-service) component.
- App Studio Environment APIs implement promotion of application versions between environments, via APIs such as `Environment`, `Snapshot`, and `SnapshotEnvironmentBinding`.
- App Studio API resources are translated by the AppStudio GitOps Service controllers into one or more Core GitOps Service API resources. For example:
    - A `SnapshotEnvironmentBinding` is reconciled into one or more `GitOpsDeployments`.
    - A `Environment` is usually reconciled into a `GitOpsDeploymentManagedEnvironment` 

In contrast, the Core GitOps Service is unopinionated, as the Core GitOps Service API translates directly into corresponding Argo CD API resources:
- `GitOpsDeployment` -> Argo CD `Application`
- `GitOpsDeploymentSyncRun` -> Argo CD Application sync operation 
(Argo CD has no support for triggering sync operations via CR)
- `GitOpsDeploymentRepositoryCredentials` -> Argo CD Repository `Secret`
- `GitOpsDeploymentManagedEnvironment` -> Argo CD Cluster `Secret`


## Core GitOps Service API

### GitOpsDeployment

The `GitOpsDeployment` custom resource (CR) describes an active continuous deployment from a source GitOps repository, to a destination namespace on a destination cluster (which may be the same cluster as where the API resources is defined).

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment

metadata:
  name: managed-environment-gitops-depl
  namespace: jane
  
  finalizers:
  # Optional: if this finalizer is specified, the GitOpsDeployment will only be cleaned up (deleted by Kubernetes) 
  # once all of the child resources of the GitOpsDeployment have been deleted.
  # This matches the behaviour of a similar Argo CD finalizer
  - resources-finalizer.managed-gitops.redhat.com

spec:

  # A reference to a GitOps repository to deploy from
  source:
    # Git URL of a repository containing Kubernetes resources
    repoURL: https://github.com/redhat-appstudio/managed-gitops
    # Path within a Git repository to deploy from
    path: resources/test-data/sample-gitops-repository/environments/overlays/dev

    # Optional: One can specify a specific Git commit to deploy
    targetRevision: (...)

  # A reference to a remote cluster (Environment) or local  
  # Optional: if not specified, defaults to the same namespace as the CR.
  destination:  
    # Optional: A reference to a ManagedEnvironment resource, containing credentials that can be used to deploy to an external
    # cluster
    environment: my-managed-environment

    # Optional: Target Namespace to deploy the resources to.
    # - If not specified, it will default to the same namespace as the GitOpsDeployment CR.
    # 
    # NOTE: this only applies to resources within the GitOps repository that do not
    # already have a non-empty .metadata.namepace field. 
    # (It does not override existing Namespace values)
    namespace: jane 

    
  # Optional: syncPolicy field allows control over how GitOps Service/Argo CD 
  # performs synchronize operations.
  syncPolicy: 

    # A list of key/value pairs options which control Argo synchronization
    syncOptions:
      # If 'CreateNamespace=true' is specified, Argo CD will ensure the Namespace specified
      # in the .spec.destination.namespace field is created before deploying the resources.
      # 
      # If false, or unspecified, the Namespace must already exist. This is the default behaviour.
      - CreateNamespace=true

  # GitOps Service has two sync behaviours:
  # - automated: changes to the GitOps repo immediately take effect (as soon as Argo CD detects them).
  # - manual: Will only deploys when a `GitOpsDeploymentSyncRun` resource is created.
  type: automated / manual

status:

  # SyncStatus contains information about the currently observed live and desired states of an application
  sync:
    # Whether the live state of the cluster is in sync with the target state in Git
    status: Synced / OutOfSync / Unknown

    # Revision contains information about the revision the comparison has been performed to
    revision: (git commit id)

  # Health contains information about the deployment's current health status
  health: 
    # Status indicates whether the resources being deployed are healthy (by Argo CD's definition of healthy)
    status: Healthy / Progressing / Degraded / Suspended / Missing / Unknown
    # Message is a human-readable informational message describing the health status
    message: (...)

  # List of child resources that have been deployed by the GitOpsDeployment
  resources:
    - group: (...)
      version: (...)
      kind: Secret
      namespace: jane
      name: my-secret
      status: Synced / OutOfSync / Unknown
      health: 
        status: Healthy / Progressing / Degraded / Suspended / Missing / Unknown
        message: (...)
    - (...)

  # ReconciledState contains the last version of the GitOpsDeployment resource that the Argo CD Controller reconciled
  # - This allows one to know whether user updates to the .spec field have been read/processed by the controller.
  reconciledState:
    source: # as defined in .spec field above
    destination: # as defined in .spec field above

  conditions:
    
    # ErrorOccurred indicates if an error occurred during reconcilation of the GitOpsDeployment.
    - type: ErrorOccurred
      reason: ErrorOccurred / ErrorOccurredResolved
      status: True / False / Unknown
      # Message contains human-readable message indicating details about the last condition.
      message: (...)

      # LastProbeTime is the last time the condition was observed.
      lastProbeTime: (...)

      # LastTransitionTime is the last time the condition transitioned from one status to another.
      lastTransitionTime: (...)

    # SyncError will display synchronize operation errors from the corresponding Argo CD Application.
    - type: SyncError
      reason: SyncError / SyncErrorResolved
      status: True / False / Unknown
      message: (human readable message from Argo CD on the cause of the sync error)

    # ComparisonError will display comparison errors from the corresponding Argo CD Application
    - type: ComparisonError
      reason: ComparisonError
      status: "True"
      lastTransitionTime: (...)
      message: (human readable message indicating the problem)

  operationState: # operationState field from the corresponding Argo CD Application. See Argo CD Application API for details
    operation:
      initiatedBy:
        automated: true
      retry:
        backoff:
          duration: 5s
          factor: 2
          maxDuration: 3m
        limit: -1
      sync:
        prune: true
        resources:
        - group: route.openshift.io
          kind: Route
          name: (...)
        revision: (...)
        syncOptions:
        - PrunePropagationPolicy=background
    message: "human readable message indicating the status"
    phase: Succeeded
    startedAt: "2023-08-17T18:01:26Z"
    finishedAt: "time finished at"    
    syncResult:
      resources:
      - (...)
      revision: # (...)
      source:
        path: # (...)
        repoURL: # (...)
        targetRevision: # (...)

```

This resource is reconciled (translated) into a corresponding [Argo CD Application Resource](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#applications), defined in an GitOps-Service-managed Argo CD namespace.

See the [GitOpsDeployment API reference](https://redhat-appstudio.github.io/book/ref/gitops.html#gitopsdeployment) for details.


### GitOpsDeploymentManagedEnvironment 

The `GitOpsDeploymentManagedEnvironment` CR describes a remote cluster which the GitOps Service will deploy to, via Argo CD. This resource references a `Secret` resource, of type `managed-gitops.redhat.com/managed-environment`, that contains the cluster credentials.

```yaml
# Describes the API URL and credentials for the target cluster 
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  name: my-managed-environment
  namespace: jane
spec:
  # URL of the cluster to connect to
  apiURL: "https://api.my-cluster.dev.rhcloud.com:6443"  

  # A reference to a Secret that contains cluster connection details. The cluster
  # details should be in the form of a kubeconfig file.
  credentialsSecret: "my-managed-environment-secret"

  # Optional: If true, the GitOps Service will allow Argo CD to connect to the specified cluster
  # even if it is using an invalid or self-signed TLS certificate.
  # Defaults to false.
  allowInsecureSkipTLSVerify: false

  # Optional: Controls whether Argo CD will use the ServiceAccount provided by the user in the Secret, or if a new ServiceAccount
  # should be created.
  # 
  # - If true, the GitOps Service will automatically create a ServiceAccount/ClusterRole/ClusterRoleBinding on the target cluster,
  #   using the credentials provided by the user in the secret. 
  #   - Argo CD will then be configured to deploy with that new ServiceAccount.
  #
  # - Default: If false, it is assumed that the credentials provided by the user in the Secret are for a ServiceAccount on the cluster, and
  #   Argo CD will be configred to use the ServiceAccount referenced by the Secret of the user. No new ServiceAccount will be created.
  #   - This should be used, for example, when the ServiceAccount Argo CD does not have full cluster access (*/*/* at cluster scope)
  createNewServiceAccount: false

  # Optional: the ServiceAccount that GitOps Service/Argo CD uses to deploy may not have access to all of the Namespaces on a cluster
  # If not specified, it is assumed that the Argo CD ServiceAccount has read/write at cluster-scope.
  # - If you are familiar with Argo CD: this field is equivalent to the field of the same name in the Argo CD Cluster Secret.
  namespaces:
    - bank-loan-app
    - bank-account-app

  # Optional: If the .spec.namespaces field is non-empty, this field will be used to determine whether Argo CD should 
  # attempt to manage cluster-scoped resources.
  # - If .spec.namespaces field is empty, this field is ignored.
  # - If you are familiar with Argo CD: this field is equivalent to the field of the same name in the Argo CD Cluster Secret.
  clusterResources: false

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
  # The 'kubeconfig' field must follow the format of a standard kubernetes config file.
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

See the [GitOpsDeploymentManagedEnvironment API reference](https://redhat-appstudio.github.io/book/ref/gitops.html#gitopsdeploymentmanagedenvironment) for details of other fields.

### GitOpsDeploymentRepositoryCredentials

The `GitOpsDeploymentRepositoryCredentials` resource is used to provide Git credentials for a private Git repository.
- This resource is not required to be defined for public Git repositories.

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentRepositoryCredentials
metadata:
  Name: private-repo-creds
spec:
  # URL of the private GitOps repository
  url: https://github.com/jgwest/private-app

  # A Secret containing username/password(PAT), or an SSH privte key, to allow
  # Argo CD to connect to the private repository
  secret: private-repo-creds-secret

---
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret
stringData:
  # URL of the GitOps Repository, must match above.
  url: https://github.com/jgwest/my-private-app
  # and:
  username: jgwest
  password: (my password)
  # or:
  sshPrivateKey: (...)
```

These resources roughly translate into an [Argo CD Repository Credentials `Secret`](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repository-credentials)

See the [GitOpsDeploymentRepositoryCredentials API reference](https://redhat-appstudio.github.io/book/ref/gitops.html#gitopsdeploymentrepositorycredential) for field details.

### GitOpsDeploymentSyncRun

The `GitOpsDeploymentSyncRun` resource is use to manually trigger an Argo CD synchronize operation: this tells Argo CD to refresh the GitOps repository, then deploy the latest Git repository contents to the target cluster.

Upon creation of this resource, the GitOps Service will trigger a deployment of the Kubernetes resources describe in the GitOps repository, to the target environment (cluster/workspace).

The `GitOpsDeploymentSyncRun` resource is not required when the `GitOpsDeployment` is of type `automated`. 
- When automated, any changes to the GitOps repository will automatically be deployed to the target environment.
- Attempting to SyncRun on an automated `GitOpsDeployment` will return an error in the `.status` field.

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentSyncRun
spec:
  # Reference to the target GitOpsDeployment to issue the synchronization operation to
  gitopsDeploymentName: jgwest-app 

  # Optional: To tell Argo CD to deploy a particular git commit SHA, specify it here.
  revisionId: (...) 

status: 
  health: Healthy # (enum from Argo CD Application health field: Healthy / Progressing / Degraded / Suspended / Missing / Unknown)
  syncStatus: Synced # (enum from Argo CD status: Synced / OutOfSync)
  conditions:
    - type: ErrorOccurred
      reason: ErrorOccurred
      status: True / False / Unknown
      # message is a human-readable message, indictating error details, if present.
      message: "Successfully completed synchronize operation."
      lastTransitionTime: "2022-10-04T02:19:14Z"
```

Behind the scenes, this will trigger a manual sync of the corresponding Argo CD `Application`. The manual sync will cause Argo CD to ensure that the K8s resources described in the GitOps repository are consistent with what is on the target cluster.

This resource has no corresponding Argo CD CR equivalent: with Argo CD, a manual sync operation can only be triggered via the Web/GRPC API (for example, via the argocd CLI). In this case, the GitOps Service uses the Web API.

See the [GitOpsDeploymentSyncRun API reference](https://redhat-appstudio.github.io/book/ref/gitops.html#gitopsdeploymentsyncrun) for details of other fields.

## GitOps Service: App Studio Environment APIs

The App Studio Environment API is based on the [Application](https://redhat-appstudio.github.io/book/ref/application-environment-api.html#application), and [Component](https://redhat-appstudio.github.io/book/ref/application-environment-api.html#component) APIs, which are primarily handled by the [application-service](https://github.com/redhat-appstudio/application-service) component. 
- These APIs are opinionated representations of the constituent parts of a user's application: a application (e.g. a loan application of a bank) contains multiple components (e.g. Node frontend, Java backend, Postgresql database).

The App Studio Environment APIs -- which are simultaneously reconciled by multiple App Studio components -- are all related to provisioning or deploying K8s resources to external environments, such as external K8s clusters/namespaces or local namespaces.

### Environment

Environment describes a target deployment location for deployments of Kubernetes resources, with the source for those Kubernetes resources coming from a GitOps repository.

An Environment could be namespace on an App Studio cluster, an Environment could be a full remote cluster, or an Environment could be another type of environment that is not yet implemented as of this writing.


```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Environment
metadata:
  name: staging
spec:
  # A user-visible, user-definable name for the Environment
  displayName: “Staging for Team A”

  # Strategy for promotion between Environments:
  # - AppStudioAutomated: 
  #   - A successful promotion to a parent environment will automatically promote to a child environment
  #   - An environment without a parent will be automatically promoted to.
  # - Manual:
  #   - A user action is required to promote from one environment to naother.
  deploymentStrategy: AppStudioAutomated / Manual

  # Environments exist in a digraph indicating promotion flow.
  # E.g. Dev -> Staging -> Production
  parentEnvironment: staging

  # Optional: tags are a user-visible, user-definable set of tags that can be applied to the environment
  tags:
    - staging

  # Contains environment-specific configurations, such as environment variables to use
  # when running container workloads in this environment.
  configuration:
    env:
      - name: My_STG_ENV
        value: "100"
  
  # Optional: this field is marked as 'unstable', indicating it is expected to change significantly.
  unstableConfigurationFields:
    # Contains K8s credentials of a remote cluster. 
    # These fields correspond to GitOpsDeploymentManagedEnvironment fields. See details there.
    kubernetesCredentials:
      apiURL: https://api.ci-ln-a1b2c3d4e5-76543.origin-ci-int-gce.dev.rhcloud.com:6443
      targetNamespace: my-namespace
      clusterCredentialsSecret: secret-containing-cluster-credential
      allowInsecureSkipTLSVerify: true/false

```

See the [Environment API reference](https://redhat-appstudio.github.io/book/ref/application-environment-api.html#environment) for details of other fields.


### Snapshot

Snapshot describes a set of container image versions for an Application. For example, you might have a 'bank-loan-app' `Application`, with `Component`s  'frontend', 'backend', and 'database'.

The Snapshot would describe a particular (combination of) versions of container images for that Application. For example:
- component 'frontend': `quay.io/jgwest-redhat/bank-loan-frontend:aaaaaa`
- component 'backend': `quay.io/jgwest-redhat/bank-loan-frontend:bbbbb`
- component 'database': `quay.io/postgresql/postgresql:13.9`

In order to update a component to a new container image, one needs to creates a new Snapshot. For example, here one creates a new Snapshot with a new backend version:
- frontend: `quay.io/jgwest-redhat/bank-loan-frontend:aaaaaa`
- backend: `quay.io/jgwest-redhat/bank-loan-frontend:ccccc1234` (new value)
- database: `quay.io/postgresql/postgresql:13.9`

The contents of a Snapshot's `components` field are immutable once created, and are used with the SnapshotEnvironmentBinding resource to deploy a set of versions of an Application to a target Environment.

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
metadata:
  name: my-snapshot
spec:
  # Application is a reference to the name of an Application resource within the same namespace, which defines the target application for the Snapshot.
  application: "new-demo-app"

  # User-visible, user-definable name for the resource
  displayName: "My known good staging snapshot"

  # User-visible, user definable description for the resource
  displayDescription: "The best we've had so far!"

  # Container images for each of the Components of the Application.
  components:
    - name: component-a
      containerImage: quay.io/jgwest-redhat/sample-workload:latest

  # Optional: an arbitrary, untyped json field for arbitrary artifact data.
  # Once we better understand this requirement, this should be converted to strong typing.
  artifacts:
    # (...) - 
status:
  # Conditions field is not currently used by GitOps Service, but is used by other App Studio components.
  conditions:
    - # (...)
```

See the [Snapshot API reference](https://redhat-appstudio.github.io/book/ref/application-environment-api.html#snapshot) for field details.


### SnapshotEnvironmentBinding 

The `SnapshotEnvironmentBinding` resource specifies the deployment relationship between (a single application, single environment, and a single snapshot) combination.

It can be thought of as a 3-tuple that defines what Application should be deployed to what Environment, and which Snapshot should be deployed (Snapshot being the specific component container image versions of that Aplication that should be deployed to that Environment).

**Note**: There should not exist multiple SnapshotEnvironmentBinding CRs in a Namespace that share the same Application and Environment value. For example:
- Good:
    - SnapshotEnvironmentBinding A: (application=appA, environment=dev, snapshot=my-snapshot)
    - SnapshotEnvironmentBinding B: (application=appA, environment=staging, snapshot=my-snapshot)    
- Bad:
    - SnapshotEnvironmentBinding A: (application=*appA*, environment=*staging*, snapshot=my-snapshot)
    - SnapshotEnvironmentBinding B: (application=*appA*, environment=*staging*, snapshot=second-snapshot)

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: SnapshotEnvironmentBinding
metadata:
  name: appa-staging-binding
  labels:
    appstudio.application: new-demo-app
    appstudio.environment: staging
spec:
  # Application is a reference to the Application resource (defined in the same namespace) that we are deploying as part of this SnapshotEnvironmentBinding.
  application: new-demo-app

  # Environment is the environment resource (defined in the namespace) that the binding will deploy to.
  environment: staging

  # Snapshot is the Snapshot resource (defined in the namespace) that contains the container image versions for the components of the Application.
  snapshot: my-snapshot

  # Component-specific configuration information, used when generating GitOps repository resources.
  components:
    - name: component-a # ref to a Component in the Namespace
      configuration:
        env: # Env vars for component-a in this Environment
          - name: My_STG_ENV
            value: "200"
        replicas: 3 # Replicas for component-a in this Environment
status:
  # GitOpsDeployments describes the set of GitOpsDeployment resources that are owned by the SnapshotEnvironmentBinding, and are deploying the Components of the Application to the target Environment.
  gitopsDeployments:
    # for each Component of the Appliction
    - componentName: (name of Component of an Application)
      gitopsDeployment: (name of GitOpsDeployment that deploys this Component to this Environment)
      syncStatus: Synced / OutOfSync / Unknown
      health: Healthy / Progressing / Degraded / Suspended / Missing / Unknown
      commitID: # (...)


  # Components describes a component's GitOps repository information.
  # This status is updated by the Application Service controller.
  components:
    - name: component-a
      gitopsRepository:
        url: "https://github.com/redhat-appstudio/managed-gitops"
        branch: main
        path: components/componentA/overlays/staging
        generatedResources:
          - abc.yaml

  # Describes operations on the GitOps repository, for example, if there were issues with generating/processing the repository.
  # This status is updated by the Application Service controller.
  gitopsRepoConditions:
    - # (see application-service docs for details)

  # bindingConditions will contain error messages from the SnapshotEnvironmentBinding reconciler.
  bindingConditions:
    - type: ErrorOccurred
      status: True/False/Unknown
      reason: ErrorOccurred
      message: # Human readable error message indicating the specific error

  # ComponentDeploymentConditions describes the deployment status of all of the Components of the Application.
  # This status is updated by the Gitops Service's SnapshotEnvironmentBinding controller. 
  componentDeploymentConditions:
    - type: AllComponentsDeployed
      status: True/False/Unknown
      reason: CommitsSynced/CommitsUnsynced/ErrorOccurred
      message: 1 of 3 components deployed # human readable message
  
``` 

See the [SnapshotEnvironmentBinding API reference](https://redhat-appstudio.github.io/book/ref/application-environment-api.html#snapshotenvironmentbinding) for details.


### PromotionRun (WIP)

PromotionRun currently only supports manual promotion, but was originally conceived to handle both automated and manual promotion. 

Promotion via PromotionRun CR was part of the original Environment API design, but this CR may no longer be required, as promotion is primarily handled by HACBS components.

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: PromotionRun  # better name suggestions welcome
metadata:
  name: appA-manual-promotion
  # labels for application/environment

spec:
  snapshot: my-snapshot # reference to snapshot to promote between environments.
  application: appA  # application to target

  # Fields specific to manual promotion.
  # Only one field should be defined: either 'manualPromotion' or 'automatedPromotion', but not both.
  manualPromotion:
    targetEnvironment: staging # promote TO this environment
    
  # Fields specific to automated promotion
  # Only one field should be defined: either 'manualPromotion' or 'automatedPromotion', but not both.
  # NOTE: not currently supported, as of this writing (March 2023).
  automatedPromotion:
    initialEnvironment: staging # start iterating through the digraph, beginning with the value specified in 'initialEnvironment'

status:

  # Whether or not the overall promotion (either manual or automated is complete)
  state: Active # Waiting (not yet scheduled) / Active (in progress) / Completed (either successfully/unsuccessfully)

  # on completion:
  completionResult: Success / Failure

  # For an automated promotion, there will be multiple steps (one for each promotion within the promotion process, beginning with targetEnvironment)
  # For a manual promotion, there will only be the first step (the promotion to the targetEnvironment)
  environmentStatus:
    - step: 1
      environmentName: poc
      status: success
    - step: 2
      environmentName: dev
      status: success
    - step: 3
      environmentName: stage
      status: in-progress
    - (...)

  # For an automated promotion, there can be multiple active bindings at a time (one for each env at a particular tree depth)
  # For a manual promotion, there will be only one.
  activeBindings:
    - appA-staging1
    - appA-staging2
    - appA-staging3
```

See the [PromotionRun API reference](https://redhat-appstudio.github.io/book/ref/application-environment-api.html#promotionrun) for details.


## Internal GitOps Service API

The internal GitOps Service APIs are only for use by GitOps Service components. End users of the GitOps Service APIs should **not** be able to create/modify/delete/view this resource on the K8s cluster.

### Operation

The Operation resource is used to communicate between the backend and cluster-agent components. 

The Operation resource has two purposes:
- To allow the backend to inform, via an async message, the cluster-agent of a change that is required to an Argo CD resource (Cluster Secret, Repository Secret, or Argo CD application). This async message is known as an 'operation'.
- To allow the cluster-agent to inform whether the the operation was successful or failed, and provide error details.

Many more details on operations are available from the GitOps Service Google Drive folder.

### Operation

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: Operation

metadata:
  name: operation-(uuid)
  namespace: gitops-service-argocd # must be created within an Argo CD namespace

spec:
  # Points to a row in the Operation table of teh database
  operationID: (uuid reference to Operation row in DB)
```

See the [Operation API reference](https://redhat-appstudio.github.io/book/ref/gitops.html#operation) for details.


## DeploymentTarget/DeploymentTargetClaim/DeploymentTargetClass

See [ADR-8](https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md) for details on `DeploymentTarget` (DT) / `DeploymentTargetClaim` (DTC) / `DeploymentTargetClass` (DTClass) provisioning. See [DT/DTC/DTClass step by step](deployment-target-claim-class-steps.md) for an example of how the DT/DTC/DTClass CRs operate at each step. 

In short:
- This API is patterned off of the `PersistentVolume`/`PersistentVolumeClaim`/`StorageClass` API.
- A `DeploymentTarget` refers to a target namespace/cluster to deploy to (in the same way that a PersistentVolume represents a volume, e.g. data storage).
- A `DeploymentTargetClaim` binds to a DeploymentTarget, becoming the owner of the DeploymentTarget (in the same way that a PersistentVolumeClaim binds to a PersistentVolume, which then allows the user to store data via the PersistentVolumeClaim)
- A `DeploymentTargetClass` indicates the type of target to be provisioned: at present, we only support SpaceRequest provisioning, which is the ability to request a temporary Namespace on an RHTAP cluster to deploy to.
- Finally, in order to deploy to a cluster/namespace that is bound to a DeploymentTargetClaim, you must reference that DeploymentTargetClaim from the `.spec` field of AppStudio Environment (see below for example).


In order to deploy (using GitOps Service) to a cluster/namespace referenced by a DeploymentTargetClaim, one needs to create an AppStudio Environment resource that references that DeploymentTargetClaim, via the Enviroinment's `.spec.configuration.target.deploymentTargetClaim.claimName` field:
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Environment
metadata:
  name: prod
spec:
  # (...)
  
  configuration:

  # Target is used to reference a DeploymentTargetClaim for a target Environment.
  # The Environment controller uses the referenced DeploymentTargetClaim to access its bounded
    # DeploymentTarget with cluster credential secret.
    target:
      deploymentTargetClaim:
        claimName: staging-dtc # refers to a name of DTClaim in the Namespace
```

The `DeploymentTargetClaim` API resource is similar to Kubernetes' PersistentVolumeClaim resource. DeploymentTargetClaim is Namespace-scoped:
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTargetClaim
metadata:
  name: prod-dtc
  annotations:
    # Annotation to indicate that the binding controller completed the binding process. 
    dt.appstudio.redhat.com/bind-complete: true

    # Annotation to indicate that the binding controller bind the DTC to a DT.
    # In practice it means that the controller set the DTC.spec.target to the value of DT.Name
    dt.appstudio.redhat.com/bound-by-controller: true
spec:
  # Reference to a cluster-scoped DeploymentTargetClass that provisioned the DT to which this DTC is bound
  deploymentTargetClassName: isolation-level-namespace
  
  # Name of a DeploymentTarget within the Namespace to which this DeploymentTargetClaim should be bound.
  targetName: dtc-9rpj6-dt-wrn7s

status:

  # Pending: DTC is waiting for the binding controller or user to bind it with a DT that satisfies it.
  # Bound: The DTC was bounded to a DT that satisfies it, by the binding controller.
  # Lost: The DTC lost its bounded DT. The DT doesn’t exist anymore because it got deleted.
  phase: Bound
```


The `DeploymentTarget` API resource is similar to Kubernetes' PersistentVolume resource. DeploymentTarget is Namespace-scoped:
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTarget
metadata:
  name: prod-dt
  annotations:
    # the name of the provisioner that provisioned this DT: shoud match .spec.provisioner field of a DeploymentTargetClass
    provisioner.appstudio.redhat.com/provisioned-by: appstudio.redhat.com/devsandbox

spec:
  # Reference to cluster-scoped DeploymentTargetClass which provisioned this DeploymentTarget.
  deploymentTargetClassName: "isolation-level-namespace"
  
  # Reference to the DeploymentTargetClaim (in the same Namespace) that a DeploymentTarget is bound to (if it is bound)
  claimRef: "dtc-52m96"
  
  # K8s credentials that a consumer of this API can use to connect to a ServiceAccount on a target cluster with read/write permissions to a given Namespace
  kubernetesCredentials:
    
    # Namespace that the credential provides access to
    defaultNamespace: "jgwest-jk9881c-env"
    
    # URL to a cluster's Kubernetes API endpoint
    apiURL: 'https://api.stone-stg-m01.7ayg.p1.openshiftapps.com:6443'
    
    # A reference to a Secret in the Namespace.
    # The Secret should contain a kubeconfig file within the 'data.kubeconfig' field.
    clusterCredentialsSecret: "dtc-1abc2-de3f4-gh5ij"

    # Whether or not the consumer of this Secret should allow/expect that the k8s API endpoint is using a self-signed certificate.
    allowInsecureSkipTLSVerify: "true / false"


status:

  # Available: DT waits for a Claim to be bound to.
  # Pending: DT is not yet available for binding.
  # Bound: DT was bounded to a DTC.
  # Released: DT was previously bound to a DTC, and the DTC was deleted, BUT the external resources were not freed (for example, because the DeploymentTargetClass had a reclaimPolicy of Retain)
  # Failed: DT was released from its claim, but there was a failure during the release of external resources.
  phase: Available / Pending / Bound / Released / Failed


```

The DeploymentTargetClass API is similar to Kubernetes' StorageClass resource. It is cluster-scoped:
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTargetClass
metadata:
  name: isolation-level-namespace
spec:

  # The name of the provisioner that handles this class. 
  # As of this writing (August 2023) only one provisioner is supported, 'appstudio.redhat.com/devsandbox'
  provisioner: "appstudio.redhat.com/devsandbox"
  
  # The reclaimPolicy field will tell the provisioner what to do with the DT
  # once its corresponding DTC is deleted, the values can be Retain or Delete.  
  # Retain: When the DTC is deleted, do not delete the DT: only release the DT so it can be bound to another DTC at a later time
  # Delete: When the DTC is deleted, also delete the DT
  reclaimPolicy: Retain / Delete
  
  parameters: {} # As of this writing (August 2023) there are no supported parameters.
  
```  
