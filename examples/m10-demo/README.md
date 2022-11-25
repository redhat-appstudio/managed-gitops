# 10 Demo

Get an openshift cluster.

```bash
# Install GitOps Operator + ArgoCD with new keys
$ make install-argocd-openshift

# Get the password for ArgoCD Web UI
$ ARGOCD_SERVER=$(oc -n gitops-service-argocd get routes gitops-service-argocd-server -o jsonpath="{.spec.host}")
$ ARGOCD_PASSWORD=$(oc -n gitops-service-argocd get secret gitops-service-argocd-cluster -o jsonpath="{.data.admin\.password}" | base64 --decode)

# Login with ArgoCD binary (optional, if you use argocd binary to verify)
$ argocd login $ARGOCD_SERVER --username admin --password $ARGOCD_PASSWORD

# Check for the keys
$ oc -n gitops-service-argocd get configmap -o jsonpath='{.data.ssh_known_hosts}'  argocd-ssh-known-hosts-cm | grep 'github.com ssh-ed25519'
$ oc -n gitops-service-argocd get configmap -o jsonpath='{.data.ssh_known_hosts}'  argocd-ssh-known-hosts-cm | grep 'github.com ecdsa-sha2-nistp256'

# Create a key pair
# You can do it in a container if you like
$ docker run --rm -it --name demo fedora /bin/bash
$ dnf install openssh-server -y
$ ssh-keygen -t ed25519
$ cat /root/.ssh/id_ed25519 # Private key
$ cat /root/.ssh/id_ed25519.pub # Public key
$ exit
```

* Go to your GitHub private repo and add the Public key. We will use it for SSH access.
* Also, generate a personal token for your GitHub account as well. We will use it for HTTPS access.

Lastly, start the Gitops Service controllers:

```bash
$ make devenv-docker
$ make start
```

## Leaking credential notice

> Notice: We cannot publish our token and SSH keys to give you access in our private repository that we use for this demo.

If you would like to follow along with this demo, create your own private repo at GitHub or Gitlab (the both are tested and they work fine). There create a folder `resources` and inside create a `configmap.yaml` file: 

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-map-in-private-repo
data: {}
```

For example:

```
https://github.com/YOUR_GITHUB_USERNAME/YOUR_PRIVATEREPO/resources/configmap.yaml
```

## Case 1: Access via HTTPS Token

Use GitOps Service to watch over a directory called `resources` that is located into a private-repository [github.com/managed-gitops-test-data/private-repo-test](https://github.com/managed-gitops-test-data/private-repo-test) and deploy the config-map `config-map-in-private-repo` from it, into your OpenShift cluster, in a namespace called `panos`.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: panos
  labels:
    # If you forget this label, you will end up with an error
    # ComparisonError: Namespace "panos" for ConfigMap "config-map-in-private-repo" is not managed
    argocd.argoproj.io/managed-by: gitops-service-argocd
```

To access this private repository you have to already have access to it as a GitHub user.
That means: either it's already yours (so you have already access) or somebody else (the owner of the private repository) has to add you to the list of GitHub users who have access to it.

As soon as you verify you can acess it, then give access to GitOps Service as well.

To do this, you will use [your personal HTTPS token from GitHub](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token). Given your GitHub user/account already has access there, then your personal token will also have access there.

Create a Kubernetes Secret resource, with your GitHub Username and your password.
But, **instead** of your *password*, type your *token*.

```yaml
apiVersion: v1
kind: Secret
metadata:
  # Remember the name of the secret and the namespace
  # as you need them later for the GitOpsDeploymentRepositoryCredential
  name: private-repo-creds-secret-https-token
  namespace: panos
type: Opaque
stringData:
  # Your GitHub e-mail
  username: PUT_HERE_YOUR_GITHUB_EMAIL
  # Your Token (remember, this is not your GitHub password)
  password: PUT_HERE_YOUR_GITHUB_PERSONAL_TOKEN
```

Then create a `gitopsdeploymentrepositorycredential` CR that associates this Secret with the private repository.

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentRepositoryCredential
metadata:
  name: private-repo-https
  # Put the Secret's namespace
  namespace: panos
spec:
  # Use the HTTPS address of the private repository
  repository: https://github.com/managed-gitops-test-data/private-repo-test.git
  # Put the Secret's name
  secret: private-repo-creds-secret-https-token
```

In the end, create a `gitopsdeployment` CR that points to this private repository,
and looks for manifests inside the directory called `resources`.

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment
metadata:
  name: private-https-depl
  namespace: panos
spec:
  source:
    # It has to be the same HTTPS link
    repoURL: https://github.com/managed-gitops-test-data/private-repo-test.git
    path: resources
  type: automated
```
 
In one YAML, all this would be:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: panos
  labels:
    argocd.argoproj.io/managed-by: gitops-service-argocd

---
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret-https-token
  namespace: panos
type: Opaque
stringData:
  username: PUT_HERE_YOUR_GITHUB_EMAIL
  password: PUT_HERE_YOUR_GITHUB_PERSONAL_TOKEN

