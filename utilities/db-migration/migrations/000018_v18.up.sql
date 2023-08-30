ALTER TABLE ApplicationState DROP COLUMN sync_error;
ALTER TABLE ApplicationState ADD COLUMN conditions bytea;