# Exploring Generic GitOps Service Architecture

### Written by
- Jonathan West (@jgwest)
- Originally written October 27, 2021

## Need for a standalone GitOps Service component

In our architecture, we need a component that will:

* Support an Argo-CD like API (but not the Argo CD API itself, see ‘Issues with storing ‘Argo CD Application CR...’ below)  
* Detect health issues with Argo CD instance(s) and remediate  
* Scale up the resources/number of Argo CD instances based on load (load would be  based on number of KCP workspaces, number of applications)  
* Configure Argo CD to watch all those KCP namespaces (add cluster credentials for KCP workspaces)  
  * As new KCP workspaces are added/removed, add/remove the cluster credential secret to/from Argo CD  
* Configure Argo CD with private repository credentials  
* Define Argo CD Application CR corresponding to KCP workspaces (target) and GitOps repositories (source)  
  * Will need to translate from Argo-CD-like API 

## Argo CD API vs Argo CD-like API

Why not use the Argo CD API?

* We don't want to expose Argo CD API/features that we don't support (mainly ‘AppProject’s; we have our own multitenancy model)  
* Unergonomic API: the 'cluster' field of Argo CD Application references a non-virtual ('physical') cluster defined in Argo CD (eg: jgwest-staging).  
  * For KCP, the target cluster will often be the KCP environment itself  
  * But we also need to support external non-virtual clusters (those not managed by KCP)  
* No way to trigger a sync using K8s CR API (at this time)  
* Security issues with exposing Argo CD on KCP workspace (the same security issues with giving non-admin users ability to create Applications in argocd namespace in Argo CD proper)  
* We don't want to necessarily tie ourselves to Argo CD model (abstraction over gitops, allowing ability to swap out other gitops engines if needed)

See proposal for Argo CD-like API below.

## Argo CD API: Issues with storing Argo CD Application CR directly on KCP control plane 

