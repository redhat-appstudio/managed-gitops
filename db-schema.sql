
-- A cluster that hosts Argo CD instances
-- Note: I use the term GitOpsEngine to refer to Argo CD, so as not to marry us to Argo CD at the database level.
CREATE TABLE GitopsEngineCluster (
	gitopsenginecluster_infcluster_id  VARCHAR (48) UNIQUE PRIMARY KEY,
	
	seq_id serial, 

	-- pointer to credentials for the cluster
	-- Foreign key to: ClusterCredentials.clustercredentials_cred_id
	clustercredentials_id VARCHAR (48) NOT NULL
	-- TODO: Make this a foreign key

);

-- Argo CD instance on a Argo CD cluster
CREATE TABLE GitopsEngineInstance (
	gitopsengineinstance_id VARCHAR (48) UNIQUE PRIMARY KEY,
	seq_id serial,

	-- An Argo CD cluster may host multiple Argo CD instances; these fields
	-- indicate which namespace this specific instance lives in.
	namespace_name VARCHAR (48) UNIQUE NOT NULL,
	namespace_uid VARCHAR (48) UNIQUE NOT NULL,

	-- Reference to the Argo CD cluster containing the instance
	-- Foreign key to: GitopsEngineCluster.gitopsenginecluster_infcluster_id	
	infcluster_id VARCHAR(48) UNIQUE NOT NULL
	-- TODO: Add an index, FK on infcluster_id

);


-- ManagedEnvironment
-- An environment (namespace(s) on a user's cluster) that they want to deploy applications to, using Argo CD
CREATE TABLE ManagedEnvironment (
	managedenvironment_id VARCHAR (48) UNIQUE PRIMARY KEY,
	seq_id serial, 
	
	-- human readable name
	name VARCHAR ( 256 ) UNIQUE NOT NULL,

	-- pointer to credentials for the cluster
	-- Foreign key to: ClusterCredentials.clustercredentials_cred_id
	clustercredentials_id VARCHAR (48) NOT NULL
);

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
	
	-- Primary key for the credentials (UID)
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


-- ClusterUser
-- An individual user/customer
--
-- Note: This is basically placeholder: a real implementation would need to be way more complex.
CREATE TABLE ClusterUser (
	cluster_user_id VARCHAR (48) PRIMARY KEY,
	user_name VARCHAR (256) NOT NULL,	
	seq_id serial
);




-- ClusterAccess
-- This table answers the questions:
-- - What managed clusters does a user have?
-- - What argo cd instance is managing those clusters?
CREATE TABLE ClusterAccess (

	-- Describes whose cluster this is (UID)
	clusteraccess_user_id VARCHAR (48) UNIQUE,

	-- Describes which managed environment the user has access to (UID)
	clusteraccess_managed_environment_id VARCHAR (48) UNIQUE,

	-- Which Argo CD instance is managing the cluster?
	clusteraccess_gitops_engine_instance_id VARCHAR (48) UNIQUE,

	-- TODO: Make these foreign keys
	-- TODO: Add an index on user_id+managed_cluster, and userid+gitops_manager_instance_Id

	-- CONSTRAINT fk_cluster_access_target_inf_cluster   FOREIGN KEY(cluster_access_target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),
	-- CONSTRAINT fk_cluster_access_user_id   FOREIGN KEY(cluster_access_user_id)  REFERENCES ClusterUser(user_id),
	
	seq_id serial,
	
	PRIMARY KEY(clusteraccess_user_id, clusteraccess_managed_environment_id, clusteraccess_gitops_engine_instance_id)
);



-- Operation
-- See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.9tzaobsoav27
-- for description of Operation
CREATE TABLE Operation (
	
	-- UID
	operation_id  VARCHAR (48) PRIMARY KEY,

	seq_id serial,

	-- TODO: Make gitops_manager_instance_id an FK
	-- Specifies which Argo CD instance is this operation against
	-- Foreign key to: GitopsEngineInstance.gitopsengineinstance_id
	instance_id VARCHAR(48) UNIQUE NOT NULL,

	-- CONSTRAINT fk_target_inf_cluster   FOREIGN KEY(target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),

	-- UID of the resource that was updated
	resource_id VARCHAR(48) NOT NULL,

	-- Resource type of the the resoutce that was updated. 
	-- This value lets the operation know which table contains the resource.
	-- 
	-- possible values:
	-- * ManagedEnvironment (specified when we want Argo CD to C/R/U/D a user's cluster credentials)
	-- * GitopsEngineInstance (specified to CRUD an Argo instance, for example to create a new namespace and put Argo CD in it, then signal when it's done)
	-- * Application (user creates a new Application via service/web UI)
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
	-- TODO: Better way to do this? 
	state VARCHAR ( 30 ) NOT NULL,
	
	-- If there is an error message from the operation, it is passed via this field.
	human_readable_state VARCHAR ( 1024 )

);


-- Application represents an Argo CD Application CR within an Argo CD namespace.
CREATE TABLE Application (
	application_id VARCHAR ( 48 ) NOT NULL UNIQUE PRIMARY KEY,

	seq_id serial,

	-- Name of the Application CR within the namespace
	name VARCHAR ( 256 ) NOT NULL,

	-- resource_uid VARCHAR ( 48 ) NOT NULL UNIQUE,

	-- '.spec' field of the Application CR
	-- Note: Rather than converting individual JSON fields into SQL Table fields, we just pull the whole spec field. 
	-- In the future, it might be beneficial to pull out SOME of the fields, to reduce CPU time spent on json parsing
	spec_field VARCHAR ( 16384 ) NOT NULL,

	
	-- Which Argo CD instance it's hosted on
	engine_instance_inst_id VARCHAR(48) UNIQUE NOT NULL,
	-- TODO: Make gitopsengine_instance_inst_id an FK

	-- Which managed environment it is targetting
	managed_environment_id VARCHAR(48) UNIQUE NOT NULL
	-- TODO: This should be indexed
	-- CONSTRAINT fk_target_inf_cluster   FOREIGN KEY(target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),
	
);

-- ApplicationState is the Argo CD health/sync state of the Application
-- (Redis may be better suited for this in the future)
CREATE TABLE ApplicationState (

	-- Also a foreign key to Application.application_id
	applicationstate_application_id  VARCHAR ( 48 ) PRIMARY KEY,
	-- TODO: applicationstate_application_id should be an FK
	-- CONSTRAINT fk_app_id  PRIMARY KEY  FOREIGN KEY(app_id)  REFERENCES Application(appl_id),

	-- Possible values:
	-- * Healthy
	-- * Progressing
	-- * Degraded
	-- * Suspended
	-- * Missing
	-- * Unknown
	health VARCHAR (30) NOT NULL,

	-- Possible values:
	-- * Synced
	-- * OutOfSync
	sync_status VARCHAR (30) NOT NULL

	-- human_readable_health ( 512 ) NOT NULL,
	-- human_readable_sync ( 512 ) NOT NULL,
	-- human_readable_state ( 512 ) NOT NULL,	

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

- Name case:
    - For tables: CamelCase
    - For fields: lowercase_snake_case

-------------------------------------------------------------------------------

Extra thoughts:

TODO: Add a field for when a resource was created? 

Notes:

seq_id is for debugging purposes only, and should not be used as a key


-------------------------------------------------------------------------------
*/
