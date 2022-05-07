package db

import (
	"context"
	"time"
)

//    \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/   \/
//
// See the separate 'db-schema.sql' schema file, for descriptions of each of these tables
// and the fields within them.
//
//    /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\   /\

// GitopsEngineCluster is used to track clusters that host Argo CD instances
type GitopsEngineCluster struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"gitopsenginecluster"` //nolint

	Gitopsenginecluster_id string `pg:"gitopsenginecluster_id,pk"`

	SeqID int64 `pg:"seq_id"`

	// -- pointer to credentials for the cluster
	// -- Foreign key to: ClusterCredentials.clustercredentials_cred_id
	Clustercredentials_id string `pg:"clustercredentials_id"`
}

// GitopsEngineInstance is an Argo CD instance on a Argo CD cluster
type GitopsEngineInstance struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"gitopsengineinstance,alias:gei"` //nolint

	Gitopsengineinstance_id string `pg:"gitopsengineinstance_id,pk"`
	SeqID                   int64  `pg:"seq_id"`

	// -- An Argo CD cluster may host multiple Argo CD instances; these fields
	// -- indicate which namespace this specific instance lives in.
	Namespace_name string `pg:"namespace_name"`
	Namespace_uid  string `pg:"namespace_uid"`

	// -- Reference to the Argo CD cluster containing the instance
	// -- Foreign key to: GitopsEngineCluster.gitopsenginecluster_id
	EngineCluster_id string `pg:"enginecluster_id"`
}

// ManagedEnvironment is an environment (eg a user's cluster, or a subset of that cluster) that they want to deploy applications to, using Argo CD
type ManagedEnvironment struct {
	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"managedenvironment,alias:me"` //nolint

	Managedenvironment_id string `pg:"managedenvironment_id,pk"`
	SeqID                 int64  `pg:"seq_id"`

	// -- human readable name
	Name string `pg:"name"`

	// -- pointer to credentials for the cluster
	// -- Foreign key to: ClusterCredentials.clustercredentials_cred_id
	Clustercredentials_id string `pg:"clustercredentials_id"`
}

// ClusterCredentials contains the credentials required to access a K8s cluster.
// The credentials may be in one of two forms:
// 1) Kubeconfig state: Kubeconfig file, plus a reference to a specific context within the
//     - This is the same content as can be found in your local '~/.kube/config' file
//     - This is what the user would initially provide via the Service/Web UI/CLI
//     - There may be (likely is) a better way of doing this, but this works for now.
// 2) ServiceAccount state: A bearer token for a service account on the target cluster
//     - Same mechanism Argo CD users for accessing remote clusters
//
// You can tell which state the credentials are in, based on whether 'serviceaccount_bearer_token' is null.
//
// It is the job of the cluster agent to convert state 1 (kubeconfig) into a service account
// bearer token on the target cluster (state 2).
//     - This is the same operation as the `argocd cluster add` command, and is the same
//       technique used by Argo CD to interface with remove clusters.
//     - See https://github.com/argoproj/argo-cd/blob/a894d4b128c724129752bac9971c903ab6c650ba/cmd/argocd/commands/cluster.go#L116
type ClusterCredentials struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"clustercredentials,alias:cc"` //nolint

	// -- Primary key for the credentials (UID)
	Clustercredentials_cred_id string `pg:"clustercredentials_cred_id,pk"`

	SeqID int64 `pg:"seq_id"`

	// -- API URL for the cluster
	// -- Example: https://api.ci-ln-dlfw0qk-f76d1.origin-ci-int-gce.dev.openshift.com:6443
	Host string `pg:"host"`

	// -- State 1) kube_config containing a token to a service account that has the permissions we need.
	Kube_config string `pg:"kube_config"`

	// -- State 1) The name of a context within the kube_config
	Kube_config_context string `pg:"kube_config_context"`

	// -- State 2) ServiceAccount bearer token from the target manager cluster
	Serviceaccount_bearer_token string `pg:"serviceaccount_bearer_token"`

	// -- State 2) The namespace of the ServiceAccount
	Serviceaccount_ns string `pg:"serviceaccount_ns"`
}

// ClusterUser is an individual user/customer
// Note: This is basically placeholder: a real implementation would need to be way more complex.
type ClusterUser struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"clusteruser,alias:cu"` //nolint

	Clusteruser_id string `pg:"clusteruser_id,pk"`
	User_name      string `pg:"user_name"`
	SeqID          int64  `pg:"seq_id"`
}

type ClusterAccess struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"clusteraccess"` //nolint

	// -- Describes whose managed environment this is (UID)
	// -- Foreign key to: ClusterUser.Clusteruser_id
	Clusteraccess_user_id string `pg:"clusteraccess_user_id,pk"`

	// -- Describes which managed environment the user has access to (UID)
	// -- Foreign key to: ManagedEnvironment.Managedenvironment_id
	Clusteraccess_managed_environment_id string `pg:"clusteraccess_managed_environment_id,pk"`

	// -- Which Argo CD instance is managing the environment?
	// -- Foreign key to: GitOpsEngineInstance.Gitopsengineinstance_id
	Clusteraccess_gitops_engine_instance_id string `pg:"clusteraccess_gitops_engine_instance_id,pk"`

	SeqID int64 `pg:"seq_id"`
}

