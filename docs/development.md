# Local development

To do proper development and testing of the GitOps Service there are some requirements you need to set up first.

**Development Environment Requirements**:

1. GNU/Linux or Mac Operating System (Windows with WSL/WSL2 or MSYS2, should also work, but is beyond the scope of this document.)
2. Running a local OpenShift cluster:
You can use [kubectx/kubens](https://github.com/ahmetb/kubectx) to manage Kubernetes context if you like.
3. A container registry that you and your Kubernetes cluster can reach. We recommend [quay.io](https://quay.io/signin/).
4. User-permissions set so scripts can start/stop containers and interact with [Docker].
5. There are other binaries (`kubectl`, `helm`, _etc_) required as well, which the Makefile will prompt you to install them when it needs them. Install the binary dependencies: `make download-deps`

Once you have a test environment already setup, you are ready to proceed with either the Docker or the Kubernetes workflow.

**Option 1: Development Workflow with Docker**:

Currently, the project has a mixture of unit-tests and integration tests, meaning running `go test` for some components, it is going to fail _unless_ you have the required test environment up and running.

To do that, follow these steps:

1. Have a local (or remote) OpenShift cluster up and running
2. Install the namespaces, CRDs and RBAC resources for both `cluster-agent` and `backend` and start the local containers (PostgreSQL and its UI dashboard): `make devenv-docker`.
3. Start `goreman` that starts all the required processes in the same terminal: `make start`

Now, you can run the tests:

```shell
make test-backend-shared            # Run test for backend-shared only
make test-backend                   # Run tests for backend only
make test-cluster-agent             # Run test for cluster-agent only
make test-appstudio-controller      # Run test for appstudio-controller only
make test                           # Or, run tests for all components
```

To reset the database run, `make reset-db`.

**Option 2: Development Workflow with Kubernetes**:

The difference in this workflow is that the PostgreSQL is running into the Kubernetes cluster.
In order to be able to access it from the outside, you would need to port-forward it locally.
Also, you will need to set a local variable `DB_PASS` with the password of PostgreSQL database, so the Pods can access it.

To do that, follow these steps:

1. Have a local (or remote) OpenShift cluster up and running
2. Deploy the components into the Kubernetes cluster: `make devenv-k8s IMG=quay.io/pgeorgia/gitops-service:latest`.

For your curiosity, this _make target_ is making sure that:

* cluster-agent/backend/backend-shared/appstudio-controller/application-api CRDs and RBAC resources are applied into the cluster 
* postgres full deployment is running on the cluster

In the end, you will see a similar output:

```shell
 ------------------------------------------------
 | To access the database outside the cluster   |
 ------------------------------------------------
  - Run:            kubectl port-forward --namespace gitops svc/gitops-postgresql-staging 5432:5432 &
  - Credentials:    HOST=127.0.0.1:5432  USERNAME=postgres  PASSWORD=(password)  DB=($POSTGRESQL_DATABASE)
  - Access Example: psql postgresql://postgres:3CqCKcXLyN@127.0.0.1:5432/$POSTGRESQL_DATABASE -c "select now()"

  - To run Backend & ClusterAgent Operators locally: export DB_PASS=(password) && goreman start
 -------------------------------------------------
 ```

Continue by applying the local port-forward and `DB_PASS` env variable.
See this example:

 1. Apply the local port-forward for the database: `kubectl port-forward --namespace gitops svc/gitops-postgresql-staging 5432:5432 &` (don't forget to _kill_ this later).
 2. Run the operators locally: `export DB_PASS=(password) && make start`.

 > To reset the database run, `make undeploy-postgresql` and afterwards install it again: `deploy-postgresql`.
 > In case you have problem with accessing the database, follow the [debug guide](./debug.md) for further investigation.

So, what is running in your cluster is only the PostgreSQL database, while the two operators (clusteragent and backend) are running locally (their respective resources although are loaded into the cluster).

[Backend Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
