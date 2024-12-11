# GitOpsDeploymentManagedEnvironment API: ‘cluster add’ behaviour

### Written by
- Jonathan West (@jgwest)
- Originally written in December 7, 2022

New AppStudio requirements (primarily driven by post-KCP shift) are requiring us to rethink the behaviour of one of the Core GitOps Service APIs, **GitOpsDeploymentManagedEnvironment**.

Primarily this is about how to add new clusters (as deployment targets) to the GitOps Service. When a user gives us credentials for the environment, should we either:

* **A)** Create a new ServiceAccount on the cluster using the given credentials (e.g. copy the behaviour of the 'argocd cluster add' CLI command)  
* Or, **B)** Use the credentials as is: give the provided API URL/token credential directly to Argo CD. 

## Final decision

The final outcome of this document, as of March 7th, 2023, was to follow option A: supporting both the old and new behaviour.

Which behaviour to use would be designed with the proposed field ‘ createNewServiceAccount=true/false’, as seen under option A below.

## Current behaviour

**TL;DR**: The GitOps Service follows the same behaviour as the '*argocd cluster add*' CLI command: it creates a new cluster-scoped ServiceAccount on the target cluster using the credentials provided by the user.

When the user wants to deploy to a target environment, they give us a ManagedEnvironment and Secret for that Environment, like so:

**ManagedEnv:**

```yaml
kind: GitOpsDeploymentManagedEnvironment
spec:
  apiURL: (api url containing credentials to use for deployment)
  credentialsSecret: (secret containing credentials to use for deployment; these will be provided as is to Argo CD)
```

**Secret:**


``` yaml
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
        token: sha256~ABCdEF(...) # fake secret, obviously
```

When a GitOpsDeploymentManagedEnvironment/Secret are created:

1. The GitOps Service creates a new **ServiceAccount/Secret** on the target cluster  
   * Using the credentials the user provided, but those are only used initially.  
2. It then creates a '\* \* \*' **ClusterRole/ClusterRoleBinding** for that ServiceAccount.  
3. Next, the Service **stores the credentials** for that new service account:   
   * the API URL, and the service account token.   
   * Together, these will be used by and allow Argo CD to login to the cluster via this ServiceAccount.

The GitOps Service thus ONLY uses the user-provided credentials (those provided in the Secret) for the initialization of the ManagedEnvironment/Secret. From that point on, it uses a ServiceAccount on the target cluster created by the GitOps Service.

This current behaviour makes sense when the user is using a Kubernetes *User* account to create the cluster credentials (which will be based on a user K8s session that will expire), but makes less sense when the user is giving us the credentials for an existing service account (which should not expire).

I believe the first use case (credentials based on a Kubernetes User account) is why the ‘argocd cluster add’ command has the behaviour that it does, and we followed this behaviour here.

## Problems with the current behaviour

The current behaviour doesn't work well when the user already has a ServiceAccount setup on a cluster, which they wish Argo CD to use.

This is especially true when the ServiceAccount they wish Argo CD to use is not cluster-scoped: for example, only being able to deploy to a fixed number of namespaces.

* The ServiceAccount, for example, might use Roles/RoleBindings on a small set of Namespaces, rather than a single \* \* \* ClusterRole/Binding

It's this exact case \-- wanting to use an existing ServiceAccount \-- that is driving the change described here.

One driver of this requirement is StoneSoup: Since we've dropped support for KCP, StoneSoup has been exploring alternative mechanisms for supporting multiple users on a single cluster.

* This includes the ability to allow untrusted users to deploy to multiple namespaces on a shared cluster.  
* For example:  
  * 'jgwest-appA', 'jgwest-appB'. Or, 'jgwest-appA-staging', 'jgwest-appB-prod'.  
  * And on the same cluster, there might also be 'wtam-appA', 'wtam-appB', and so on.

The current proposed mechanism to achieve this is SpaceRequests and new AppStudio Environment APIs, which are outside the scope of this document.

In any case, what the GitOps Service will get out of this process is an existing credentials/API URL for ServiceAccount. The ServiceAccount will NOT have \* \* \* ClusterRole/RoleBinding, and thus the existing behaviour will fail: the logic will attempt to create a CR/CRB on the cluster, and not be able to, due to lack of permissions.

## New proposed behaviour

**TL;DR**: We use the cluster credential exactly as provided by the user.

When a GitOpsDeploymentManagedEnvironment/Secret is created:

1) We test that the credentials work: that we are able to connect to the target cluster using that API URL/token  
2) We set up Argo CD to use those: the API URL, and the service account token.   
3) (Unlike with the original behaviour, we do not create a new ServiceAccount on the target cluster)

Together, these will allow Argo CD to login to the cluster via the user-provided credentials.

## Options

So the main open question is whether to support both options, or support only the new behaviour.

**A) Support both old (argocd-cli-like) and new (serviceaccount-centric) behaviour**

* Requires an API change: user must indicate which behaviour they want.  
* This option is my preference, as it allows us support both use cases: a user can specified whether they want us to create a new ServiceAccount based on the provided credentials, or just use what they have provided directly

**B) Support only new behaviour**

* Does not require an API change: we just switch the code/tests to use the new behaviour.

**C) ~~Support only the old behaviour~~**

* I think this is untenable, due to the problem identified above.

### And, if we go with option A) \- what API do we want

If we go with Option A), we will need to make an API change to allow the user (the user is the creator/consumer of the **GitOpsDeploymentManagedEnvironnment** CR) to specify which behaviour they want.

```yaml
kind: GitOpsDeploymentManagedEnvironment
spec:
  apiURL: (api url containing credentials to use for deployment)
  credentialsSecret: (secret containing credentials to use for deployment; these will be provided as is to Argo CD)
  allowInsecureSkipTLSVerify: (...)

  createNewServiceAccount: true/false # new field: false is default. if true, will follow option A behaviour. If false, option B.
```


   		   
Ideas for new field names:

* **createNewServiceAccount** (descriptive, but it isn't very declarative)  
* **createNewDeploymentServiceAccount** (more descriptive, getting too long, not very declarative)  
* **serviceManagedServiceAccount** (whether or not the GitOps Service should create and manage a new ServiceAccount on the cluster, description but confusing terminology )  
* **managedServiceAccount** (managed term is overloaded, and thus not especially descriptive here)  
* **shouldWeCreateAndManageAServiceAccountOnTheClusterInsteadOfUsingTheCredentialsYouGiveUs** 😛  
* **useExistingSecret** or **useExistingServiceAccount**  
* **importServiceAccount**  
* others?

**Another option that was proposed in cabal, was a user specifying the particular SA to use:**

```yaml
kind: GitOpsDeploymentManagedEnvironment
spec:
  apiURL: (api url containing credentials to use for deployment)
  credentialsSecret: (secret containing credentials to use for deployment; these will be provided as is to Argo CD)
  allowInsecureSkipTLSVerify: (...)

  # option c), this new field:
  remoteClusterServiceAccount:
    name: my-service-account
    namespace: jgw
```

I like the idea of this, but there are a few issues with this option:

* The cluster credentials provided by the user in credentialSecret might be different from the ones provided in remoteClusterServiceAccount  
  * Which is fine, but there’s no reason for them to be different. This is a configuration option without a purpose.  
* If this field is not specified, a serviceaccount will automatically be created. This violates the principle of least surprise.

