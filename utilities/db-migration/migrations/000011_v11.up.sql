ALTER TABLE ClusterCredentials ADD COLUMN created_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE ClusterUser ADD COLUMN created_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE ClusterAccess ADD COLUMN created_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
