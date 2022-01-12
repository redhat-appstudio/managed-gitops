# Managed GitOps Backend-Shared Libraries

[![Go Report Card](https://goreportcard.com/badge/github.com/redhat-appstudio/managed-gitops/backend-shared)](https://goreportcard.com/report/github.com/redhat-appstudio/managed-gitops/backend-shared)
[![GoDoc](https://godoc.org/github.com/redhat-appstudio/managed-gitops/backend-shared?status.svg)](https://pkg.go.dev/mod/github.com/redhat-appstudio/managed-gitops/backend-shared)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)
![Codecov](https://img.shields.io/codecov/c/github/redhat-appstudio/managed-gitops/tree/main/backend-shared)

----

The Managed GitOps Backend-Shared is a group of libraries shared among the rest of the components.

It consists of the following exported Go packages:

- [Operation API]: used by the [GitOps Operation Controller] of the [Cluster-Agent] component.
- [Database functions and structs]: interact with the PostgreSQL database.
- [Service Account]: creates a service account on the remote cluster.
- [Proxy Client]: a simple utility function/struct that may be used to write unit tests that mock the K8s client.
- [Find an ArgoCD Instance]: functions responsible to return an available ArgoCD instance.

It has the following manifests (`YAML`):

- [Operation CRD]: required for the [GitOps Operation Controller].

----

## Development

There is nothing to `go build` here.
These are Go packages imported by the other components of the monorepo.

You can do:

```shell
make manifests generate
make lint
make gosec
```

### To run unit tests:

This component is **not** meant to be tested in isolation, but it requires the rest of the monorepo components.

Triggering `make test` it's _probably_ going to **fail** unless you have the required environment up and running.
To do this, please refer to [monorepo README] to find more information about it.


[Operation API]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared/apis/managed-gitops/v1alpha1
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
[backend-shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-share
[Database functions and structs]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared/config/db
[Service Account]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/hack/service_account.go
[Proxy Client]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/util/proxyclient.go
[Find an ArgoCD instance]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/util/utils.go
[Operation CRD]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml