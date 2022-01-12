# Managed GitOps Cluster-Agent

[![Go Report Card](https://goreportcard.com/badge/github.com/redhat-appstudio/managed-gitops/cluster-agent)](https://goreportcard.com/report/github.com/redhat-appstudio/managed-gitops/cluster-agent)
[![GoDoc](https://godoc.org/github.com/redhat-appstudio/managed-gitops/cluster-agent?status.svg)](https://pkg.go.dev/mod/github.com/redhat-appstudio/managed-gitops/cluster-agent)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)
![Codecov](https://img.shields.io/codecov/c/github/redhat-appstudio/managed-gitops/tree/main/cluster-agent)

----

The Managed GitOps Cluster-Agent component is responsible for keeping synchronised the [ArgoCD Application CR] and its relevant Application entry in the database.

It consists of:

* A Kubernetes Operator with two controllers: [ArgoCD Application Controller] and [GitOps Operation Controller]
* The [EventLoop]; which is the reconciliation logic behind the [GitOps Operation Controller].

And also it comes with a [utils] pkg, that currently is used to retrieve Metrics.

----

## How it works

**Note: this is still in active development phase!**

The [Cluster-Agent] component gets triggered by either the [ArgoCD Application CR] or the [Operation CR], in the following scenarios respectively:

### Scenario 1: When application is unhealthy (difference between Git and Cluster)

**Source of truth**: Git Repo

If the _desired state_ (GitOps repo) is different from the the _observed state_ (app running into the cluster), then ArgoCD reports back _unhealthy_ via the `.status` section of the [ArgoCD Application CR].

The [ArgoCD Application Controller] is monitoring for Application CRs and as soon as there is a change in them, it starts reconciling.

The reconcilation logic, simply put, **compares** the Application CR with the its own application entry in the database, and finally it applies some logic into it.

### Scenario 2: When user wants to change what's in Git

**Source of truth**: Database

This change is noted originally by the [Managed GitOps Backend] which updates the database and then _pings_ the [Cluster-Agent] component by creating an [Operation CR].
The [GitOps Operation Controller] is monitoring for such Operation CR resources and starts the reconciliation loop.
The whole reconcilation logic has been _exported_ to the [EventLoop] which receives the Operation CR resource from the controller.
Using knowledge obtained from the [OperationID], the [EventLoop] retrieves the Application entry from the database (if cannot find it, it logs an error) and becomes aware of it.
It runs some checks to make sure this request is actually valid (e.g. target cluster exists, the resource exists in the namespace, etc) and if it's legit, it applies whatever the Application entry in the database says to the cluster.
Finally, it updates the Database that the Operation is now _complete_ and then it deletes the Operation CR.

**Note:**

* The API for the Operation is  not present in the same component, but in the [backend-shared](https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared/apis/managed-gitops/v1alpha1)

## Development

### Build

You can either build it from the [monorepo Makefile] typing: `make cluster-agent` or do it from within the component's Makefile itself typing `make build`.

### Test

This component is **not** meant to be tested in isolation, but it requires the rest of the monorepo components.

So if you trigger `make test` it's going to fail unless you have the required environment up and running.
To do this, please refer to [monorepo README] to find more information about it.

### Generate API changes

The [Operation CRD API] is part of the the [backend-shared] component of the monorepo.
In case you have made any changes to the API, please re-generate the CRDs and DeepCopy:

```shell
make manifests generate
```

**NOTE**: The generated files will be part of the **backend-shared** component of the monorepo.

### Initial bootstrap

Currently, it built using the **Operator SDK Framework** version **v1.11**

[ArgoCD Application Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/argoproj.io/application_controller.go
[GitOps Operation Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/managed-gitops/operation_controller.go
[EventLoop]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/managed-gitops/eventloop/eventloop.go
[Namespace instance]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#namespace-v1-core
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
[monorepo Makefile]: https://github.com/redhat-appstudio/managed-gitops/blob/main/Makefile
[monorepo README]: https://github.com/redhat-appstudio/managed-gitops/blob/main/README.md
[ArgoCD Application CR]: https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/
[Managed GitOps Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Operation CR]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
[OperationID]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/apis/managed-gitops/v1alpha1/operation_types.go#L25
[Operation CRD API]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared/apis/managed-gitops/v1alpha1
[backend-shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[utils]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent/utils