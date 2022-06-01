
-- ClusterCredentials contains the credentials required to access a K8s cluster. 
-- The credentials may be in one of two forms:
-- 1) Kubeconfig state: Kubeconfig file, plus a reference to a specific context within the
--     - This is the same content as can be found in your local '~/.kube/config' file
--     - This is what the user would initially provide via the Service/Web UI/CLI
--     - There may be (likely is) a better way of doing this, but this works for now.
-- 2) ServiceAccount state: A bearer token for a service account on the target cluster
--     - Same mechanism Argo CD users for accessing remote clusters
--
-- You can tell which state the credentials are in, based on whether 'serviceaccount_bearer_token' is null. 
--
-- It is the job of the cluster agent to convert state 1 (kubeconfig) into a service account 
-- bearer token on the target cluster (state 2).
--     - This is the same operation as the `argocd cluster add` command, and is the same
--       technique used by Argo CD to interface with remove clusters.
--     - See https://github.com/argoproj/argo-cd/blob/a894d4b128c724129752bac9971c903ab6c650ba/cmd/argocd/commands/cluster.go#L116
CREATE TABLE ClusterCredentials (
	
	-- Primary key for the credentials (UID), is a random UUID
	clustercredentials_cred_id VARCHAR (48) UNIQUE PRIMARY KEY,

	-- API URL for the cluster 
	-- Example: https://api.ci-ln-dlfw0qk-f76d1.origin-ci-int-gce.dev.openshift.com:6443
	host VARCHAR (512),

	-- State 1) kube_config containing a token to a service account that has the permissions we need.
	kube_config VARCHAR (65000),

	-- State 1) The name of a context within the kube_config 
	kube_config_context VARCHAR (64),

	-- State 2) ServiceAccount bearer token from the target manager cluster
	serviceaccount_bearer_token VARCHAR (128),

	-- State 2) The namespace of the ServiceAccount
	serviceaccount_ns VARCHAR (128),

	seq_id serial
);

-- GitopsEngineCluster
-- A cluster that hosts Argo CD instances
-- Note: I use the term GitOpsEngine to refer to Argo CD, so as not to marry us to Argo CD at the database level.
CREATE TABLE GitopsEngineCluster (

	-- Primary key for the GitopsEngineCluster (UID), is a random UUID
	gitopsenginecluster_id  VARCHAR (48) UNIQUE PRIMARY KEY,
	
	seq_id serial, 

	-- pointer to credentials for the cluster
	-- Foreign key to: ClusterCredentials.clustercredentials_cred_id
	clustercredentials_id VARCHAR (48) NOT NULL,
	CONSTRAINT fk_cluster_credential FOREIGN KEY(clustercredentials_id) REFERENCES ClusterCredentials(clustercredentials_cred_id) ON DELETE NO ACTION ON UPDATE NO ACTION

);

-- GitopsEngineInstance
-- Represents an Argo CD instance on a cluster; the specific cluster is pointed to by the enginecluster field, and the
-- namespace of the Argo CD install is listed here.
CREATE TABLE GitopsEngineInstance (

	-- Primary key for the GitopsEngineInstance (UID), is a random UUID
	gitopsengineinstance_id VARCHAR (48) UNIQUE PRIMARY KEY,

	seq_id serial,

	-- An Argo CD cluster may host multiple Argo CD instances; these fields
	-- indicate which namespace this specific instance lives in (and uid of the namespace)
	namespace_name VARCHAR (48) NOT NULL,
	namespace_uid VARCHAR (48) NOT NULL,

	-- Reference to the Argo CD cluster containing the instance
	-- Foreign key to: GitopsEngineCluster.gitopsenginecluster_id
	enginecluster_id VARCHAR(48) NOT NULL,
	CONSTRAINT fk_gitopsengine_cluster FOREIGN KEY (enginecluster_id) REFERENCES GitopsEngineCluster(gitopsenginecluster_id) ON DELETE NO ACTION ON UPDATE NO ACTION
	
);


