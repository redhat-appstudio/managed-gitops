CREATE TABLE RepositoryCredentials (

    -- Primary Key, that is an auto-generated UID
    repositorycredentials_id VARCHAR ( 48 ) NOT NULL UNIQUE PRIMARY KEY,

    -- User of GitOps service that wants to use a private repository
    -- Foreign key to: ClusterUser.Clusteruser_id
    repo_cred_user_id VARCHAR (48) NOT NULL,
    CONSTRAINT fk_clusteruser_id FOREIGN KEY (repo_cred_user_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

    -- URL of the Git repository (example: https://github.com/my-org/my-repo)
    repo_cred_url VARCHAR (512) NOT NULL,

    -- Authorized username login for accessing the private Git repo
    repo_cred_user VARCHAR (256),

    -- Authorized password login for accessing the private Git repo
    repo_cred_pass VARCHAR (1024),

    -- Alternative authentication method using an authorized private SSH key
    repo_cred_ssh VARCHAR (1024),

    -- The name of the Secret resource in the Argo CD Repository, in the GitOps Engine instance
    repo_cred_secret VARCHAR(48) NOT NULL,

    -- The internal RedHat Managed cluster where the GitOps Engine (e.g. ArgoCD) is running
    -- NOTE: It is expected the 'repo_cred_secret' to be stored there as well.
    -- Foreign key to: GitopsEngineInstance.Gitopsengineinstance_id
    repo_cred_engine_id VARCHAR(48) NOT NULL,
    CONSTRAINT fk_gitopsengineinstance_id FOREIGN KEY (repo_cred_engine_id) REFERENCES GitopsEngineInstance(gitopsengineinstance_id) ON DELETE NO ACTION ON UPDATE NO ACTION,

    seq_id serial

);