**The biggest issue**: Argo CD only supports Application CRs from within the ‘argocd’ namespace (e.g. from within the same namespace as Argo CD is installed). Argo CD does not handle Application CRs from remote clusters (or other namespaces). (This is mentioned in, but a non-goal of, argo-cd [\#6409](https://github.com/argoproj/argo-cd/pull/6409)/[6537](https://github.com/argoproj/argo-cd/pull/6537/files) \- Applications outside Argo CD namespace)

**Security issues with allowing users to define Application CR (they can change the project, target cluster, and target namespace at will)**

Argo CD’s current security model is that if a user can create Application CRs within the Argo CD namespace, then they have full control over that Argo CD instance. This is because there are no security checks at the CR level: it is assumed that if you can create an Argo CD CR, then you have full admin privileges.

Unsafe Argo CD Application fields (“for privileged users only”):

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: guestbook

  # Allowing users to customize this field would prevent the Argo CD controller from detecting the Application CR
  namespace: argocd

spec:
  # Allowing users to customize this field would allow them to break out of the RBAC sandbox
  project: default

  source:
    # Allowing users to customize this field would allow them to deploy other user’s private Git repositories (there is no RBAC checking of private repos for CRS)
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: guestbook

  # Destination cluster and namespace to deploy the application
  destination:
    # Allowing users to customize these fields would allow them to target clusters/namespaces that they should not be able to:
    server: https://kubernetes.default.svc
    name: # as above
    namespace: guestbook
```


Thus, rather than storing Argo CD Application CR directly on the KCP control plane, it makes more sense to store an Argo-CD-Like API on the KCP control plane.

## GitOpsDeployment CR API

An Argo-CD-like API, but works on KCP. Contrast with the [Application CR](https://raw.githubusercontent.com/argoproj/argo-cd/4a4b43f1d204236d1c9392f6076b292378bfe8a3/docs/operator-manual/application.yaml). A light abstraction over Argo CD.

**GitOpsDeployment CR:**

Create this CR to enable synchronization between a Git repository, and a KCP workspace:

```yaml
apiVersion: v1alpha1
kind: GitOpsDeployment
metadata:
  name: jgwest-app
spec:

  # Note: no ‘project:’ field, multi-tenancy is instead via KCP

  source:
    repository: https://github.com/jgwest/app
    path: /
    revision: master
  
  destination:
    namespace: my-namespace
    managedEnvironment: some-non-kcp-managed-cluster # ref to a managed environment. optional: if not specified, defaults to same KCP workspace as CR 

  type: manual # Manual or automated, a placeholder equivalent to Argo CD syncOptions.automated field

status:
  conditions:
   - (...) # status of deployment (health/sync status)
```

**GitOpsDeploymentSyncRun**:

Create this to trigger a manual sync (if automated sync is not enabled above):

```yaml
apiVersion: v1alpha1
kind: GitOpsDeploymentSyncRun
spec:
  deploymentName: jgwest-app
  revisionId: (...)   # commit hash 
status:
  conditions:
    - "(...)" # status of sync operation
```

```yaml
apiVersion: v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  Name: some-non-kcp-managed-cluster
spec:
  clusterCredentials: (...)
```


## How to scale Argo CD: single-instance vs multi-instance

**Option 1\) GitOps Service: Argo CD *multiple*\-instance model (*may also support multiple-controller-replicas*):**

*Description*: Multiple instances of Argo CD, managed by GitOps Service. Number of instances can be scaled up/down as needed based on demand (number of managed clusters, number of applications).

*Advantages:*

* Does not require upstream Argo CD changes  
* Allows us to do partial rollouts of new Argo CD versions  
* Will necessarily scale better versus a solution that does use instances

*Disadvantages:*

* Spinning up a new Argo CD instance is slightly more difficult (create a new namespace with Argo CD, rather than just increasing replicas)  
  * Note: For MVP I am assuming we will have a single, large, shared Argo CD instance  
* More difficult to babysit multiple Argo CD instances (with fewer replicas), than a single Argo CD instance with a bunch of replicas  
* Less of our code for implementing this is in upstream Argo CD

Work Required:

* Implement logic which translates individual KCP workspaces  to the corresponding Argo CD sharded instances

**Option 2\) GitOps Service: Argo CD *single*\-instance, multiple-controller-replicas model:**

*Description*: A single instance of Argo CD, with multiple replicas

*Advantages:*

* Slightly less complex to implement: all Argo CD Application CRs are on a single cluster  
* More of our code is in upstream Argo CD (but still a lot that isn’t)

*Disadvantages:*

* Requires extensive upstream Argo CD changes (and upstream has been resistant to  changes in the past)  
  * See below.  
* Risky: not guaranteed to scale, even after making those upstream changes  
  * These are the known bottlenecks: we may encounter additional previously unknown bottlenecks after these initial known bottlenecks are handled  
* Upgrading to new Argo CD versions  
  * Upgrading Argo CD version will cause downtime for all users (all controller replicas must restarted at the same time, as they all share a K8s Deployment resource)  
  * No way to do partial rollouts of new Argo CD versions, for example, to test a new version for a subset of users (all replicas in a deployment must be the same version)  
* Scaling:  
  * The problem of 10,000+ Application CRs in a single Argo CD namespace  
  * No way to dynamically scale up the number of replicas without downtime: current implementation of cluster sharding means that increasing the number of replicas requires restarting all controllers (see sharding.go in Argo CD for the specific logic)  
  * Corollary: Draining a replica requires a restart of Argo CD  
  * A [*ton* of K8s watches are needed](https://gist.github.com/jgwest/572a97aba2e196924a0eb3fddcdee57c) (one for each CR+R, per workspace, eg 48 \* \# of KCP workspaces), potentially saturating I/O bandwidth/CPU/Memory.  
  * Sharding algorithm is simplistic, which limits scaling: it doesn’t help in the case where a single user overwhelms the capacity of a single replica.  
    * No way to rebalance between shards.  
  * No way to scale across multiple clusters  
    * With replicas we can scale across multiple nodes, but not multiple clusters  
    * All controller replicas are limited running on the same cluster  
    * May bottleneck on upstream due to single cluster I/O, even with multiple nodes  
  * Doesn’t work with a multi-geo (or multi-public cloud) KCP: user has different environments running in different geos, and KCP moves between those Geos  
    * The best solution to this problem is to run instances on each cloud.  
    * Otherwise you pay $$$$ for outbound public cloud bandwidth  
* Reliability:  
  * ‘Putting all our eggs in one basket:’ If something takes down the entire Argo CD instance, then it takes down all users, rather than just a subset  
    * [https://github.com/argoproj/argo-cd/issues/7484](https://github.com/argoproj/argo-cd/issues/7484)  
    * [https://github.com/argoproj/argo-cd/issues/5817](https://github.com/argoproj/argo-cd/issues/5817)  
  * Repo service locking logic is complex, which makes me concerned for deadlocks.  
* Security  
  * With an architecture that scales via replicas, it’s more difficult to handle the scenario where a user wants a dedicated Argo CD instance, or where we want our architecture would not support geo-based instances   
* “Politics”  
  * Our momentum is gated on the upstream project  
  * Significant push back on previous major changes 

*Work Required:*

* As a thought experiment, consider that a single instance Argo CD instance (set of 1 or more replicas) might need to handle:  
  * 10,000 target clusters (kcp workspaces)  
    * 10,000 cluster secrets  
  * 10,000 target gitops repository  
    * 10,000 repo secrets  
  * 25,000 applications (avg of 2.5 apps per workspace)  
  * 480,000 active watch API requests (48 \* \# of kcp workspaces)  
* Add ability to watch and respond to Argo CD Application CRs on remote clusters, in non-Argo CD namespaces (part of this is handled by argo-cd [\#6409](https://github.com/argoproj/argo-cd/pull/6409)/[6537](https://github.com/argoproj/argo-cd/pull/6537/files) \- Applications outside Argo CD namespace)  
* Logic which translates KCP workspaces \-\> Argo CD (api server/controller/repo server/redis) replicas  
* Shard repo/cluster settings (Secrets and ConfigMaps in the argo cd namespace)  
* Scale up API controller  
  * Sharding based on cluster doesn’t work when there are just too many Applications in a workspace for one replica to handle  
  * Need to be able to handle large numbers of Application CRs in a single Argo CD namespace (or split up into multiple NS, but then someone needs to manage that split mechanism)  
  * Some mechanism to tag which target repo/ target cluster should be used for each application (can use destination cluster)  
* Shard and scale up repo server  
  * At the moment, all controller replicas share the same repo server.  
* Shard and scale up redis  
  * At the moment, all controller replicas share the same redis server.  
* Shard and scale up API server  
  * At the moment, all controller replicas share the same API server.  
* Identify additional areas in the code that do not scale in the expected manner  
* BUT: A number of items under disadvantages are not solved by this work required, due to the nature of the architecture:  
  * Partial rollouts, multi-cluster, upstream politics

