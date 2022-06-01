
ALTER TABLE SyncOperation DROP COLUMN operation_id;

ALTER TABLE ApplicationState ADD COLUMN resources bytea;

ALTER TABLE APICRToDatabaseMapping RENAME COLUMN api_resource_workspace_uid TO api_resource_namespace_uid;

ALTER TABLE DeploymentToApplicationMapping RENAME COLUMN workspace_uid TO namespace_uid;
