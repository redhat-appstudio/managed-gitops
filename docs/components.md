# Component Index

There are 4 separated, tightly-coupled components:

- [Backend]: responsible for receiving the latest user's intent and then informing both the database and the [Cluster-Agent] about it. It also includes a REST APIServer for webhooks, called [routes].
- [Cluster-Agent]: responsible for keeping synchronised the [ArgoCD Application CR] and its relevant Application entry in the database.
- [App Studio Controller]: Responsible for interfacing with other components of App Studio, including watching them and creating corresponding API objects in the GitOps Serice. For example: When an Argo CD `Application` resource is created, we might create a corresponding `GitOpsDeployment` resource.
- [App Studio Shared]: Go types and CRDs for the AppStudio Environment API.
- [Backend Shared]: group of libraries shared among the rest of the components.
The frontend code is merely a mock collection of react pages created using [PatternFly](https://www.patternfly.org/) which is using dummy data to portray a very basic idea of how UI might look like. The actual UI for the same will differ.

For detailed step-by-step guide on how each component works, check the `README` file of each individual component.

## Other interesting files

- [Load Test]: _at the moment, this is just a bare-bone project for load testing_.
- [Manifests]: Postgres installation & some files related with the GitOps service deployment (such CRDs).
  - the [Operation CRD] is located elsewhere though, inside the [Backend Shared].
- [db-schema]: the database schema used by the components
- [psql.sh]: script that allows you to interact with the DB from the command line. Once inside the psql CLI, you may issue SQL statements, such as `select * from application;` (don't forget the semi-colon at the end, `;`!)
- `(create/delete/stop)-dev-env.sh`: Create, or delete or stop the database containers.

**NOTE**: See also, the targets in `Makefile` for additional available operations.
Plus, within each of the components there is a `Makefile`, which can be used for local development of that component.

[App Studio Controller]: https://github.com/redhat-appstudio/managed-gitops/tree/main/appstudio-controller
[App Studio Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/appstudio-shared
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
[Design]: https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.s0hdo22ap5cp
