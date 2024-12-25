# Component Index

There are 4 separated, tightly-coupled components:

- [Backend]: responsible for watching and determining the user's intent via the `GitOpsDeployent*` API resources, next the backend component updates the database with the users intent, and finally it informs the [Cluster-Agent] which will act on the user's intent.
- [Cluster-Agent]: responsible ensuring that Argo CD state is up-to-date with the user intent (as defined in the database). Or, said another way, the cluster-agent component ensures that Argo CD resources ([ArgoCD Application CR]), and Cluster/Repository `Secret`s, are configured to be consistent with user intent.
- [App Studio Controller]: Responsible for implementing the App Studio Environment API (see [API doc for details](api.md)).
- [Backend Shared]: Contains code that is shared among the rest of the components. This is where shared utility code lives, such as GitOpsDeployment resource Go types, database Go types, and database access functionality. Including code within this component ensures it is not duplicated within the other components.

There is also a fifth component, which lives in a separate GitHub repository:
- [application-api]: defines the public API of GitOps Service 'appstudio-controller' component (e.g the Environment API), and the App Studio 'application-service' component.

For detailed step-by-step guide on how each component works, check the `README` file of each individual component.

## Other interesting files

- [Manifests]: Postgres installation & files related to deploying the GitOps Service to a Kubernetes cluster, via kustomize.
- [db-schema]: the database schema used by the components
- [psql.sh]: script that allows you to interact with the DB from the command line. Once inside the psql CLI, you may issue SQL statements, such as `select * from application;` (don't forget the semi-colon at the end, `;`!)
- `(create/delete/stop)-dev-env.sh`: Create, or delete or stop the database containers.
- [Load Test]: A small utility project for load testing Argo CD.

**NOTE**: See also, the targets in `Makefile` for additional available operations.
Plus, within each of the components there is a `Makefile`, which can be used for local development of that component.

## Deprecated components
- GitOps Service Frontend: The 'frontend' component of the GitOps Service was an early UI prototype for interfacing with the GitOps Service via a Web UI based on PatternFly. The contents of this prototype can be found [under this commit](https://github.com/redhat-appstudio/managed-gitops/tree/52696fbb48070bf43170687a6a775ff80dfb13be/frontend).

[application-api]: https://github.com/redhat-appstudio/application-api/
[App Studio Controller]: https://github.com/redhat-appstudio/managed-gitops/tree/main/appstudio-controller
[App Studio Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/appstudio-shared
[Backend Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
[Frontend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/frontend
[Load Test]: https://github.com/redhat-appstudio/managed-gitops/tree/main/utilities/load-test
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
[Design]: ./designs/gitops-service-internal-architecture-appstudio
