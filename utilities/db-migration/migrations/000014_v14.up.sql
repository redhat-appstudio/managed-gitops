-- AppProjectRepository is used by ArgoCD AppProject
CREATE TABLE AppProjectRepository (

	-- Primary Key, that is an auto-generated UID
	appproject_repository_id VARCHAR(48) NOT NULL PRIMARY KEY,

	-- Describes whose cluster this is (UID)
	-- Foreign key to: ClusterUser.clusteruser_id
	clusteruser_id VARCHAR (48) NOT NULL,
	CONSTRAINT fk_clusteruser_id FOREIGN KEY (clusteruser_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Describes which repositorycredentials the user has access to (UID)
	-- Foreign key to: RepositoryCredentials.repositorycredentials_id
	repositorycredentials_id VARCHAR ( 48 ),
	CONSTRAINT fk_repositorycredentials_id FOREIGN KEY (repositorycredentials_id) REFERENCES RepositoryCredentials(repositorycredentials_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
	
	-- Normalized Repo URL
	repo_url VARCHAR (256) NOT NULL,

	UNIQUE (clusteruser_id, repo_url),

	created_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

	seq_id serial
);
-- Add an index on clusteruser_id
CREATE INDEX idx_userid_cluster_rc ON AppProjectRepository(clusteruser_id);

-- AppProjectManagedEnvironment is used by ArgoCD AppProject
CREATE TABLE AppProjectManagedEnvironment (

	-- Primary Key, that is an auto-generated UID
	appproject_managedenv_id VARCHAR(48) NOT NULL PRIMARY KEY,

	-- Describes whose cluster this is (UID)
	-- Foreign key to: ClusterUser.clusteruser_id
	clusteruser_id VARCHAR (48) NOT NULL,
	CONSTRAINT fk_clusteruser_id FOREIGN KEY (clusteruser_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

	-- Describes which managedenvironment the user has access to (UID)
	-- Foreign key to: ManagedEnvironment.managed_environment_id
	managed_environment_id VARCHAR (48) NOT NULL,
	CONSTRAINT fk_managedenvironment_id FOREIGN KEY (managed_environment_id) REFERENCES ManagedEnvironment(managedenvironment_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
	
	created_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	
	seq_id serial
);
-- Add an index on clusteruser_id
CREATE INDEX idx_userid_cluster_me ON AppProjectManagedEnvironment(clusteruser_id);