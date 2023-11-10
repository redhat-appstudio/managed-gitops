# Self Healing of out-of-sync/orphaned API resources and database rows in GitOps Service

Within the GitOps Service, there a number of resources that track the service state, including `GitOpsDeployment*` APIs in the user namespace, database rows in the RDBMS, and Argo CD `Application`/`AppProject`/`Secret` *(both Cluster/Repository Secrets)* resources.

In exceptional circumstances (or due to bugs), it is possible (though it should be rare) for these resources to become out of sync with each other. For example: 
- A user creates a `GitOpsDeployment` resource in their Namespace, but the GitOps Service fails to create the corresponding `Application` row in the database.
- Or, an `Application` row is successfully created in the database, but the corresponding Argo CD Cluster Secret is not created in the Argo CD namespace.

In general, as a service, we wish to ensure that resources that are out of sync are quickly and automatically identified by the service itself. This avoids the need for manual intervention from team members based on user tickets.

Ideally resources would never be out of sync to begin with: we should also aim to fix the bugs that cause them to be out of sync. Fortunately, when our self-healing logic runs, it will log the actions that it takes. We can then examine the logs and investigate the issue that caused these self-healing actions to be required.

## Introduction

The self-healing/orphaned resource detection algorithms presented here are slightly over simplified vs the code actual code: in some cases, there may be additional conditions to prevent race conditions (such as skipping the processing of any resources that are newer than a certain age, using the `created_on` field).

## Terminology


A **link row** is a database row whose purpose is to 'link' (express some relationship between) two other entities. Currently, the following link rows exist: 
* **APICRToDBMapping**: Tracks the relationship between a `GitOpDeplomentManagedEnvironment`/`GitOpsDeploymentRepository`/`GitOpsDeploymentSyncRun` (but NOT a `GitOpsDeployment`) K8s API resource to/from the corresponding `ManagedEnvironment`/`RepositoryCredential`/`SyncOperation` row, respectively.
* **DeploymentToApplicationMapping**: Tracks the relationship between a `GitOpsDeployment` K8s API Resource and the corresponding `Application` row.
* **KubernetesToDBResourceMapping**: Tracks the relationship between a `Namespace`, and a row in the database. The nature of the relationship depends on which database row is involved.

These rows are primarily used to track the lifecycle of these resources across the service: For example, when a `GitOpsDeployment` is deleted, we can use the `DeploymentToApplicationMapping` row to find the corresponding `Application` row, and ensure that it is also deleted.

## Self-healing of `GitOpsDeployment*` APIs <-> Link rows 

This relationship is bidirectional, so we have code that scans in both directions.

### A) K8s API Resources -> Link row

**Self-healing algorithm**:
- This is handled by standard controller-runtime reconciliation. We don't have any specific logic for this in our code (because it's not necessary).
- By default, for any controller-runtime based controller: every X hours, controller-runtime will call `Reconcile` on all K8s resources that are watched by the controller, to allow them to be re-reconciled.
- After the resources are re-reconciled, they are handled by the usual runners/event handlers, which are listed below.

| From | To | Implementation Location |
| -------- | -------- | -------- |
| `GitOpsDeploymentManagedEnvironment` CR    | `APICRToDBMapping` row     | backend: `sharedresourceloop_managedenv.go`  |
| `GitOpsDeploymentRepositoryCredential` CR     | `APICRToDBMapping` row     | backend: `sharedresourceloop_repositorycredential.go`  |
| `GitOpsDeploymentSyncRun` CR    | `APICRToDBMapping` row     | backend: `application_event_runner_syncruns.go`  |
| `GitOpsDeployment` CR     | `DeploymentToApplicationMapping` row    | backend: `application_event_runner_deployments.go`     |


### B) Link row -> K8s API Resources

**Self-healing algorithm**:
- Iterate through all the `DeploymentToApplicationMapping`/`APICRToDBMapping` rows and see if the K8s resource still exist
    - If not found, delete the link row, delete the corresponding database row, and create an Operation pointing to the database row.


