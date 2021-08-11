
-- TODO: When a resource was created? 
-- seq_id is for debugging purposes only, and should not be used as a key


-- A cluster that hosts Argo CD instances
-- Note: I use the term GitOpsManager to refer to Argo CD, so as not to marry us to Argo CD at the database level.
CREATE TABLE GitopsManagerCluster (
	infcluster_id  VARCHAR (48) UNIQUE PRIMARY KEY,
	-- inf_cluster_id serial PRIMARY KEY,
	seq_id serial, 

	-- kube_config containing a token to a service account that has the permissions we need.
	-- TODO: We can probably replace this with: a TCP-IP address to the API server, and a SA token. (See how Argo CD handles this)
	kube_config VARCHAR (65000) NOT NULL,

	-- The name of a context within the kube_config 
	-- TODO: We can probably replace this with: a TCP-IP address to the API server, and a SA token (that won't expire)
	kube_config_context VARCHAR (64) NOT NULL
);

-- Argo CD instance on a Argo CD cluster
CREATE TABLE GitopsManagerInstance (
	gitopsmanagerinstance_id VARCHAR (48) UNIQUE PRIMARY KEY,
	seq_id serial,
	namespace_name VARCHAR (48) UNIQUE NOT NULL,
	namespace_uid VARCHAR (48) UNIQUE NOT NULL,

	-- TODO: Add an index on infcluster_id

	-- Reference to the Argo CD cluster containing the namespace/instance
	infcluster_id VARCHAR(48) UNIQUE NOT NULL
);


-- ManagedCluster
-- The user's cluster that they want to deploy applications to (using Argo CD)
CREATE TABLE ManagedCluster (
	managedcluster_id VARCHAR (48) UNIQUE PRIMARY KEY,
	seq_id serial, 
	-- managed_cluster_id serial PRIMARY KEY,
	-- human readable name
	name VARCHAR ( 256 ) UNIQUE NOT NULL,
	kube_config VARCHAR (65000) NOT NULL
);


-- ClusterUser
-- An individual user/customer
CREATE TABLE ClusterUser (
	cluster_user_id VARCHAR (48) PRIMARY KEY,
	user_name VARCHAR (256) NOT NULL,	
	seq_id serial
);




-- ClusterAccess
-- Answers thq question:
-- What managed clusters does a user have?
-- What argo cd instance is managing those clusters?
CREATE TABLE ClusterAccess (

	-- Whose cluster is this?
	clusteraccess_user_id VARCHAR (48) UNIQUE,

	-- Which managed cluster do they own?
	clusteraccess_managed_cluster_id VARCHAR (48) UNIQUE,

	-- Which Argo CD instance is managing the cluster?
	clusteraccess_gitops_manager_instance_id VARCHAR (48) UNIQUE,

	-- TODO: Make these foreign keys
	-- TODO: Add an index on user_id+managed_cluster, and userid+gitops_manager_instance_Id

	-- CONSTRAINT fk_cluster_access_target_inf_cluster   FOREIGN KEY(cluster_access_target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),
	-- CONSTRAINT fk_cluster_access_user_id   FOREIGN KEY(cluster_access_user_id)  REFERENCES ClusterUser(user_id),
	

	PRIMARY KEY(clusteraccess_user_id, clusteraccess_managed_cluster_id, clusteraccess_gitops_manager_instance_id)
);



------------------------------------------------------


-- Operation
-- See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.9tzaobsoav27
-- for description of Operation
CREATE TABLE Operation (
	operation_id  VARCHAR (48) PRIMARY KEY,
	seqid  serial,

	-- TODO: Make gitops_manager_instance_id an FK
	-- Specifies which Argo CD instance is this operation against
	gitops_manager_instance_id VARCHAR(48) UNIQUE NOT NULL,

	-- CONSTRAINT fk_target_inf_cluster   FOREIGN KEY(target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),

	-- UID of the resource that was updated
	resource_id VARCHAR(48) NOT NULL,

	-- Resource type of the the resoutce that was updated. 
	-- This value lets the operation know which table contains the resource.
	-- 
	-- possible values:
	-- ManagedCluster (specified when we want Argo CD to  C/R/U/D a user cluster)
	-- GitopsManagerInstance (specified to CRUD an Argo instance, for example to creat a new namespace and put Argo CD in it, then signal when it's done)
	resource_type VARCHAR(32) NOT NULL,

	created_on TIMESTAMP NOT NULL,

	-- last_state_update is set whenever state changes
	-- (initial value should be equal to created_on)
	last_state_update TIMESTAMP NOT NULL,

	-- possible values:
	-- waiting, in_progress, completed, failed
	-- TODO: Better way to do this? 
	state VARCHAR ( 30 ) NOT NULL,
	
	-- If there is an error message from the operation, it is passed via this field.
	human_readable_state VARCHAR ( 1024 )

);


-- Application represents an Argo CD Application CR within an Argo CD namespace.
CREATE TABLE Application (
	application_id VARCHAR ( 48 ) NOT NULL UNIQUE PRIMARY KEY,

	seqid serial,

	-- Name of the Application CR within the namespace
	name VARCHAR ( 256 ) NOT NULL,

	-- resource_uid VARCHAR ( 48 ) NOT NULL UNIQUE,

	-- '.spec' field of the Application CR
	-- Note: Rather than converting individual JSON fields into SQL Table fields, we just pull the whole spec field. 
	-- In the future, it might be beneficial to pull out SOME of the fields, to reduce CPU time spent on json parsing
	spec_field VARCHAR ( 16384 ) NOT NULL,

	-- TODO: Make gitops_manager_instance_id an FK
	-- Which Argo CD instance it's hosted on
	gitops_manager_instance_id VARCHAR(48) UNIQUE NOT NULL

	-- TODO: What if the user defines their own cluster, not managed by us?
	-- TODO: This should be indexed
	-- CONSTRAINT fk_target_inf_cluster   FOREIGN KEY(target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),
	
);

-- ApplicationState is the Argo CD health/sync state of the Application
CREATE TABLE ApplicationState (

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
	-- state VARCHAR ( 30 ) NOT NULL,
	-- human_readable_health ( 512 ) NOT NULL,
	-- human_readable_sync ( 512 ) NOT NULL,
	-- human_readable_state ( 512 ) NOT NULL,	

);