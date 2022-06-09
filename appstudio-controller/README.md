# AppStudio Controller

Responsible for interfacing with other components of App Studio, including watching them and creating corresponding API objects in the GitOps Serice. For example: When an Argo CD `Application` resource is created, the *appstudio-controller* should create a corresponding `GitOpsDeployment` resource.

## Trying out the AppStudio Controller

You can try out the AppStudio Controller.

Ensure that the appropriate resources are installed to your k8s cluster, and start the GitOps service:
```bash
make devenv-docker
make start
```

Create the 'jgw' namespace:
```bash
kubectl create namespace jgw
```

Next, you can create AppStudio `Application` or `Component` resources, which the AppStudio controller will respond to. See the [examples](examples) folder README.md for details.

### Bootstrapped with operator-sdk v1.17

These are the commands that were used to bootstrap the AppStudio controller. This should be useful when we later upgrade (re-bootstrap) onto a newer version of the operator-sdk.

```yaml
go mod init github.com/redhat-appstudio/managed-gitops/appstudio-controller

operator-sdk-v1.17 init --domain redhat.com

# Next, edit the PROJECT file and add:
# multigroup: true
# to an empty line.

operator-sdk-v1.17 create api --group appstudio.redhat.com --version v1alpha1 --kind Application --controller # --resource 
```

