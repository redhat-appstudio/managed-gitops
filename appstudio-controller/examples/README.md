# AppStudio Resource Examples

The folder contains sample AppStudio resources, such as `Application` and `Component`, which you can use for testing the appstudio-controller. 

## How do I create appstudio resources, such as Application/Component, for testing the appstudio-controller components?

First, you can create then using `kubectl apply -f`.

HOWEVER, this will not set the `status` field of these resources.
- The status field contains valuable information that is required by the GitOps Service, such as where the GitOps repository can be found.

To set the status field, I recommend using the [`kubectl-edit-status` kubectl plugin](https://github.com/ulucinar/kubectl-edit-status), which can be installed using krew. `kubectl` itself doesn't support updating the `.status` field of a resource from the command line, so we need to use this plugin.

For example:
```
kubectl apply -f application.yaml -n (your namespace)
kubectl edit-status application.appstudio.redhat.com/new-demo-app -n (your namespace)
# Next, paste the `status` field from application.yaml into the editor, and save.
```
