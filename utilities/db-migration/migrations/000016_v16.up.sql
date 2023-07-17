-- ApplicationOwner indicates which Applications are owned by which user(s)
CREATE TABLE ApplicationOwner (

    -- Foreign key to Application.application_id
    application_owner_application_id VARCHAR(48) NOT NULL,
    CONSTRAINT fk_app_id FOREIGN KEY (application_owner_application_id) REFERENCES Application(application_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
    
    -- Describes whose cluster this is (UID)
    -- Foreign key to: ClusterUser.clusteruser_id
    application_owner_user_id VARCHAR(48) NOT NULL,
    CONSTRAINT fk_clusteruser_id FOREIGN KEY (application_owner_user_id) REFERENCES ClusterUser(clusteruser_id) ON DELETE NO ACTION ON UPDATE NO ACTION,
    
    seq_id SERIAL,
    
    -- When ClusterUser was created, which allows us to tell how old the resources are
    created_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    PRIMARY KEY (application_owner_application_id, application_owner_user_id)
);