-- ManagedEnvironment
-- An environment (cluster) that the user wants to deploy applications to, using Argo CD
CREATE TABLE ManagedEnvironment (

	-- Primary key for the ManagedEnvironment (UID), is a random UUID
	managedenvironment_id VARCHAR (48) UNIQUE PRIMARY KEY,
	seq_id serial, 
	
	-- human readable name
	name VARCHAR ( 256 ) NOT NULL,

	-- pointer to credentials for the cluster
	-- Foreign key to: ClusterCredentials.clustercredentials_cred_id
	clustercredentials_id VARCHAR (48) NOT NULL,
	CONSTRAINT fk_cluster_credential FOREIGN KEY (clustercredentials_id) REFERENCES ClusterCredentials(clustercredentials_cred_id) ON DELETE NO ACTION ON UPDATE NO ACTION
);


-- ClusterUser
-- An individual user/customer
--
-- Note: This is basically placeholder: a real implementation would need to be way more complex.
CREATE TABLE ClusterUser (
	
	-- Primary key for the ClusterUser (UID), is a random UUID
	clusteruser_id VARCHAR (48) PRIMARY KEY,

	-- A generic string representing a user; this likely need to be replaced with a field
	-- more consistent with the user configuration we are operating within.
	user_name VARCHAR (256) NOT NULL UNIQUE,

	seq_id serial
);