type OperationState string

const (
	OperationState_Waiting     OperationState = "Waiting"
	OperationState_In_Progress OperationState = "In_Progress"
	OperationState_Completed   OperationState = "Completed"
	OperationState_Failed      OperationState = "Failed"
)

const (
	OperationResourceType_SyncOperation = "SyncOperation"
	OperationResourceType_Application   = "Application"
)

// Operation
// Operations are used by the backend to communicate database changes to the cluster-agent.
// It is the reponsibility of the cluster agent to respond to operations, to read the database
// to discover what database changes occurred, and to ensure that Argo CD is consistent with
// the database state.
//
// See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.9tzaobsoav27
// for description of Operation
type Operation struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"operation,alias:op"` //nolint

	// Auto-generated primary key, based on a random UID
	Operation_id string `pg:"operation_id,pk"`

	// -- Specifies which Argo CD instance this operation is targeting
	// -- Foreign key to: GitopsEngineInstance.gitopsengineinstance_id
	Instance_id string `pg:"instance_id"`

	// Primary key of the resource that was updated
	Resource_id string `pg:"resource_id"`

	// -- The user that initiated the operation.
	Operation_owner_user_id string `pg:"operation_owner_user_id"`

	// Resource type of the the resoutce that was updated.
	// This value lets the operation know which table contains the resource.
	//
	// Possible values:
	// * ClusterAccess (specified when we want Argo CD to C/R/U/D a user's cluster credentials)
	// * GitopsEngineInstance (specified to CRUD an Argo instance, for example to create a new namespace and put Argo CD in it, then signal when it's done)
	// * Application (user creates a new Application via service/web UI)
	Resource_type string `pg:"resource_type"`

	// -- When the operation was created. Used for garbage collection, as operations should be short lived.
	Created_on time.Time `pg:"created_on"`

	// -- last_state_update is set whenever state changes
	// -- (initial value should be equal to created_on)
	Last_state_update time.Time `pg:"last_state_update"`

	// Whether the Operation is in progress/has completed/has been processed/etc.
	// (possible values: Waiting / In_Progress / Completed / Failed)
	State OperationState `pg:"state"`

	// -- If there is an error message from the operation, it is passed via this field.
	Human_readable_state string `pg:"human_readable_state"`

	SeqID int64 `pg:"seq_id"`
}

// Application represents an Argo CD Application CR within an Argo CD namespace.
type Application struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"application"` //nolint

	// primary key: auto-generated random uid.
	Application_id string `pg:"application_id,pk"`

	// Name of the Application CR within the namespace
	// Value: gitopsdepl-(uid of the gitopsdeployment)
	// Example: gitopsdepl-ac2efb8e-2e2a-45a2-9c08-feb0e2e0e29b
	Name string `pg:"name"`

	// '.spec' field of the Application CR
	// Note: Rather than converting individual JSON fields into SQL Table fields, we just pull the whole spec field.
	Spec_field string `pg:"spec_field"`

	// Which Argo CD instance it's hosted on
	Engine_instance_inst_id string `pg:"engine_instance_inst_id"`

	// Which managed environment it is targetting
	// Foreign key to ManagedEnvironment.Managedenvironment_id
	Managed_environment_id string `pg:"managed_environment_id"`

	SeqID int64 `pg:"seq_id"`
}

