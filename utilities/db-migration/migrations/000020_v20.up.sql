ALTER TABLE ApplicationState DROP COLUMN health, DROP COLUMN sync_status, DROP COLUMN message, DROP COLUMN revision,DROP COLUMN resources,DROP COLUMN operation_state,DROP COLUMN reconciled_state, DROP COLUMN conditions;

ALTER TABLE ApplicationState ADD COLUMN argocd_application_status bytea;