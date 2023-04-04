ALTER TABLE ClusterCredentials ADD COLUMN namespaces VARCHAR (4096);
ALTER TABLE ClusterCredentials ADD COLUMN cluster_resources BOOLEAN DEFAULT FALSE;