---
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentRepositoryCredential
metadata:
  name: private-repo-https
  namespace: panos
spec:
  repository: https://github.com/managed-gitops-test-data/private-repo-test.git
  secret: private-repo-creds-secret-https-token

---
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment
metadata:
  name: private-https-depl
  namespace: panos
spec:
  source:
    repoURL: https://github.com/managed-gitops-test-data/private-repo-test.git
    path: resources
  type: automated
```

If everything went well, this `configmap` has to exist:

```bash
oc -n panos get cm config-map-in-private-repo
```

You can also see the ArgoCD secret that has been created behind the scenes.
It has the same name with your secret but instead of living into `panos` namespace,
it exists into `gitops-service-argocd`:

```bash
$ oc -n gitops-service-argocd get secret private-repo-creds-secret-https-token -o yaml
```

Notice the `argocd.argoproj.io/secret-type: repository` and the values are base64.

You can also verify the Health of ArgoCD via looking either at ArgoCD Dashboard or CLI:

```bash
# Check the private repo access
$ argocd repo list --grpc-web --refresh hard

TYPE  NAME                                   REPO                                                               INSECURE  OCI    LFS    CREDS  STATUS      MESSAGE  PROJECT
git   private-repo-creds-secret-https-token  https://github.com/managed-gitops-test-data/private-repo-test.git  false     false  false  true   Successful


# Check the app that deploys from this private repo
$ argocd app list --grpc-web 

NAME                                                                   CLUSTER     NAMESPACE  PROJECT  STATUS  HEALTH   SYNCPOLICY  CONDITIONS  REPO                                                               PATH       TARGET
gitops-service-argocd/gitopsdepl-c9ec8a71-0580-4662-9070-fbb9461d5eb1  in-cluster  panos      default  Synced  Healthy  Auto-Prune  <none>      https://github.com/managed-gitops-test-data/private-repo-test.git  resources
```

Same verification can be obtained from ArgoCD UI:

![](https://i.imgur.com/HrAdh2U.png)

![](https://i.imgur.com/V1NeGyY.png)

## Case 2: Access via SSH Key

Do the same thing, but this time deploy into another namespace, called `drpaneas` and use an SSH key that is specific to this private repository (instead of your personal GitHub account).

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: drpaneas
  labels:
    argocd.argoproj.io/managed-by: gitops-service-argocd

---
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret-ssh
  namespace: drpaneas
type: Opaque
stringData:
  sshPrivateKey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    PUT
    HERE
    YOUR
    KEY
    -----END OPENSSH PRIVATE KEY-----

---
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeploymentRepositoryCredential
metadata:
  name: private-repo-ssh
  namespace: drpaneas
spec:
  repository: git@github.com:managed-gitops-test-data/private-repo-test.git
  secret: private-repo-creds-secret-ssh

---
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment
metadata:
  name: private-ssh-depl
  namespace: drpaneas
spec:
  source:
    repoURL: git@github.com:managed-gitops-test-data/private-repo-test.git
    path: resources
  type: automated
```

If everything went well, you should see:

```bash
$ oc -n drpaneas get cm config-map-in-private-repo
NAME                         DATA   AGE
config-map-in-private-repo   0      7m58
```

You can also verify this via the ArgoCD UI:

![](https://i.imgur.com/QtU6uld.png)

![](https://i.imgur.com/xRWtxxJ.png)

Or you can use the CLI:

```bash
# List the apps using the private repo (HTTPS & SSH)
$ argocd app list --grpc-web
NAME                                                                   CLUSTER     NAMESPACE  PROJECT  STATUS  HEALTH   SYNCPOLICY  CONDITIONS  REPO                                                               PATH       TARGET
gitops-service-argocd/gitopsdepl-46f91243-2d52-480d-b5f5-7c6a45492c04  in-cluster  panos      default  Synced  Healthy  Auto-Prune  <none>      https://github.com/managed-gitops-test-data/private-repo-test.git  resources
gitops-service-argocd/gitopsdepl-d8c2c272-9cf0-44bc-9218-e4759a7de7a7  in-cluster  drpaneas   default  Synced  Healthy  Auto-Prune  <none>      git@github.com:managed-gitops-test-data/private-repo-test.git      resources

# List the private repo (access via HTTPS & SSH)
$ argocd app list --grpc-web
NAME                                                                   CLUSTER     NAMESPACE  PROJECT  STATUS  HEALTH   SYNCPOLICY  CONDITIONS  REPO                                                               PATH       TARGET
gitops-service-argocd/gitopsdepl-46f91243-2d52-480d-b5f5-7c6a45492c04  in-cluster  panos      default  Synced  Healthy  Auto-Prune  <none>      https://github.com/managed-gitops-test-data/private-repo-test.git  resources
gitops-service-argocd/gitopsdepl-d8c2c272-9cf0-44bc-9218-e4759a7de7a7  in-cluster  drpaneas   default  Synced  Healthy  Auto-Prune  <none>      git@github.com:managed-gitops-test-data/private-repo-test.git      resources
```