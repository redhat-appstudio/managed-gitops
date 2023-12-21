# Argo CD Application API suitability for a KCP-based Managed GitOps Service

### Written by
- Jonathan West (@jgwest)
- Originally written September 26th, 2022

One of the requirements we got from folks early on in the project was that we did not want to expose Argo CD-specific features. E.g. that we were building GitOpsaaS, not ArgoCDaaS.

That is, that the GitOps Service should be an abstraction over the underlying GitOps tool (including not exposing/supporting the Argo CD CLI/UI).

Thus a GitOps-agnostic API was the API [I proposed all the way back in October 2021](initial-api-design.md), and implemented in late 2021 and early 2022.

So with this requirement, and my knowledge of existing issues with the Argo CD API (see below), and the speed that Argo CD multitenancy improvements have taken in being merged by Argo CD principals, I didn’t put a ton of additional thought into whether to support Argo CD Application as our target API (and no concerns were raised).

However, this is a useful opportunity to codify some of my thoughts on why I don’t believe the Argo CD Application API is a good choice for a large-scale, public managed service (and especially one on KCP).


## So here is my post facto case against directly using the Argo CD Application API:

### 1) Supporting the Argo CD Application API would mean locking us into that API: we can’t add new fields, for example, to support deployment to KCP workspace (or other KCP/Red Hat use cases).

For example, it would be great to be able to do this:

```yaml
kind: Application
spec:
  source:
    repoURL: (...)
    path: (...)
    
  destination:   
    # deploy to a child workspace:
    kcpWorkspace: staging
    
    # or, deploy to an absolute path:
    kcpWorkspacePath: :root:users:jgwest:staging:staging1
    
    namespace: my-app
```

But, of course, we can’t add any fields to the Application API that aren’t upstream (and/or we REALLY SHOULDN’T). And while, the ‘.spec.destination’ field does include a name field, this refers to an opaque cluster secret, with its own fixed API.

In contrast, here is the GitOps Service version of this API, which has the desired property of making it easy to target a KCP workspace with Argo CD, while retaining the main Argo CD API fields:

```yaml
kind: GitOpsDeployment
spec:
  source:
    repoURL: (...)
    path: (...)
    
  destination:
    environment: staging-kcp-workspace # reference to KCP workspace via managed environment
    namespace: my-app
```

and

```yaml
kind: ManagedEnvironment
metadata:
  name: staging-kcp-workspace

spec:
  kcpWorkspace: staging
  # or
  kcpWorkspacePath: :root:users:jgwest:staging:staging1
  
  kcpCredentials:
    # any additional credentials that are needed to connect to the KCP workspace, e.g. if the workspace is not a child workspace
    credentialsSecret: # (...)
```

(ManagedEnvironment is the GitOps Service equivalent to an Argo CD cluster secret)


### 2) It locks us into Argo CD as our GitOps technology of choice.

One of the requirements we got from folks early on in the project was that we did not want to expose Argo CD-specific features. That is, that the GitOps Service would be an abstraction over the underlying GitOps tool. This includes not exposing/supporting the Argo CD CLI/UI.

This has the advantage of allowing us to abstract the underlying GitOps technology: if we find that Argo CD is not a good choice for a large scale managed service (for example, is too expensive to scale), or we find the project direction is non-strategic.


### 3) The AppProject concept is baked into the Application CR: while AppProject is one of the main mechanisms for multitenancy in Argo CD, it doesn’t fit well with a KCP-based approach

AppProject works well enough for Argo CD, where all the Argo CD Applications live within a single namespace, and a set of privileged administrators are responsible for configuring Argo CD on behalf of users.

In contrast, with the KCP model, since with KCP it’s so cheap to create virtual clusters (known in KCP as KCP workspaces), applications and users are more likely to get their own dedicated virtual clusters.

Argo CD namespace:

- target cluster 1 (cluster secret)

- target cluster 2

- target cluster 3

- gitops repo 1 (repo secret)

- gitops repo 2

- gitops repo 3

- Application appA: should be read from gitops repo 3 and write to target cluster 1

- Application appB: should be read from gitops repo 2 and write to target cluster 2

- Application appC: should be read from gitops repo 1 and write to target cluster 3

- AppProjects to enforce the above

In a KCP world, this is more likely to look like (with each line being a workspace, or a K8s resource within the workspace):

```
.
└── kcp-root 
    └── applications
        ├── appA (workspace)
        │   ├── appA-gitopsdeployment
        │   ├── git-repository-credential-1
        │   ├── dev (workspace)
        │   ├── prod (workspace)
        │   └── staging (workspace)
        ├── appB (workspace)
        │   ├── appB-gitopsdeployment
        │   ├── git-repository-credential-2
        │   ├── dev (workspace)
        │   ├── prod (workspace) 
        │   └── staging (workspace)
        └── appC (workspace)
           └── (...)
```

e.g. each application is more likely to be segregated into its own virtual workspace, with only the Git credentials required to read that Application’s GitOps repository, and only the target clusters, associated with the GitOpsDeployment.

If a user only has access to the ‘:kcp-root:applications:appA’ workspace, they have no way to access the repository credentials for any of the other workspaces. Likewise, the GitOpsDeployment would only be able to deploy to child workspaces.

