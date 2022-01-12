
# Managed GitOps Backend

[![Go Report Card](https://goreportcard.com/badge/github.com/redhat-appstudio/managed-gitops/backend)](https://goreportcard.com/report/github.com/redhat-appstudio/managed-gitops/backend)
[![GoDoc](https://godoc.org/github.com/redhat-appstudio/managed-gitops/backend?status.svg)](https://pkg.go.dev/mod/github.com/redhat-appstudio/managed-gitops/backend)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)
![Codecov](https://img.shields.io/codecov/c/github/redhat-appstudio/managed-gitops/tree/main/backend)

----

The Managed GitOps Backend component is responsible for receiving the latest user's intent and then informing both the database and the [Cluster-Agent] about it.
It consists of:

A Kubernetes Operator with two controllers:

* [GitOps Deployment Controller]
* [GitOps Deployment SyncRun Controller]
* The [EventLoop]; which is the reconciliation logic behind the two controllers.

It has the following manifests (`YAML`):

* [GitOpsDeployment CRD]: required for the [GitOps Deployment Controller].
* [GitOpsDeploymentSyncRun CRD]: required for the [GitOps Deployment SyncRun Controller]

Also, it comes with a REST APIServer used for plugging webhooks (_currently not used in the code_)

Lastly, there are also some complementary helpful functions inside the [util] package.

----

## How it works

**Note: this is still in active development phase!**

### Getting user's intent

When the user wants to perform an action, a `GitOpsDeployment` Custom Resource (CR) is applied to the namespace of their choice.
This can be either a new `GitOpsDeployment` CR or a modification (even deletion) of an already existing one.
When this happens, the [GitOps Deployment Controller] gets triggered.
It instantiates a [Namespace instance] and sets only its name (_taken from the CR_) and passes this down to the [EventLoop].

### Creates an Event and checks if there's _valid_ work to do

The [EventLoop] receives this as an _event_ and gathers information about it, such as `workspaceID` and `gitopsDeplID`.
Once all the required information is gathered, it starts _processing_ it, by doing a series of checks to make sure this is a valid work.

### Work Part 1: Update the Database

If yes, then it sends the work down the `depl event runner`.
The `depl event runner` is trying to answer if this _event_ is related to an _already existing_ deployment or _not_.
This is important because we need to know if we need to _create_ or _get_ resources.
In any case, it creates a database entry with some metadata such as the `workspaceID`, the `gitopsDeplID` and an Operation (which consists of `operation-id`, `instance-id`, `owner`, `resource` and a `resource-type`).

### Work Part 2: Inform the [Cluster-Agent]

After updating the database, the `depl event runner` passes the information back to [Cluster-Agent], by creating an `Operation CR` into the `argocd` namespace, with the appropriate operation information from the database.

### Waiting ...

From here, the [Cluster-Agent] and ArgoCD instance are getting triggered, and they create an ArgoCD application.

**NOTE**: This is still in active development. Per current implementation of the [EventLoop] is not waiting for them, and it _deletes_ the `Operation` CR almost immediately after its creation.
This marks the finish end of the event processing work.

#### Missing documentation:

* Document the `GitOpsDeploymentSyncRun` scenario.
* Document the internals of `EventLoop` into its own `README`.

## Development

### Build

You can either build it from the [monorepo Makefile] typing: `make build-backend` or do it from within the component's Makefile itself typing `make build`.

### Test

This component is **not** meant to be tested in isolation, but it requires the rest of the monorepo components.
So if you trigger `make test` it's going to fail unless you have the required environment up and running.
To do this, please refer to [monorepo README] to find more information about it.

### Generate API changes

In case you have made any changes to the API, please re-generate the CRDs and DeepCopy:

```shell
make manifests generate
```

### Initial bootstrap

Currently, it built using the **Operator SDK Framework** version **v1.11**

[GitOps Deployment Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend/controllers/managed-gitops/gitopsdeployment_controller.go
[GitOps Deployment SyncRun Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend/controllers/managed-gitops/gitopsdeploymentsyncrun_controller.go
[EventLoop]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/eventloop
[Namespace instance]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#namespace-v1-core
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
[monorepo Makefile]: https://github.com/redhat-appstudio/managed-gitops/blob/main/Makefile
[monorepo README]: https://github.com/redhat-appstudio/managed-gitops/blob/main/README.md
[routes]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/routes
[GitOpsDeployment CRD]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml
[GitOpsDeploymentSyncRun CRD]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml
[util]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/util