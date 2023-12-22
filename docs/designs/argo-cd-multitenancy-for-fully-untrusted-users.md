# Argo CD Multitenancy for fully untrusted users of GitOps Service<a id="argo-cd-multitenancy-for-fully-untrusted-users-of-gitops-service"></a>

### Written by
- Jonathan West (@jgwest)
- Originally written May 9th, 2022


## Table of Contents<a id="table-of-contents"></a>


## Introduction<a id="introduction"></a>

As [described here](initial-api-design.md), Argo CD is not necessarily built for multitenancy fully untrusted users, rather it has historically been focused on ‘partially trusted users’:

- **Partially trusted users**: 

  - Employees within an enterprise, whose identities are fully known to the enterprise

  - Serious consequences for abusing system resources (employment termination, or criminal charges in the worst cases).

- **Fully untrusted users**: 

  - In contrast, public cloud products (such as ours) operate in a very different context.

  - We don’t know our users: we only ask them for an email address (and unvalidated PI, which we do not check): we don’t (for example) ask them for credit card or validated phone number

  - The only consequence for abusing system resources is our terminating their account.

The contextual difference between these two groups of users means that attacks are far more likely from one group, than the other.


## Why Argo CD multitenancy?<a id="why-argo-cd-multitenancy"></a>

Since, I suspect, most users of the GitOps Service via AppStudio (at least initially) will have only a small number of deployments, it does not make sense (from a resource perspective) to dedicate to them a full Argo CD instance.

Likewise, since Argo CD mostly just sits around idling (low CPU utilization), receiving K8s watch events and occasionally checking Git, we can drive better resource utilization via shared resources between users.

Thus, if there was a way to securely share Argo CD instances between multiple users, this would be the ideal.

For reference, an OpenShift GitOps operator installed onto an empty clusterbot cluster consumes these resources:

- Argo CD (server+appset+repo+app controller+redis): 119MB

- OpenShift GitOps Misc (kam/cluster/dex) 99MB

OTOH, AWS EC2 is roughly 120GB of memory per dollar per hour.


## Multitenancy: possible solutions<a id="multitenancy-possible-solutions"></a>

### Option A: Argo CD instances shared between multiple users<a id="option-a-argo-cd-instances-shared-between-multiple-users"></a>

The best resource utilization (e.g. maximizing the number of users per dollar of RH infrastructure costs) would be achieved with multiple users (securely) sharing Argo CD instances. 


### Option B: Dedicated Argo CD instances per user<a id="option-b-dedicated-argo-cd-instances-per-user"></a>

One option for providing Argo CD to fully untrusted users is just to give each user of the GitOps Service their own Argo CD instance. This way, in the worst case scenario, they can only corrupt or DOS their own Argo CD instance.

Based on the above numbers, there would be a base overhead of about \~120MB per user.


### Option C: Hybrid approach - shared Argo CD instances with dedicated components (for example, a repo server per user)<a id="option-c-hybrid-approach---shared-argo-cd-instances-with-dedicated-components-for-example-a-repo-server-per-user"></a>

Another approach would be to share the Argo CD control plane and application controller components of Argo CD between users, but not the repository server.

This would be one approach for mitigating malicious Git repositories, for example, but would not mitigate the per-user cost of a single Argo CD instance.


## Current challenge with shared multitenancy: Repository credentials<a id="current-challenge-with-shared-multitenancy-repository-credentials"></a>

### Sketch of Problem<a id="sketch-of-problem"></a>


**1) User 1 adds repo credentials for a private repo:** http\://github.com/private-org/private-project.git

**2) User 1 creates an Argo CD Application to deploy from that repo**

- bound to an AppProject that allows them to access that repo

**3) User 2 adds their own repo credentials for a private repo:** http\://github.com/private-org/private-project.git (same repo url as from step 1, but different credentials)

**4) User 2 creates an Argo CD Application to deploy from that repo**

- bound to an AppProject that allows them to access that repo

_The problem is - what if user 2's Git credentials expire? Expected behaviour: Argo CD should no longer deploy the contents of that Git repo for them._

**5) \*However\*, this is not the actual behaviour:** Argo CD will continue to deploy the latest git repo changes via user 2's Application (e.g. to their namespace), even though their credentials are now invalid.

You might say: well of course, because the AppProject still allows them to do so, which is true! But, the problem is there is no existing mechanism to invalidate a user's access to an AppProject if the repo credentials they provided are no longer valid.


**Ideal behaviour:**

1\) User provided some repo credentials to create an AppProject/Application

2\) Those credentials are associated to an AppProject or Application (e.g. in the same way that an Application is associated with a particular a cluster secret, via the ‘target’ field of Argo CD Application)

3\) As soon as those credentials are invalid, the Argo CD Application will no longer deploy.


Instead, the best alternative I can think of using existing Argo CD mechanisms:

1\) Poll (every X minutes) the set of repo credentials each user has provided, and if they fail, remove the repository from the AppProject of their Application.

- The problem with this is there is now a significant lag between when the credentials expire, and when access is revoked.

- It may also not scale to a large number of users (too much polling)


### Actual GitOps Service Example<a id="actual-gitops-service-example"></a>

