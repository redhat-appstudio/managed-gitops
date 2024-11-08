# Private repository support for a fixed set of private, GitHub organizations

### Written by
- Jonathan West (@jgwest)
- Originally written in September 26, 2023

At present, within AppStudio, all users' GitOps repositories are created within the [https://github.com/redhat-appstudio-appdata](https://github.com/redhat-appstudio-appdata) organization. The Application Service (HAS) component of AppStudio has the credentials for this repo, and uses the GitHub REST API to create/delete these repositories, and the Git API to push to them.

As of this writing, those 'redhat-appstudio-appdata' repositories are all public, but this is primarily because GitOps Service did not support private Git repositories in the early stages of the AppStudio project (and we have not been asked to add AppStudio private repository support since).

As part of [RHTAP-1023](https://issues.redhat.com/browse/RHTAP-1023), however, Git repositories may now be private, in order to support embargoed content. We in GitOps Service thus need to ensure that GitOps Service configures Argo CD to pull from Appstudio-managed private Git repositories.

Unlike [GITOPSRVCE-28](https://issues.redhat.com/browse/GITOPSRVCE-28), which allows users to provide credentials for their own private repos, instead, this Epic is limited to provide a single, global pool of tokens to be used for organization-managed GitOps Repos, such as [https://github.com/redhat-appstudio-appdata](https://github.com/redhat-appstudio-appdata)

* AFAIK this org is the ONLY org we need to provide private repository support for, at this time.

This point means this epic is much more limited in scope versus the more open, user-focused, GITOPSRVCE-28.

# Out of Scope

As above, with this epic, there are no changes to the following behaviours of AppStudio:

* Users will not be able to provide their own private repository credentials.  
* Users will not be able to provide their own GitOps repository URL  
* Users cannot customize their GitOps repository (beyond the ability to provide a custom devfile)

# Proposed Workflow

**1\) AppStudio maintains a list of GitHub API tokens (personal access tokens, PATs), either shared, or per team**

* Ideally we would be able to share HAS’ token pool, which would obviate the need for GitOps Service to maintain their own token pool.  
  * BUT, this requires shared consensus between the teams.   
* The actual shared list of tokens is stored in app-sre’s Hashicorp vault instance  
* An [External Secrets resource](https://github.com/redhat-appstudio/infra-deployments/blob/main/components/has/base/external-secrets/has-github-token.yaml) reads the secret from Hashicorp vault, and writes it to a Secret in 'gitops' Namespace.  
  * Hashicorp \-\> External Secrets is the standard AppStudio mechanism for this  
* See below for the format of the Secret (based on HAS' team format)

**2\) The 'cluster-agent' component of GitOps Service should be the only component that we need to provide access to this token pool**

* Add env var(s) to cluster-agent, referencing the token list Secret in the Namespace.  
* That would look like this:

```yaml
# cluster-agent's Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
spec:
  template:
    spec:
      containers:
      - command:
        - gitops-service-cluster-agent
        # (...)
        name: manager
        env:
        # A group of Secrets for each org containing private repos: But, I currently expect  we'll need only one, for 'http://github.com/redhat-appstudio-appdata'
        # TOKEN_POOL_1_*
        - name: TOKEN_POOL_1_ORG_URL
          value: "http://github.com/redhat-appstudio-appdata"
        - name: TOKEN_POOL_1_SECRET
          value: "token-pool-tokens" # reference to Secret in Namespace

        # (...)
        # TOKEN_POOL_N_*
        - name: TOKEN_POOL_N_ORG_URL
          value: "(...)"
        - name: TOKEN_POOL_1_SECRET
          value: "(...)"

---

# Token Pool Secret (actual contents coming from Hashicorp vault via External Secrets)

kind: Secret
metadata:
  name: token-pool-tokens
data:
  # Secret format from HAS
  tokens: "token1:(...),token2:(...),tokenN:(...),token7:(...)"    
```

**3\) In cluster-agent, whenever cluster-agent creates/modifies an Argo CD Application CR (via an Application Operation), AND the '.spec.source' field of the Argo CD Application CR matches one of the TOKEN\_POOL\_X\_ORG\_URLs defined in the environment variables, we should do the following:**

Create (ensure there exists) a [Repository Credential Secret](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repository-credentials) for that repository:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: "repo-cred-(sha-256 hash of repo url)"
  namespace: gitops-service-argocd
  labels:
    argocd.argoproj.io/secret-type: repo-creds

stringData:
  type: git
  url: "http://github.com/redhat-appstudio-appdata/(repo URL value from .spec.source field of Argo CD Application)"
  password: "(token from token pool, chosen using below algorithm)"
  username: username
```

**4\) What value should we use for "(token from token pool)", in the previous step? Well, we can use the following algorithm to determine which token to use**

Pseudocode:

```go

githubTokenListFromSecret := { /* read from has-github-token Secret */ }

// hash the URL
hashedValue := sha256.Sum256(gitRepositoryURL)

// use the first byte of the hashed value to index into token list
secretIndex := hashedValue[0]  %  len(githubTokenListFromSecret)

repositoryTokenValToUse := githubTokenListFromSecret[secretIndex]
```

**TL;DR**: hash the git URL and use that to index into the token list, to ensure an even distribution between tokens.

# Alternatives Considered

**Why not just define a GitOpsRepositoryCredential CR in each Namespace, containing the credentials for the repo URL?**

* GitOpsDeploymentRepositoryCredential works great for cases where users have their own private Git Repository, and their own private credentials  
* However, in this case, the user does not have the credentials for the private GitOps repository (these are only known by Red Hat)  
* With GitOpsDeploymentRepositoryCredential, the token is stored in a Secret in the user’s ‘(username)-tenant’ namespace  
* In AppStudio, users can view Secrets in their own Namespace  
* Thus, the GitHub token PAT that we use to communicate with the repo would necessarily be viewable to the user, with this approach  
* Thus, the only way this would work would be if we generated a PAT token PER USER, which would be excessive

**Rather than defining an Argo CD Repository Secret for each repository, why not define a single Argo CD Repository Secret to be shared by all the repos?**

* Since we have a large number of users on AppStudio, we want to ensure that we do not overuse a single PAT token for all our Git requests, but rather we distribute that work over multiple accounts (tokens).  
  * On multi-tenant prod, 341 Argo CD Applications (and roughly the same number of Git repos)  
* This allows us to evenly distribute (via SHA-256 hashes indexing into token lists) the work across all available tokens

