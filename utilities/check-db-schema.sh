#!/usr/bin/env bash

# Stop exiting and return non-zero error code if an error occurs
set -ue
export POSTGRESQL_DATABASE="postgres"
SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

REPO_ROOT="$SCRIPTPATH/.."

# Checks if a binary is present on the local system
exit_if_binary_not_installed() {
    for binary in "$@"; do
        command -v "$binary" >/dev/null 2>&1 || {
            echo >&2 "Script requires '$binary' command-line utility to be installed on your local machine. Aborting..."
            exit 1
        }
    done
}

cd "$REPO_ROOT/utilities/db-migration"
go mod download
echo "Start PostgreSQL"

cd "$REPO_ROOT"
USE_MASTER_SCHEMA="true" ./create-dev-env.sh
USE_MASTER_SCHEMA="true" make reset-db

MASTER_SQL_FILE="$(mktemp)"
echo "MASTER_SQL_FILE: $MASTER_SQL_FILE"

MIGRATION_SQL_FILE="$(mktemp)"
echo "MIGRATION_SQL_FILE: $MIGRATION_SQL_FILE"

exit_if_binary_not_installed "pg_dump"

echo "Create a pg_dump file for master schema"
pg_dump postgresql://postgres:gitops@localhost:5432/$POSTGRESQL_DATABASE > "$MASTER_SQL_FILE" && cat "$MASTER_SQL_FILE"

echo "Clean the state of db."
make db-drop

echo "Apply migrations to the db."
make db-migrate

echo "Remove the schema_migration table."
make db-drop_smtable

echo "Create a pg_dump file for migration schema"
pg_dump postgresql://postgres:gitops@localhost:5432/$POSTGRESQL_DATABASE > "$MIGRATION_SQL_FILE" && cat "$MIGRATION_SQL_FILE"

echo "Execute diff to check for any discrepencies"
diff -Nur "$MIGRATION_SQL_FILE" "$MASTER_SQL_FILE"