Here’s how this same example looks using the API of the GitOps Service.

**User A: User A wants to deploy a private repo to their organization.**

GitOpsDeployment (equivalent to Argo CD Application):


```yaml
apiVersion: v1alpha1
kind: GitOpsDeployment
metadata:
  name: my-deployment
spec:

  # GitHub repository containing K8s resources to deploy
  source:
    repository: https://github.com/private-org/private-repo
    path: .
    revision: master
 
  # Target K8s repository
  destination: { } # (...)

  type: automated # Manual or automated, a placeholder equivalent to Argo CD syncOptions.automated field
```


GitOpsDeploymentRepositoryCredentials:
```yaml
apiVersion: v1alpha1
kind: GitOpsDeploymentRepositoryCredentials
metadata:
  Name: private-repo-creds
spec:
  url: https://github.com/private-org/private-repo
  secret: private-repo-creds-secret
```

Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret
stringData:
  url: https://github.com/private-org/private-repo
  # and:
  username: userA
  password: (my password)
```


**User B:**

User B (in their own namespace) creates the same GitOps Service API resources as user A, but with user B specifying their own GitHub credentials in the secret.


```yaml
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret
stringData:
  url: https://github.com/private-org/private-repo
  # and:
  username: userB
  password: (my password)
```


**The GitOps service turns those GitOpsDeployments/GitOpsRepositoryCredentials/Secrets into the corresponding Argo CD Application/Repository Secret/AppProjects**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: user-A-repo-creds
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: git
  url: https://github.com/private-org/private-repo

  username: userA
  password: (password)
```


```yaml
apiVersion: v1
kind: Secret
metadata:
  name: user-B-repo-creds
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: git
  url: https://github.com/private-org/private-repo

  username: userB
  password: (password)
```

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: userA-Application
  namespace: argocd
spec:
  project: user-A-project-that-allows-access-to-private-repo
  source:
    repoURL: https://github.com/private-org/private-repo
    targetRevision: HEAD
    path: .
  destination: {} # ( .. user A's cluster .. )
```

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: userB-Application
  namespace: argocd
spec:
  project: user-B-project-that-allows-access-to-private-repo
  source:
	repoURL: https://github.com/private-org/private-repo
	targetRevision: HEAD
	path: .
  destination: {} # ( .. user B's cluster .. )
```

**However, we can see where this now breaks down:**

If user B’s GitHub credentials to the private organization (‘private-org’) are revoked, Argo CD will continue to successfully deploy, because there also exist credentials for another user for that repository within the workspace.


### Sketch of similar problem<a id="sketch-of-similar-problem"></a>

1\) **User 1** adds repo credentials for a private repo: http\://github.com/private-org/private-project.git

2\) **User 1** creates an Argo CD Application to deploy from that repo

- bound to an AppProject A that allows them to access that repo

3\) **User 2** creates an Argo CD Application to deploy from that repo

- bound to an AppProject B that allows them to access that repo

Argo CD allows user 2 to access the repo, because the credentials for it are defined by user 1.

Why? As long as a user is allowed to access a repository via an AppProject, Argo CD will use whichever credential it can to connect to that repository.

Could we prevent this by having the GitOps Service require credentials for private repos? Sure, but that’s just solution D below. It’s not an Argo CD native, and will only guarantee eventual consistency of a repository access (e.g. if you make a public repo private, we will eventually restrict other users from being able to access it: once our poll detects the change and updates the appprojects).



### Possible solutions to the problem of shared credentials<a id="possible-solutions-to-the-problem-of-shared-credentials"></a>

1. Dedicated Argo CD Instance per user

2. Dedicated repo server per user

3. Upstream support for restricting a particular repository secret credential to a particular AppProject

4. Downstream solution: the Core GitOps Service could periodically poll every user’s repository credential, and if we detected that a user’s credential no longer worked for the repository, we would remove that user’s repository from that user’s AppProject 


## Current challenge with shared multitenancy: malicious Git repositories<a id="current-challenge-with-shared-multitenancy-malicious-git-repositories"></a>

Malicious Git repositories can be constructed, with two aims: denial of service or credential theft.

Denial of service:

- Create a GitOps repository which is several GB in size, requiring Argo CD to spend downstream bandwidth on transferring it

- Create a GitOps repository contains a large number of files, causing an OOM on repo-server Pod when it attempts to process them all (e.g. via Kustomize)

Credential theft/privileged information disclosure:

- Example: [CVE-2022-24731](https://github.com/argoproj/argo-cd/security/advisories/GHSA-h6h5-6fmq-rh28)

- Theoretical ability to trigger a malicious workload via kustomize/helm



Potential mitigations for these issues:

- My preference - run kustomize/helm/etc in disposable, fully untrusted K8s Job: perform all interactions with the Git repository, and invocations of kustomize/helm/etc, within a disposable, untrusted, K8s Job resource.

  - Malicious workloads will be confined to within the Job (to the extent possible)

  - Job is disposed of on job completion.

  - No possibility for leakage of data between users (since all data is self-contained to the container and volume)

  - If there is a concern with the need to do a git clone every time: You can also use a persistent volume per user (shared into the Job as a volume)

- Alternative solution: a repo server per user
