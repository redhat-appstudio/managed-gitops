ALTER TABLE SyncOperation ADD COLUMN operation_id VARCHAR(48) NOT NULL DEFAULT 0;

ALTER TABLE ApplicationState DROP COLUMN resources;

ALTER TABLE APICRToDatabaseMapping RENAME COLUMN api_resource_namespace_uid TO api_resource_workspace_uid;

ALTER TABLE DeploymentToApplicationMapping RENAME COLUMN namespace_uid TO workspace_uid;