// ApplicationState is the Argo CD health/sync state of the Application
type ApplicationState struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"applicationstate"` //nolint

	// -- Foreign key to Application.application_id
	Applicationstate_application_id string `pg:"applicationstate_application_id,pk"`

	// -- Possible values:
	// -- * Healthy
	// -- * Progressing
	// -- * Degraded
	// -- * Suspended
	// -- * Missing
	// -- * Unknown
	Health string `pg:"health"`

	// -- Possible values:
	// -- * Synced
	// -- * OutOfSync
	// -- * Unknown
	Sync_Status string `pg:"sync_status"`

	Message string `pg:"message"`

	Revision string `pg:"revision"`

	Resources []byte `pg:"resources"`

	// -- human_readable_health ( 512 ) NOT NULL,
	// -- human_readable_sync ( 512 ) NOT NULL,
	// -- human_readable_state ( 512 ) NOT NULL,

}

// Represents relationship from GitOpsDeployment CR in the namespace, to an Application table row
// This means: if we see a change in a GitOpsDeployment CR, we can easily find the corresponding database entry
// Also: if we see a change to an Argo CD Application, we can easily find the corresponding GitOpsDeployment CR
//
// See for details:
// 'What are the DeploymentToApplicationMapping, KubernetesToDBResourceMapping, and APICRToDatabaseMapping, database tables for?:
// (https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.45brv1rx6wmo)
type DeploymentToApplicationMapping struct {

	// TODO: GITOPSRVCE-67 - DEBT - PK should be (mapping uid, application_id), rather than just mapping uid?

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"deploymenttoapplicationmapping,alias:dta"` //nolint

	// UID of GitOpsDeployment resource in K8s/KCP namespace
	// (value from '.metadata.uid' field of GitOpsDeployment)
	Deploymenttoapplicationmapping_uid_id string `pg:"deploymenttoapplicationmapping_uid_id,pk"`

	// Name of the GitOpsDeployment in the namespace
	DeploymentName string `pg:"name"`

	// Namespace of the GitOpsDeployment
	DeploymentNamespace string `pg:"namespace"`

	// UID (.metadata.uid) of the Namespace, containing the GitOpsDeployments
	// value: (uid of namespace)
	NamespaceUID string `pg:"workspace_uid"`

	// Reference to the corresponding Application row
	// -- Foreign key to: Application.Application_id
	Application_id string `pg:"application_id"`

	SeqID int64 `pg:"seq_id"`
}

const (
	APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun = "GitOpsDeploymentSyncRun"

	APICRToDatabaseMapping_DBRelationType_SyncOperation = "SyncOperation"
)

// Maps API custom resources on the workspace (such as GitOpsDeploymentSyncRun), to a corresponding entry in the database.
// This allows us to quickly go from API CR <-to-> Database entry, and also to identify database entries even when the API CR has been
// deleted from the workspace.
//
// See for details:
// 'What are the DeploymentToApplicationMapping, KubernetesToDBResourceMapping, and APICRToDatabaseMapping, database tables for?:
// (https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.45brv1rx6wmo)
type APICRToDatabaseMapping struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"apicrtodatabasemapping,alias:atdbm"` //nolint

	APIResourceType string `pg:"api_resource_type"`
	APIResourceUID  string `pg:"api_resource_uid"`

	APIResourceName      string `pg:"api_resource_name"`
	APIResourceNamespace string `pg:"api_resource_namespace"`
	WorkspaceUID         string `pg:"api_resource_workspace_uid"`

	DBRelationType string `pg:"db_relation_type"`
	DBRelationKey  string `pg:"db_relation_key"`

	SeqID int64 `pg:"seq_id"`
}

// Represents a generic relationship between Kubernetes CR <-> Database table
// The Kubernetes CR can be either in the workspace, or in/on a GitOpsEngine cluster namespace.
//
// Example: when the cluster agent sees an Argo CD Application CR change within a namespace, it needs a way
// to know which GitOpsEngineInstance database entries corresponds to the Argo CD namespace.
// For this we would use:
// - kubernetes_resource_type: Namespace
// - kubernetes_resource_uid: (uid of namespace)
// - db_relation_type: GitOpsEngineInstance
// - db_relation_key: (primary key of gitops engine instance)
//
// Later, we can query this table to go from 'argo cd instance namespace' <= to => 'GitopsEngineInstance database row'
//
// See DeploymentToApplicationMapping for another example of this.
//
// This is also useful for tracking the lifecycle between CRs <-> database table.
type KubernetesToDBResourceMapping struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"kubernetestodbresourcemapping,alias:ktdbrm"` //nolint

	KubernetesResourceType string `pg:"kubernetes_resource_type,pk"`

	KubernetesResourceUID string `pg:"kubernetes_resource_uid,pk"`

	DBRelationType string `pg:"db_relation_type,pk"`

	DBRelationKey string `pg:"db_relation_key,pk"`

	SeqID int64 `pg:"seq_id"`
}

// Sync Operation tracks a sync request from the API. This will correspond to a sync operation on an Argo CD Application, which
// will cause Argo CD to deploy the K8s resources from Git, to the target environment. This is also known as manual sync.
type SyncOperation struct {

	//lint:ignore U1000 used by go-pg
	tableName struct{} `pg:"syncoperation,alias:so"` //nolint

	SyncOperation_id string `pg:"syncoperation_id,pk"`

	Application_id string `pg:"application_id"`

	Operation_id string `pg:"operation_id"`

	DeploymentNameField string `pg:"deployment_name"`

	Revision string `pg:"revision"`

	DesiredState string `pg:"desired_state"`
}

// TODO: GITOPSRVCE-67 - DEBT - Add comment.
type DisposableResource interface {
	Dispose(ctx context.Context, dbq DatabaseQueries) error
}

type AppScopedDisposableResource interface {
	DisposeAppScoped(ctx context.Context, dbq ApplicationScopedQueries) error
}
