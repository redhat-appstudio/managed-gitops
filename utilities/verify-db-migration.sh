#!/usr/bin/env bash

# Stop exiting and return non-zero error code if an error occurs
set -ue

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
USE_MASTER_SCHEMA="false" ./create-dev-env.sh
make db-drop
make db-migrate

MASTER_SQL_FILE="$(mktemp)"
echo "MASTER_SQL_FILE: $MASTER_SQL_FILE"

MIGRATION_SQL_FILE="$(mktemp)"
echo "MIGRATION_SQL_FILE: $MIGRATION_SQL_FILE"

exit_if_binary_not_installed "pg_dump"

export POSTGRESQL_DATABASE="postgres"
echo "Create a pg_dump file for master schema"
pg_dump postgresql://postgres:gitops@localhost:5432/$POSTGRESQL_DATABASE > "$MASTER_SQL_FILE" && cat "$MASTER_SQL_FILE"

echo "Check if downgrading and upgrading by a single version leads to same schema."
make db-migrate-downgrade
make db-migrate-upgrade
pg_dump postgresql://postgres:gitops@localhost:5432/$POSTGRESQL_DATABASE > "$MIGRATION_SQL_FILE" && cat "$MIGRATION_SQL_FILE"
diff -Nur "$MIGRATION_SQL_FILE" "$MASTER_SQL_FILE"

echo "Check if downgrading and upgrading by a two versions leads to same schema."
make db-migrate-downgrade
make db-migrate-downgrade
make db-migrate-upgrade
make db-migrate-upgrade
pg_dump postgresql://postgres:gitops@localhost:5432/$POSTGRESQL_DATABASE > "$MIGRATION_SQL_FILE" && cat "$MIGRATION_SQL_FILE"
diff -Nur "$MIGRATION_SQL_FILE" "$MASTER_SQL_FILE"