| From | To | Implementation Location |
| -------- | -------- | -------- |
| `APICRToDBMapping` row     | `GitOpsDeploymentManagedEnvironment` CR     | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ACTDM`/`cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment` |
| `APICRToDBMapping` row     | `GitOpsDeploymentRepositoryCredential` CR     | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ACTDM`/`cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential`     |
| `APICRToDBMapping` row     | `GitOpsDeploymentSyncRun` CR     | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ACTDM`/`cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun`     |
| `DeploymentToApplicationMapping` row     | `GitOpsDeployment` CR     | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_DTAM` |


## Self-healing of database rows <-> link rows

This relationship is likewise bidirectional, so we have code that scans in both directions.

### C) Link row to DB row

**Self-healing algorithm**:
- Iterate through all the `DeploymentToApplicationMapping`/`APICRToDBMapping` rows and see if the referenced DB row still exists
    - If not found, delete the link row.

| From | To | Implementation Location |
| -------- | -------- | -------- |
| `APICRToDBMapping` row    | `RepositoryCredentials` row    | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ACTDM`/`cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential`     |
| `APICRToDBMapping` row    | `ManagedEnvironment` row    | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ACTDM`/`cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment`     |
| `APICRToDBMapping` row    | `SyncOperation` row    | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ACTDM`/`cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun`     |
| `DeploymentToApplicationMapping` row     | `Application` row    | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_DTAM` |


### D) DB row to link row 

**Self-healing algorithm**:
- Iterate through all the DB rows to see if there a corresponding link row
    - If not found, delete the db row (and create an Operation)

