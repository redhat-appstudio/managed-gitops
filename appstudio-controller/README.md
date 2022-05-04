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

Here is a sample Application yaml you can use (which is valid schema, as of this writing). You can `kubectl apply -f` this, and the appstudio-controller should create a corresponding `GitOpsDeployment` resource.
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Application
metadata:
#  finalizers:
#  - application.appstudio.redhat.com/finalizer
  name: my-app
  namespace: jane
spec:
  appModelRepository:
    url: "https://github.com/jgwest/my-app"
  displayName: my-app
  gitOpsRepository:
    url: "https://github.com/jgwest/my-app"
status:
  conditions: []
  devfile: |
    metadata:
      attributes:
        appModelRepository.context: /
        appModelRepository.url: https://github.com/redhat-appstudio-appdata/test-application-concentrate-complete
        gitOpsRepository.context: /
        gitOpsRepository.url: https://github.com/redhat-appstudio-appdata/test-application-concentrate-complete
      name: Test Application
    projects:
    - git:
        remotes:
          origin: https://github.com/devfile-samples/devfile-sample-code-with-quarkus
      name: java-quarkus
    schemaVersion: 2.1.0
```

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

