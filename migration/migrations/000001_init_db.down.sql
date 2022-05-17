BEGIN;


DROP TABLE IF EXISTS ApplicationState;
DROP TABLE IF EXISTS DeploymentToApplicationMapping;
DROP TABLE IF EXISTS KubernetesToDBResourceMapping;
DROP TABLE IF EXISTS APICRToDatabaseMapping;
DROP TABLE IF EXISTS SyncOperation;
DROP TABLE IF EXISTS Application;
DROP TABLE IF EXISTS Operation;
DROP TABLE IF EXISTS ClusterAccess;
DROP TABLE IF EXISTS GitopsEngineInstance;
DROP TABLE IF EXISTS GitopsEngineCluster;
DROP TABLE IF EXISTS ManagedEnvironment;
DROP TABLE IF EXISTS ClusterCredentials;
DROP TABLE IF EXISTS ClusterUser;




COMMIT;