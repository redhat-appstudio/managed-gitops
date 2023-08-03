ALTER TABLE ApplicationState ADD COLUMN sync_error VARCHAR ( 4096 );
ALTER TABLE ApplicationState DROP COLUMN conditions;