-- ClusterAccess
--
-- This table answers the questions:
-- - What managed clusters does a user have?
-- - What argo cd instance is managing those clusters?
CREATE TABLE ClusterAccess (

	-- Describes whose cluster this is (UID)
	-- Foreign key to: ClusterUser.clusteruser_id
	clusteraccess_user_id VARCHAR (48),
	CONSTRAINT fk_clusteruser_id FOREIGN KEY (clusteraccess_user_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Describes which managed environment the user has access to (UID)
	-- Foreign key to: ManagedEnvironment.managedenvironment_id
	clusteraccess_managed_environment_id VARCHAR (48),
	CONSTRAINT fk_managedenvironment_id FOREIGN KEY (clusteraccess_managed_environment_id) REFERENCES ManagedEnvironment(managedenvironment_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Which Argo CD instance is managing the cluster?
	-- Foreign key to: GitopsEngineInstance.gitopsengineinstance_id
	clusteraccess_gitops_engine_instance_id VARCHAR (48),
	CONSTRAINT fk_gitopsengineinstance_id FOREIGN KEY (clusteraccess_gitops_engine_instance_id) REFERENCES GitopsEngineInstance(gitopsengineinstance_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
	
	seq_id serial,
	
	-- All fields of the cluster are part of the primary key (not seq_uid), and in addition we have indexes
	-- below for quickly locating a subset of the table.
	PRIMARY KEY(clusteraccess_user_id, clusteraccess_managed_environment_id, clusteraccess_gitops_engine_instance_id)
);
-- Add an index on user_id+managed_cluster, and userid+gitops_manager_instance_Id
CREATE INDEX idx_userid_cluster ON ClusterAccess(clusteraccess_user_id, clusteraccess_managed_environment_id);
CREATE INDEX idx_userid_instance ON ClusterAccess(clusteraccess_user_id, clusteraccess_gitops_engine_instance_id);



-- Operation
-- Operations are used by the backend to communicate database changes to the cluster-agent.
-- It is the reponsibility of the cluster agent to respond to operations, to read the database
-- to discover what database changes occurred, and to ensure that Argo CD is consistent with
-- the database state.
-- 
-- See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.9tzaobsoav27
-- for description of Operation
CREATE TABLE Operation (
	
	-- Primary key for the Operation (UID), is a random UUID
	operation_id  VARCHAR (48) PRIMARY KEY,

	seq_id serial,

	-- Specifies which Argo CD instance is this operation against
	-- Foreign key to: GitopsEngineInstance.gitopsengineinstance_id
	instance_id VARCHAR(48) NOT NULL,
	CONSTRAINT fk_gitopsengineinstance_id FOREIGN KEY (instance_id) REFERENCES GitopsEngineInstance(gitopsengineinstance_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- ID of the database resource that was modified (usually this field contains a primary key of another database table )
	resource_id VARCHAR(48) NOT NULL,

	-- The user that initiated the operation.
	-- Foreign key to: ClusterUser.clusteruser_id
	operation_owner_user_id VARCHAR(48),
	CONSTRAINT fk_clusteruser_id FOREIGN KEY (operation_owner_user_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Resource type of the resource that was modified
	-- This value lets the operation know which table contains the resource.
	-- 
	-- possible values:
	-- * ManagedEnvironment (specified when we want Argo CD to C/R/U/D a user's cluster credentials)
	-- * GitopsEngineInstance (specified to CRUD an Argo instance, for example to create a new namespace and put Argo CD in it, then signal when it's done)
	-- * Application (user creates a new Application via service/web UI)
	-- * SyncOperation (user wants a GitOps engine sync operation performed)
	resource_type VARCHAR(32) NOT NULL,

	-- When the operation was created. Used for garbage collection, as operations should be short lived.
	created_on TIMESTAMP NOT NULL,

	-- last_state_update is set whenever state changes
	-- (initial value should be equal to created_on)
	last_state_update TIMESTAMP NOT NULL,

	-- possible values:
	-- * Waiting
	-- * In_Progress
	-- * Completed
	-- * Failed
	state VARCHAR ( 30 ) NOT NULL,
	
	-- If there is an error message from the operation, it is passed via this field.
	human_readable_state VARCHAR ( 1024 )

);

-- Application represents an Argo CD Application CR within an Argo CD namespace.
CREATE TABLE Application (
	application_id VARCHAR ( 48 ) NOT NULL UNIQUE PRIMARY KEY,

	-- Name of the Application CR within the namespace
	name VARCHAR ( 256 ) NOT NULL,

	-- resource_uid VARCHAR ( 48 ) NOT NULL UNIQUE,

	-- '.spec' field of the Application CR
	-- Note: Rather than converting individual JSON fields into SQL Table fields, we just pull the whole spec field. 
	-- In the future, it might be beneficial to pull out SOME of the fields, to reduce CPU time spent on json parsing
	spec_field VARCHAR ( 16384 ) NOT NULL,

	-- Which Argo CD instance it's hosted on
	-- Foreign key to: GitopsEngineInstance.gitopsengineinstance_id
	engine_instance_inst_id VARCHAR(48) NOT NULL,
	CONSTRAINT fk_gitopsengineinstance_id FOREIGN KEY (engine_instance_inst_id) REFERENCES GitopsEngineInstance(gitopsengineinstance_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Which managed environment it is targetting
	-- Foreign key to: ManagedEnvironment.managedenvironment_id
	managed_environment_id VARCHAR(48) NOT NULL,
	CONSTRAINT fk_managedenvironment_id FOREIGN KEY (managed_environment_id) REFERENCES ManagedEnvironment(managedenvironment_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
	
	seq_id serial

);

-- ApplicationState is the Argo CD health/sync state of the Application
CREATE TABLE ApplicationState (

	-- Foreign key to Application.application_id
	applicationstate_application_id  VARCHAR ( 48 ) PRIMARY KEY,
	CONSTRAINT fk_app_id FOREIGN KEY (applicationstate_application_id) REFERENCES Application(application_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- health field comes directly from Argo CD Application CR's .status.health field
	-- Possible values:
	-- * Healthy
	-- * Progressing
	-- * Degraded
	-- * Suspended
	-- * Missing
	-- * Unknown (this is returned by Argo CD, but is also used when Argo CD's health field is "")
	health VARCHAR (30) NOT NULL,

	-- message field comes directly from Argo CD Application CR's .status.healthStatus.Message
	message VARCHAR (1024),

    -- revision field comes directly from Argo CD Application CR's .status.SyncStatus.Revision field
	revision VARCHAR (1024),

	-- sync_status field comes directly from Argo CD Application CR's .status.SyncStatus field
	-- Possible values:
	-- * Synced
	-- * OutOfSync
	-- * Unknown (this is used when Argo CD's status field is "")
	sync_status VARCHAR (30) NOT NULL,

	-- resources field comes directly from Argo CD Application CR's .Status.Resources field
	resources bytea
);

-- Represents the relationship from GitOpsDeployment CR in the API namespace, to an Application table row.
-- This means: if we see a change in a GitOpsDeployment CR, we can easily find the corresponding database entry
-- by looking for a DeploymentToApplicationMapping that captures the relationship (and vice versa)
--
-- See for details:
-- 'What are the DeploymentToApplicationMapping, KubernetesToDBResourceMapping, and APICRToDatabaseMapping, database tables for?:
-- (https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.45brv1rx6wmo)
CREATE TABLE DeploymentToApplicationMapping (
	
	-- uid of our gitops deployment CR within the K8s namespace (or KCP control plane)
	deploymenttoapplicationmapping_uid_id VARCHAR(48) UNIQUE NOT NULL PRIMARY KEY,
	
	-- name of the GitOpsDeployment CR in the API namespace
	name VARCHAR ( 256 ),
	-- name of the API namespace
	namespace VARCHAR ( 96 ),
	-- uid of the API namespace 
	namespace_uid VARCHAR ( 48 ), 

	-- The Application DB entry that corresponds to this GitOpsDeployment CR
	-- (For example, deleting the GitOpsDeployment CR should delete the corresponding Application DB table, and likewise
	-- should delete this table's entry)
	-- Foreign key to: Application.application_id
	application_id VARCHAR ( 48 ) NOT NULL UNIQUE,
	CONSTRAINT fk_app_id FOREIGN KEY (application_id) REFERENCES Application(application_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	seq_id serial

);

-- Represents a generic relationship between: Kubernetes CR <->  Database table
-- The Kubernetes CR can be either in the API namespace, or in/on a GitOpsEngine cluster namespace.
--
-- Example: when the cluster agent sees an Argo CD Application CR change within a namespace, it needs a way
-- to know which GitOpsEngineInstance database entries corresponds to the Argo CD namespace.
-- 
-- For this we would use:
-- - kubernetes_resource_type: Namespace
-- - kubernetes_resource_uid: (uid of namespace)
-- - db_relation_type: GitOpsEngineInstance
-- - db_relation_key: (primary key of gitops engine instance)
--
-- Later, we can query this table to determine 'Argo CD instance namespace' <=> 'GitopsEngineInstance database row'
--
-- This is also useful for tracking the lifecycle between CRs <-> database table.
--
-- See also the FAQ of the internal architeture doc for info on what this database table ise used for:
-- https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.3jwssie7xjn2
CREATE TABLE KubernetesToDBResourceMapping  (

	-- kubernetes CR resource (example: Namespace)
	-- As of this writing (Jan 2022), Namespace is the only supported value for this field.
	-- - See 'kubernetesresourcetodbresousemapping.go' for latest list.
	kubernetes_resource_type VARCHAR(64) NOT NULL,

	-- UID of the CR (from the .metadata.UID field)
	kubernetes_resource_uid  VARCHAR(64) NOT NULL,

	-- name of database table
	-- As of this writing (Jan 2022), ManagedEnvironment, GitopsEngineCluster, and GitopsEngineInstance are the only supported values for this field.
	-- - See 'kubernetesresourcetodbresousemapping.go' for latest list.
	db_relation_type  VARCHAR(64) NOT NULL,

	-- primary key of database table
	db_relation_key  VARCHAR(64) NOT NULL,

	seq_id serial,

	PRIMARY KEY(kubernetes_resource_type, kubernetes_resource_uid, db_relation_type, db_relation_key)

);

CREATE INDEX idx_db_relation_uid ON KubernetesToDBResourceMapping(kubernetes_resource_type, kubernetes_resource_uid, db_relation_type);
-- Used by: GetDBResourceMappingForKubernetesResource

-- Maps API custom resources in an API namespace (such as GitOpsDeploymentSyncRun), to a corresponding entry in the database.
-- This allows us to quickly go from API CR <-to-> Database entry, and also to identify database entries even when the API CR has been
-- deleted from the namespace/workspace.
--
-- See for details:
-- 'What are the DeploymentToApplicationMapping, KubernetesToDBResourceMapping, and APICRToDatabaseMapping, database tables for?:
-- (https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.45brv1rx6wmo)
CREATE TABLE APICRToDatabaseMapping  (

	-- The custom resource type of the K8S custom resource being referenced in the mapping
	-- As of this writing (Jan 2022), the only supported value for this field is GitOpsDeploymentSyncRun.
	-- See APICRToDatabaseMapping_ResourceType_* constants for latest list.
	api_resource_type VARCHAR(64) NOT NULL,
	
	-- The UID (from .metadata.uid field) of the K8s resource
	api_resource_uid VARCHAR(64) NOT NULL,
	
	-- The name of the k8s resource (from .metadata.name field) of the K8s resource
	api_resource_name VARCHAR(256) NOT NULL,

	-- The namespace containing the k8s resource
	api_resource_namespace VARCHAR(256) NOT NULL,
	
	-- The UID (from .metadata.uid field) of the namespace containing the k8s resource
	api_resource_namespace_uid VARCHAR(64) NOT NULL,

	-- The name of the database table being referenced. 
	-- As of this writing (Jan 2022), the only supported value for this field is SyncOperation.
	-- See APICRToDatabaseMapping_DBRelationType_ constants for latest list.
	db_relation_type VARCHAR(32) NOT NULL,

	-- The primary key of the row in the database table being referenced.
	db_relation_key VARCHAR(64) NOT NULL,

	seq_id serial,

	PRIMARY KEY(api_resource_type, api_resource_uid, db_relation_type, db_relation_key)

);
-- TODO: GITOPSRVCE-68 - PERF - Add index to APICRToDatabaseMapping to correspond to the access patterns we are using.

-- Sync Operation tracks a sync request from the API. This will correspond to a sync operation on an Argo CD Application, which 
-- will cause Argo CD to deploy the K8s resources from Git, to the target environment. This is also known as manual sync.
CREATE TABLE SyncOperation (

	-- Primary key for the SyncOperation (UID), is a random UUID
	syncoperation_id  VARCHAR(48) NOT NULL PRIMARY KEY,

	-- The target Application that is being synchronized
	-- Foreign key to: Application.application_id
	application_id VARCHAR(48),
	CONSTRAINT fk_so_app_id FOREIGN KEY (application_id) REFERENCES Application(application_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- The 'gitopsDeploymentName' field of the GitOpsDeploymentSyncRun CR
	deployment_name VARCHAR(256) NOT NULL,

	-- The 'revisionID'  field of the GitOpsDeploymentSyncRun CR
	revision VARCHAR(256) NOT NULL,

	-- Whether we want the SyncOperation to continue running, or to be terminated.
	-- values: Running, Terminated
	desired_state VARCHAR(16) NOT NULL,	

	seq_id serial

);

/*
-------------------------------------------------------------------------------

Schema Design Guidelines:

- All primary keys (PKs) should include the name of the table, in the field name.
    - Example:
	    - initial field name: infcluster_id
		- table: GitopsEngineCluster
		- field name is thus: gitopsenginecluster_infcluster_id  
		    - (eg [table]_[initial file name])
	- Why? This makes it easy to track usage of the PK, and refactor at a later date. 
    - Foreign keys (FKs) to PKs do NOT need to include the table name

- No other tables should use a field with the same name as the PK of another table.
	- Why? This makes it easy to track usage of the PK, and refactor at a later date. 

- Use UUIDs for table PK, rather than seqids:
	- Why? Unlike seqids:
	    - UUIDS do not leak information on # of users
		- UUIDs are not vulnerable to increment by one attacks
		- Allows the contents of two RDBMS databases to be easily merged (no need to fix all the seqids)
		- Give flexibility over underlying database store (Other RDBMS-es, other DB technologies)
		- Prevents accidental erasure of table rows (collisions between uuids is much less likely than between integers)

- Name case:
    - For tables: CamelCase
    - For fields: lowercase_snake_case

-------------------------------------------------------------------------------


Foreign key relationships between tables, as of this writing:
(Tables entries must be deleted in this order, from top to bottom, and created in reverse order)

Miro Diagram: https://miro.com/app/board/o9J_lgiqJAs=/?moveToWidget=3458764513858646837&cot=14

SyncOperation -> Operation
SyncOperation -> Application

ApplicationState ->  Application

DeploymentToApplicationMapping -> Application

Operation -> ClusterUser
Operation -> GitopsEngineInstance

Application -> ManagedEnviroment
Application ->  GitopsEngineInstance

ClusterAccess -> ClusterUser
ClusterAccess -> ManagedEnvironment
ClusterAccess -> GitopsEngineInstance

GitopsEngineInstance -> GitopsEngineCluster

GitopsEngineCluster -> ClusterCredentials
ManagedEnvironment -> ClusterCredentials


ClusterCredentials -> .

ClusterUser -> .

KubernetesToDBResourceMapping -> .


-------------------------------------------------------------------------------

Extra thoughts:

Notes:

seq_id is for debugging purposes only, and should not be used as a key


-------------------------------------------------------------------------------
*/