(Note: the specific pattern above is not prescribed anywhere: users are free to use GitOps Service and KCP together as they wish, with whatever workspace configuration they want. However I suspect a pattern like this would be a good initial best practice)

There may still be a use for an AppProject-like CR, but this is less useful outside the standard ‘All Argo CD Applications in a namespace’ model of Argo CD, or for the global-cluster scope of repository credentials/cluster credentials.


### 4) If we expose the Argo CD API, but don’t support the Argo CD Application full feature set, this provides a bad API UX to users: they will see that we support the Argo CD Application CR and expect all of the .spec and .status fields to ‘just work’.

If a user sees that we support the Argo CD Application resource, they are going to assume that all of the fields of that Application CR are supported by the service. 

- They attempt to reference an AppProject

- They attempt to specify a user-provided plugin

- They attempt to use sync windows feature

- (etc)

There are a number of fields that the Argo CD Application CR includes, but are non-trivial for us to support in a managed service:

.spec.project / AppProjects in general:

- .spec.project makes sense when you have a single Argo CD namespace that contains all the Application CRs.

- In our case, we no longer have that restriction.

- We have no corresponding concept of AppProject, and it really doesn’t make sense in a multitenant environment.

.spec.source.plugin:

- We don’t have support for user-provided plugins.

.revisionHistoryLimit:

- Argo CD’s only preserving the previous X revisions has historically hampered us from implementing a longer-term record of deployments.

syncWindows

.status:

- Many of the .status fields

This forces us to have to tell users that while we support the Argo CD Application API, we don’t support all of the fields, which is a bad experience. In contrast, by using our own API, we can only expose these in our API once they become ready.


### 5) Opinion: The Argo CD API design is out of step with Red Hat/OpenShift’s traditional API strategy: Red Hat has preferred to use CustomResources for configuration data (e.g. operands for operators), whereas the Argo CD has thus far preferred opaque Secrets/ConfigMaps

Or, said another way, Argo CD’s API is ergonomically challenging in many cases: opaque secrets/configmaps vs CRs, and this makes this part of the API difficult to consume. 

An example of all various opaque Secret/ConfigMaps they use:

- <https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#atomic-configuration>

- <https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters>

- <https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories>

compare:

```yaml
apiVersion: v1
kind: Secret
metadata:
  annotations:
    managed-by: argocd.argoproj.io
  labels:
    argocd.argoproj.io/secret-type: cluster
  name: cluster-api.ci-ln-c086nhk-72292.origin-ci-int-gce.dev.rhcloud.com-3625407592
  namespace: argocd
data:
  # yes, this is a .json field in YAML
  config: |
    {
    "bearerToken": "(long string)",
    "tlsClientConfig": {
      "insecure": true
    }
  } 
  name: default/api-ci-ln-c086nhk-72292-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
  server:  https://api.ci-ln-c086nhk-72292.origin-ci-int-gce.dev.rhcloud.com:6443
type: Opaque
```

to

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  name: my-managed-environment
spec:
  apiURL: "https://api.ci-ln-c086nhk-72292.origin-ci-int-gce.dev.rhcloud.com:6443"
  allowInsecureHost: true

  credentialsSecret: "my-managed-environment-secret" # bearer token in field of Secret
```


For instance:

- Repository secrets are an opaque Secret

- Cluster secrets are an opaque Secret

- Configuration secrets are opaque Secrets

In contrast:

- GitOps Service Repository credentials are a CR

- GitOps Service Cluster Secrets are a CR

Why has this API been historically preferred this? Probably because users just use the Argo CD CLI to generate these ConfigMaps/Secrets (‘argocd cluster add’), which OpenShift GitOps has not historically supported IIRC, and likewise with the GitOps Service.


### 6) Not specific to the Application CR, but there's no way to trigger manual sync of an Argo CD Application without either using the CLI (not supported) or the Web UI (not supported).

So for the GitOps Service, we’ve introduced a new CR for this(GitOpsDeploymentSyncRun).


## So what do I recommend?

Well the strategy we have used in this GitOps Service to date is to use our own resource group/version/kind (**GitOpsDeployment** in[ managed-gitops.redhat.com/v1alpha](http://managed-gitops.redhat.com/v1alpha)), BUT ensuring we still maintain close API compatibility with the Argo CD Application CR. 

This way we get the best of both worlds:

- Users can take their existing Argo CD Applications, replace the ‘kind:’ and ‘apiVersion:’ with ours, and in most cases be good to go.

  - Existing knowledge of Argo CD API should allow them to get up and running quickly with our API

  - If they use features we don’t support, their API will fail to validate, immediately informing them what they need to change.

  - Besides Application CR, the other Argo CD APIs also have a corresponding GitOpsService equivalents: 

    - Repository Secret -> GitOpsDeploymentRepositoryCredentials, 

    - Cluster Secret -> GitOpsDeploymentManagedEnvironment.

    - As above, unlike Argo CD, we use CRs instead of opaque Secrets

- With the added advantage that we can add our own fields to this API, without breaking the existing Argo CD API guarantees

- If/when we implement a new Argo CD Application feature in the GitOps Service, we can add the corresponding API to the GitopsDeployment resource (e.g. gradually expose Argo CD functionality, over time).

  - For example, once we support sync windows, we could add that to the GitOpsDeployment API: but we wouldn’t add it to the GitOpsDeployment API until we support it.
