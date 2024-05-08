# Managed GitOps Service

<image width="45" align="left" src="https://user-images.githubusercontent.com/242652/138285004-b27d55b3-163b-4fe3-a8ff-6c34518044bd.png">

**This is currently a work in progress and is subject to changes. Do not use this in any production environment.**

---

## Overview

This repo is the home for all the Managed GitOps Service components.
It contains various tools and services that you can use to deploy, as well as the libraries you can use to develop the GitOps Service that will be integrated with the Stonesoup project.

This repository is closely associated with the [application-api](https://github.com/konflux-ci/application-api/) repository, which contains the public Stonesoup APIs exposed by the application-service and appstudio-controller components.

---

## Documentation

### ⚡ Project Info
* [Public Kubernetes/KCP API](./docs/api.md) - The Kubernetes-resource-based API that is used to interact with the GitOps Service.
* 👉 **[Component Index](./docs/components.md)** 👈 - Directory structure of this repo
* [Design] - High-level architecture of the GitOps Managed Service and its components
* [Google Drive] - Lot's of information for new people to the project

### ⭐ For Users

* [Install](./docs/install.md) - Install GitOps Managed Service to your Kubernetes cluster
* [Run it](./docs/run.md) - How to run the service

### 🔥 For Developers

* [Building](./docs/building.md) - Instructions for building each component and the Docker image
* [Development](./docs/development.md) - Instructions for developers who want to contribute
* [Debugging](./docs/debug.md) - Common errors and pitfalls

---

## Contributions

If you like to contribute to GitOps Managed Service, please be so kind to read our [Contribution Policy](./docs/CONTRIBUTING.md) first.

[Backend Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
[Load Test]: https://github.com/redhat-appstudio/managed-gitops/tree/main/utilities/load-test#argo-cd-load-test-utility
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
[Design]: https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.s0hdo22ap5cp
[Google Drive]: https://drive.google.com/drive/u/0/folders/1p_yIOJ1WLu-lqz-BVDn076l1K1pEOc1d
