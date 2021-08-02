


-- InfrastructureCluster
CREATE TABLE InfrastructureCluster (
	inf_cluster_id serial PRIMARY KEY,
	kube_config VARCHAR (65000) NOT NULL,
);

-- ManagedCluster
CREATE TABLE ManagedCluster (
	managed_cluster_id serial PRIMARY KEY,
	name VARCHAR ( 256 ) UNIQUE NOT NULL,
	kube_config VARCHAR (65000) NOT NULL,
);


-- ClusterUser
CREATE TABLE ClusterUser (
	user_id serial PRIMARY KEY,
	user_name VARCHAR (256) NOT NULL,	
);


-- ClusterAccess
CREATE TABLE ClusterAccess (
	CONSTRAINT fk_cluster_access_target_inf_cluster   FOREIGN KEY(cluster_access_target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),
	CONSTRAINT fk_cluster_access_user_id   FOREIGN KEY(cluster_access_user_id)  REFERENCES ClusterUser(user_id),
	
	PRIMARY KEY(cluster_access_user_id, cluster_access_target_inf_cluster)
);



------------------------------------------------------


-- Operation
CREATE TABLE Operation (
	operation_id  serial PRIMARY KEY,
	operation_uuid  VARCHAR (48) UNIQUE NOT NULL,

	CONSTRAINT fk_target_inf_cluster   FOREIGN KEY(target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),

	created_on TIMESTAMP NOT NULL,
	last_state_update TIMESTAMP NOT NULL,
	-- in_progress, completed, failed
	state VARCHAR ( 30 ) NOT NULL,
	human_readable_state ( 512 ) NOT NULL,
);


-- Application
CREATE TABLE Application (
	appl_id serial PRIMARY KEY,
	name VARCHAR ( 256 ) NOT NULL,
	resource_uid VARCHAR ( 48 ) NOT NULL UNIQUE,

	spec_field VARCHAR ( 16384 ) NOT NULL,

	-- TODO: This should be indexed
	CONSTRAINT fk_target_inf_cluster   FOREIGN KEY(target_inf_cluster)  REFERENCES InfrastructureCluster(inf_cluster_id),
	
);

-- ApplicationState
CREATE TABLE ApplicationState (

	-- TODO: This should be a primary key.
	CONSTRAINT fk_app_id  PRIMARY KEY  FOREIGN KEY(app_id)  REFERENCES Application(appl_id),

	state VARCHAR ( 30 ) NOT NULL,
	human_readable_health ( 512 ) NOT NULL,
	human_readable_sync ( 512 ) NOT NULL,
	human_readable_state ( 512 ) NOT NULL,	

)







/*
# Application

# Operation
target infrastructure cluster
uuid
create/update/delete
repository/application/managed cluster
name




CREATE TABLE accounts (
	user_id serial PRIMARY KEY,
	username VARCHAR ( 50 ) UNIQUE NOT NULL,
	password VARCHAR ( 50 ) NOT NULL,
	email VARCHAR ( 255 ) UNIQUE NOT NULL,
	created_on TIMESTAMP NOT NULL,
        last_login TIMESTAMP 
);


*/
