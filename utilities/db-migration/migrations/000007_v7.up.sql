ALTER TABLE ApplicationState ADD COLUMN reconciled_state VARCHAR ( 4096 );
ALTER TABLE ApplicationState ADD COLUMN sync_error VARCHAR ( 4096 );
