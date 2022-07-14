
ALTER TABLE Application ALTER COLUMN managed_environment_id DROP NOT NULL;

ALTER TABLE ClusterCredentials ALTER COLUMN serviceaccount_bearer_token type VARCHAR (2048);
