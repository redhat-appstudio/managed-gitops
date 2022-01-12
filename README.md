# Managed GitOps Service Monorepo

**This is currently a work in progress and is subject to changes. Do not use this in any production environment.**

## Overview

> For an introduction to the project, and project details, see [Google Drive](https://drive.google.com/drive/u/0/folders/1p_yIOJ1WLu-lqz-BVDn076l1K1pEOc1d).

This repo is the home for all the Managed GitOps Service components.

This repo contains various tools and services that you can use to deploy, as well as the libraries you can use to develop the GitOps Service that will be integrated with RedHat AppStudio.

---

## Component Index

There are 4 separated, tighly-coupled components:

- [Backend]: responsible for receiving the latest user's intent and then informing both the database and the [Cluster-Agent] about it. It also includes a REST APIServer for webhooks, called [routes].
- [Cluster-Agent]: responsible for keeping synchronised the [ArgoCD Application CR] and its relevant Application entry in the database.
- [Backend Shared]: group of libraries shared among the rest of the components.
The frontend code is merely a mock collection of react pages created using [PatternFly](https://www.patternfly.org/) which is using dummy data to portray a very basic idea of how UI might look like. The actual UI for the same will differ.

For detailed step-by-step guide on how each component works, check the `README` file of each individual component.

### Other interesting files

- [Load Test]: _at the moment, this is just a barebones project for load testing_.
- [Manifests]: Postgress installation & some files related with the GitOps service deployment (such CRDs).
  - the [Operation CRD] is located elsewhere though, inside the [Backend Shared].
- [db-schema]: the database schema used by the components
- [psql.sh]: script that allows you to interact with the DB from the command line. Once inside the psql CLI, you may issue SQL statements, such as `select * from application;` (don't forget the semi-colon at the end, `;`!)
- `(create/delete/stop)-dev-env.sh`: Create, or delete or stop the database containers.

**NOTE**: See also, the targets in `Makefile` for additional available operations.
Plus, within each of the components there is a `Makefile`, which can be used for local development of that component.

---

## Development

### Build the components

Use either the main Makefile or the Makefile of each individual component (you can also call those from the main Makefile as well).
See the following _build_ targets:

```shell
make build                          # Build all the components
make build-backend                  # Build backend only
make build-cluster-agent            # Build cluster-agent only
make docker-build                   # Build docker image
```

### Build Container images

The project uses just one single base image from which you can select which `binary` you would like to run.
It basically clones the repository and builds each individual component using their own Makefile respectively.
Then it copies over their binaries into `/usr/local/bin`.
For more information, look at the Dockerfile.
By default it will run `/bin/bash` shell and it uses a `non-root` user.

#### Build and Push

```shell
# Login to the contaier registry (e.g. quay.io)
docker login quay.io

# Optional:
# You can override the name with provide your own username, image name, and tag
# by adding the 'IMG' variable: IMG=quay.io/USERNAME/IMAGE_NAME:TAG
# By default, the name would be 'gitops-service:latest' (defined in the Makefile)
# ----
# The IMG variable consists of other variables, such as:
#  * by adding 'USERNAME' variable you need to be logged into your registry.
#  * by adding 'BASE_IMAGE' variable you can change only the name of the image.
#  * by adding 'TAG' variable you can change only the version tag of the image

# Build the container
make docker-build

# Push to registry
make docker-push -USERNAME=drpaneas

# Or combine them
make docker-build docker-push IMG=quay.io/$USERNAME/$IMAGE_NAME:$TAG

# Other optional variables
make docker-build BASE_IMAGE="foo" TAG="bar"
```

#### Run the containers

```shell
docker run --rm -it gitops-service:latest gitops-service-backend
docker run --rm -it gitops-service:latest gitops-service-cluster-agent
```

or if you need to debug the image:

```shell
docker run --rm -it gitops-service:latest # to get /bin/bash
```

### Local development

To do proper development and testing of the GitOps Service there are some requirements you need to setup first.

**Development Environment Requirements**:

1. GNU/Linux or Mac Operating System (Windows with WSL/WSL2 or MSYS2, should also work, but is beyond the scope of this document.)
2. Running a local Kubernetes cluster: Our team uses [KinD] or [K3s] for simplicity.
You can use [kubectx/kubens](https://github.com/ahmetb/kubectx) to manage Kubernetes context if you like.
3. A container registry that you and your Kubernetes cluster can reach. We recommend [quay.io](https://quay.io/signin/).
4. User-permissions set so scripts can start/stop containers and interact with [Docker].
5. There are also other binaries (`kubectl`, `helm`, _etc_) required as well, which the Makefile will prompt you to install them when it needs them. Install the binary dependencies: `make download-deps`

Once you have a test environment already setup, you are ready to proceed with the testing workflow.

**Development Workflow**:

Currently the project has a mixture of unit-tests and integration tests, meaning running `go test` for some components, it is going to fail _unless_ you have the required test envrionment up and running.

To do that, follow these steps:

1. Have a local (or remote) Kubernetes cluster up and running (_e.g._ `kind create cluster`)
2. Install **ArgoCD** (`argocd` namespace) and **GitOps service** (`gitops` namespace) CRD manifests: `make -n install-argo`
3. Start the local containers (PostgreSQL and its UI Dashboard): `./create-dev-env.sh`
4. Start `goreman` that starts all the required processes in the same terminal: `make start`

Now, you can run the tests:

```shell
make test-backend-shared            # Run test for backend-shared only
make test-backend                   # Run tests for backend only
make test-cluster-agent             # Run test for cluster-agent only
make test                           # Or, run tests for all components
```

To reset the database run, `make reset-db`.

[Backend Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
[Frontend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/frontend
[Load Test]: https://github.com/redhat-appstudio/managed-gitops/tree/main/utilities/load-test#argo-cd-load-test-utility
[Manifests]: https://github.com/redhat-appstudio/managed-gitops/tree/main/manifests
[KinD]: https://kind.sigs.k8s.io/docs/user/quick-start/
[k3s]: https://k3s.io/
[EventLoop]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/eventloop
[ArgoCD Application CR]: https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/
[Another Event-Loop]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/managed-gitops/eventloop
[GitOps Operation Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/managed-gitops/operation_controller.go
[ArgoCD Application Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/argoproj.io/application_controller.go
[Docker]: https://www.docker.com/
[db-schema]: https://github.com/redhat-appstudio/managed-gitops/blob/main/db-schema.sql
[psql.sh]: https://github.com/redhat-appstudio/managed-gitops/blob/main/psql.sh
[Operation CRD]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
[routes]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/routes
