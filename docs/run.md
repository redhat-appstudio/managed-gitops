# Run

## Current status

Given you have [installed](./install.md) GitOps Managed Service into your Kubernetes cluster, you can start using it by applying a CR.
This can be either a `GitOpsDeployment` or a `GitOpsDeploymentSyncRun` to trigger the `backend` operator.

Here's a sample:

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment
metadata:
  name: gitopsdeployment-sample
spec:
  source:
    repoURL: "https://github.com/user_name/repo_name"
  type: "manual"
```

In the current development phase, this is the expected output:

**backend logs**

```shell
kubectl -n gitops logs managed-gitops-backend-service-77c9648b5b-hgkt4 -c manager -f
```

```
2022-01-31T11:23:22.945Z	DEBUG	preprocess event loop router received event:	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "name": "gitopsdeployment-sample", "namespace": "default", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: ''"}
2022-01-31T11:23:22.952Z	DEBUG	Emitting event to workspace event loop	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "name": "gitopsdeployment-sample", "namespace": "default", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'"}
2022-01-31T11:23:22.953Z	DEBUG	eventLoop received event	{"event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'", "workspace": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:22.953Z	INFO	workspaceEventLoopRouter started	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:22.953Z	DEBUG	applicationEventLoop received event	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'"}
2022-01-31T11:23:22.955Z	INFO	applicationEventQueueLoop started. gitopsDeplID: 9a4a6a74-900b-4101-b9a2-4bdf6d72060a. workspaceID: 815fa61b-0c05-437e-9f72-401e7cefecfd	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "gitOpsDeplID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a"}
2022-01-31T11:23:22.955Z	DEBUG	applicationEventQueueLoop received event	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "gitOpsDeplID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'"}
2022-01-31T11:23:22.955Z	DEBUG	applicationEventLoopRunner started	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:22.956Z	DEBUG	applicationEventLoopRunner started	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:22.972Z	DEBUG	applicationEventLoopRunner - event received	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'"}
2022-01-31T11:23:22.972Z	DEBUG	applicationEventLoopRunner - processing event	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'", "attempt": 1}
2022-01-31T11:23:22.972Z	DEBUG	Sent work to depl event runner	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "gitOpsDeplID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'"}
2022-01-31T11:23:22.983Z	DEBUG	sharedResourceEventLoop receive message: getOrCreateClusterUserByNamespaceUID	{"workspace": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:22.992Z	DEBUG	workspacerEventLoopRunner_handleDeploymentModified processing event	{"gitopsDeploymentExists": true, "deplToAppMapExists": false}
2022-01-31T11:23:22.992Z	DEBUG	sharedResourceEventLoop receive message: getOrCreateSharedResources	{"workspace": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:23.002Z	INFO	STUB: handleNewGitOpsDeplEvent after getOrCreateManagedEnvironmentByNamespaceUID, skipping creating the Argo CD Cluster Secret via a new Operation.
2022-01-31T11:23:23.191Z	INFO	Creating database operation	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "operation": "operation-id: , instance-id: e7020b99-65ae-415b-b944-e7e5bc89ab19, owner: 59f42d7a-6d02-4f79-965b-44073d1ad7ed, resource: 5b5a872c-2ad8-42d2-9a4e-3282e97b8c69, resource-type: Application, "}
2022-01-31T11:23:23.197Z	INFO	Creating K8s Operation CR	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "operation": "b41de877-1524-4522-8ac6-e8952c5e8172"}
2022-01-31T11:23:23.209Z	INFO	STUB: Not waiting for create Application operation to complete, in handleNewGitOpsDeplEvent	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd"}
2022-01-31T11:23:23.209Z	DEBUG	Deleting operation CR: operation-b41de877-1524-4522-8ac6-e8952c5e8172	{"gitopsDeplUID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "operation": "b41de877-1524-4522-8ac6-e8952c5e8172", "namespace": "argocd"}
2022-01-31T11:23:23.224Z	DEBUG	applicationEventQueueLoop received work complete event	{"workspaceID": "815fa61b-0c05-437e-9f72-401e7cefecfd", "gitOpsDeplID": "9a4a6a74-900b-4101-b9a2-4bdf6d72060a", "event": "[DeploymentModified] default/gitopsdeployment-sample/GitOpsDeployment, for workspace '815fa61b-0c05-437e-9f72-401e7cefecfd', gitopsdepluid: '9a4a6a74-900b-4101-b9a2-4bdf6d72060a'"}
```

**cluster-agent logs**

```shell
kubectl -n gitops logs managed-gitops-clusteragent-service-85d4db6968-gl98l -c manager -f
```

It crashes:

```
2022-01-31T11:23:55.057Z	INFO	controller-runtime.manager.controller.operation	Operation event seen in reconciler: argocd/operation-8b25bf97-a9d7-4508-94cf-560b319a46c2	{"reconciler group": "managed-gitops.redhat.com", "reconciler kind": "Operation", "name": "operation-8b25bf97-a9d7-4508-94cf-560b319a46c2", "namespace": "argocd"}
SELECT "op"."operation_id", "op"."seq_id", "op"."instance_id", "op"."resource_id", "op"."operation_owner_user_id", "op"."resource_type", "op"."created_on", "op"."last_state_update", "op"."state", "op"."human_readable_state" FROM "operation" AS "op" WHERE (operation_id = '8b25bf97-a9d7-4508-94cf-560b319a46c2')
2022-01-31T11:23:55.072Z	INFO	controller-runtime.manager.controller.operation	Operation event seen in reconciler: argocd/operation-8b25bf97-a9d7-4508-94cf-560b319a46c2	{"reconciler group": "managed-gitops.redhat.com", "reconciler kind": "Operation", "name": "operation-8b25bf97-a9d7-4508-94cf-560b319a46c2", "namespace": "argocd"}
SELECT "gei"."gitopsengineinstance_id", "gei"."seq_id", "gei"."namespace_name", "gei"."namespace_uid", "gei"."enginecluster_id" FROM "gitopsengineinstance" AS "gei" WHERE (gei.Gitopsengineinstance_id = 'e7020b99-65ae-415b-b944-e7e5bc89ab19')
SELECT "application"."application_id", "application"."seq_id", "application"."name", "application"."spec_field", "application"."engine_instance_inst_id", "application"."managed_environment_id" FROM "application" AS "application" WHERE (application_id = '5b5a872c-2ad8-42d2-9a4e-3282e97b8c69')
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x18 pc=0x28e1220]