| From | To | ImplementationLocation |
| -------- | -------- | -------- |
| `RepositoryCredentials` row    | `APICRToDBMapping` row   | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_RepositoryCredential`     |
| `ManagedEnvironment` row  | `APICRToDBMapping` row  | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_ManagedEnvironment`   |
| `SyncOperation` row  | `APICRToDBMapping` row | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_SyncOperation`     |
| `Application` row  | `DeploymentToApplicationMapping` row  | backend (`db_reconciler.go`): `cleanOrphanedEntriesfromTable_Application`      |


## Self-healing of DB Row <-> Argo CD CR 

This relationship is likewise bidirectional, so we have code that scans in both directions.

### E) Argo CD CR -> DB Row

**Self-healing algorithm**:
- Iterate through each Argo CD CR, and locate the corresponding DB row, using the annotation/label/name of the Argo CD resource
    - If not found, delete the Argo CD CR 

| From | To | Implementation Location |
| -------- | -------- | -------- |
| Argo CD `Application` CR | `Application` row | cluster-agent (`namespace_reconciler.go`): `cleanOrphanedCRsfromCluster_Applications` |
| Argo CD Cluster `Secret` CR|`ManagedEnvironment` row | cluster-agent (`namespace_reconciler.go`): `cleanOrphanedCRsfromCluster_Secret` |
| Argo CD Repository `Secret` CR|`RepositoryCredential` row | cluster-agent (`namespace_reconciler.go`): `cleanOrphanedCRsfromCluster_Secret` |
| `AppProject` CR | `AppProjectManagedEnv` row | TBD |
| `AppProject` CR | `AppProjectRepositoryCredentials` row | TBD |

### F) Row DB -> Argo CD CR

**Self-healing algorithm**:
- Iterate through each database row, and attempt to locate the corresponding Argo CD CR in the Argo CD namespace
    - If the Argo CD resource couldn't be found, then create a new `Operation` CR/row pointing to the DB row, so that the Argo CD resource is created
    - If the Argo CD resource is found, compare the contents. If they are different, create an `Operation` CR/row pointing to the DB row, so that the Argo CD resource is updated.

| From | To | Implementation Location |
| -------- | -------- | -------- |
| `Application` row | Argo CD `Application` CR | cluster-agent (`namespace_reconciler.go`): `syncCRsWithDB_Applications` |
| `ManagedEnvironment` row | Cluster `Secret`  resource | cluster-agent (`namespace_reconciler.go`): `recreateClusterSecrets_ManagedEnvironments` |
| `RepositoryCredential` row | Repository `Secret` resource |  cluster-agent (`namespace_reconciler.go`): `recreateClusterSecrets_RepositoryCredentials` |
| AppProjectManagedEnv row  | `AppProject` CR | TBD |
| AppProjectRepositoryCredentials row | `AppProject` CR | TBD |

## G) Operations

### Operation Row -> Operation CR

**Self-healing algorithm**:
- Iterate through Operation rows in DB:
    - If the Operation row is complete (has completed state), delete the Operation row
    - If the Operation row is not complete, and the corresponding Operation CR doesn't exist, then create an Operation CR pointing to that row.

| From | To | Implementation Location |
| -------- | -------- | -------- |
| `Operation` row | `Operation` CR | backend: `cleanOrphanedEntriesfromTable_Operation` |


### Operation CR -> Operation Row

**Self-healing algorithm**:
- Iterate through Operation CRs in Argo CD Namespce:
    - If the Operation row doesn't exist, delete the CR.
    - If the Operation row exists, but is complete, delete the CR. 

| From | To | Implementation Location |
| -------- | -------- | -------- |
| `Operation` CR | `Operation` row | cluster-agent (`namespace_reconciler.go`): `cleanOrphanedCRsfromCluster_Operation` |

## H) Others:

### Orphaned ClusterCredentials rows

**Self-healing algorithm**:
- Iterate through ClusterCredentials in the database, and identify any ClusterCredentials which are older than 1 hour:
    - For those ClusterCredentials, delete any ClusterCredentials that are not referenced by any ManagedEnvironments, or GitOpsEngineInstances.

This is implemented in `cleanOrphanedEntriesfromTable_ClusterCredential` in `backend/eventloop/db_reconciler.go`

### Orphaned ClusterUsers rows

**Self-healing algorithm**:
- Iterate through ClusterUsers in the database, and identify any ClusterCredentials which are older than 1 hour, AND which are not the special cluster user (used for admin jobs within GitOps Service controllers):
    - For those ClusterUsers, delete any ClusterUsers that are not referenced by ClusterAccess, RepositoryCredential, or Operation.

This is implemented in `cleanOrphanedEntriesfromTable_ClusterUser` in `backend/eventloop/db_reconciler.go`


### Orphaned Argo CD resources

In normal circumstances, when an Argo CD `Application` is deleted, Argo CD will automatically delete all the child resource of that `Application`.

However, in some exceptional circumstances, some cluster resources may be missed. This is a significant problem in RHTAP, where users do not necessarily have permissions to modify/delete all of the API resources within their Namespace.

To handle this case, every X hours, the GitOps Service scans the `*-tenant` namespaces of the clusters for any orphaned Argo-CD-deployed resources, and deletes those resources.

**Self-healing algorithm**:
- For each supported API resource on the cluster:
    - Retrieve all instances of that API resource on the cluster
        - Skip resources that are NOT in `*-tenant` Namespaces
        - Skip resources that have a non-nil `.metadata.deletionTimestamp`
    - Look for `app.kubernetes.io/instance` label on that resource: extract the GitOpDeployment UID from that label.
    - Look for a corresponding `GitOpsDeployment` in the same namespace as the resource that has that UID
        - If no such GitOpsDeployment exists, then delete the API resource.

This is implemented in `cleanOrphanedResources` in `backend/eventloop/cluster_reconciler.go`.


## I) KubernetesToDBResourceMapping

KubernetesToDBResourceMapping tracks the relationship between a Namespace, and a row in the database. The nature of the relationship depends on which database row is involved. This is described below.

| From | To | Description |
| -------- | -------- | -------- |
| `Namespace` row | `ManagedEnvironment` CR | Each user's API namespace has a corresponding ManagedEnvironment. A mapping is maintained between the user's API namespace (by UID) and the corresponding ManagedEnvironment row that corresponds to that Namespace.|
| `Namespace` row | `GitOpsEngineCluster` CR | A mapping between the kube-system Namespace (by `.metadata.uid`), and the GitOpsEngineCluster row. |
| `Namespace` row | `GitOpsEngineInstance` CR | A mapping of an Argo CD instance namepace on the cluster, and the corresponding GitOpsEngineInstance row for that Argo CD instance.|

At present, there is no logic for identifying orphaned KubernetesToDBResourceMappings, but this could be added in the future if the need arises.
