-- AppProjectRepository is used by ArgoCD AppProject
CREATE TABLE AppProjectRepository (

    -- Primary Key, that is an auto-generated UID
	app_project_repository_id VARCHAR(48) NOT NULL PRIMARY KEY,

	-- Describes whose cluster this is (UID)
	-- Foreign key to: ClusterUser.clusteruser_id
	cluster_user_id VARCHAR (48),
	CONSTRAINT fk_clusteruser_id FOREIGN KEY (cluster_user_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Describes which repositorycredentials the user has access to (UID)
	-- Foreign key to: RepositoryCredentials.repositorycredentials_id
	repositorycredentials_id VARCHAR (48),
	CONSTRAINT fk_repositorycredentials_id FOREIGN KEY (repositorycredentials_id) REFERENCES RepositoryCredentials(repositorycredentials_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
	
	-- Normalized Repo URL
	repo_url VARCHAR (256) NOT NULL,

	UNIQUE (cluster_user_id, repo_url),

	seq_id serial
);
-- Add an index on cluster_user_id
CREATE INDEX idx_userid_cluster_rc ON AppProjectRepository(cluster_user_id);

-- AppProjectManagedEnvironment is used by ArgoCD AppProject
CREATE TABLE AppProjectManagedEnvironment (

    -- Primary Key, that is an auto-generated UID
	app_project_managedenv_id VARCHAR(48) NOT NULL PRIMARY KEY,

	-- Describes whose cluster this is (UID)
	cluster_user_id VARCHAR (48),

	-- Describes which managedenvironment the user has access to (UID)
	-- Foreign key to: ManagedEnvironment.managed_environment_id
	managed_environment_id VARCHAR (48) NOT NULL,
	CONSTRAINT fk_managedenvironment_id FOREIGN KEY (managed_environment_id) REFERENCES ManagedEnvironment(managedenvironment_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
	
	seq_id serial
);
-- Add an index on cluster_user_id
CREATE INDEX idx_userid_cluster_me ON AppProjectManagedEnvironment(cluster_user_id);