goroutine 140 [running]:
github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop.(*processEventTask).performTask(0xc000bc1f38, 0x312ae48, 0xc00013a010, 0x0, 0x0, 0x0)
	/workspace/cluster-agent/controllers/managed-gitops/eventloop/eventloop.go:196 +0x9e0
github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop.controllerEventLoopRouter(0xc00036ccc0)
	/workspace/cluster-agent/controllers/managed-gitops/eventloop/eventloop.go:78 +0xcf
created by github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop.NewControllerEventLoop
	/workspace/cluster-agent/controllers/managed-gitops/eventloop/eventloop.go:36 +0x8c
```

To trigger the `cluster-agent` you have to create an `Operation` CR that points to a fake application (or an `ArgoCD application` CR):

```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: Operation
metadata:
  name: operation-sample
spec:
  operationID: "fake-uuid!!!!"
```

The cluster-agent should wake up, detect the change, attempt to connect to the database (which should succeed), and throw a database error indicating that there was no Operation table entry with the given id.

```
2022-01-31T11:25:13.951Z	INFO	controller-runtime.manager.controller.operation	Operation event seen in reconciler: default/operation-sample	{"reconciler group": "managed-gitops.redhat.com", "reconciler kind": "Operation", "name": "operation-sample", "namespace": "default"}
SELECT "op"."operation_id", "op"."seq_id", "op"."instance_id", "op"."resource_id", "op"."operation_owner_user_id", "op"."resource_type", "op"."created_on", "op"."last_state_update", "op"."state", "op"."human_readable_state" FROM "operation" AS "op" WHERE (operation_id = 'fake-uuid!!!!')

2022-01-31T11:25:13.966Z	ERROR	unable to locate operation 'fake-uuid!!!!': no rows in result set	{"error": "unable to locate operation 'fake-uuid!!!!': no rows in result set"}
github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop.controllerEventLoopRouter
	/workspace/cluster-agent/controllers/managed-gitops/eventloop/eventloop.go:78
  ```

However, this should test that cluster-agent has appropriate permission to see the Operation CR, and that it can connect to the DB and issues queries.
