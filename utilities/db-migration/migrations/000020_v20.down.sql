ALTER TABLE ApplicationState DROP COLUMN argocd_application_status;

ALTER TABLE ApplicationState ADD COLUMN health VARCHAR (30) NOT NULL DEFAULT 'Unknown';
ALTER TABLE ApplicationState ADD COLUMN message VARCHAR (1024);
ALTER TABLE ApplicationState ADD COLUMN revision VARCHAR (1024);
ALTER TABLE ApplicationState ADD COLUMN sync_status VARCHAR (30) NOT NULL DEFAULT 'Unknown';
ALTER TABLE ApplicationState ADD COLUMN resources bytea;
ALTER TABLE ApplicationState ADD COLUMN reconciled_state VARCHAR ( 4096 );
ALTER TABLE ApplicationState ADD COLUMN operation_state bytea;
ALTER TABLE ApplicationState ADD COLUMN conditions bytea;