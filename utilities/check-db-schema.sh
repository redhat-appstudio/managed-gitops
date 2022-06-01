#!/bin/bash

# Stop exiting and return non-zero error code if an error occurs
set -ue

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

REPO_ROOT="$SCRIPTPATH/.."

cd $REPO_ROOT/utilities/db-migration
go mod download
echo "Start PostgreSQL"


cd $REPO_ROOT
./create-dev-env.sh
make reset-db

MASTER_SQL_FILE=`mktemp`
MIGRATION_SQL_FILE=`mktemp`

echo "Create a pg_dump file for master schema"
pg_dump postgresql://postgres:gitops@localhost:5432/postgres > $MASTER_SQL_FILE && cat $MASTER_SQL_FILE

echo "Clean the state of db."
make db-drop

echo "Apply migrations to the db."
make db-migrate

echo "Remove the schema_migration table."
make db-drop_smtable

echo "Create a pg_dump file for migration schema"
pg_dump postgresql://postgres:gitops@localhost:5432/postgres > $MIGRATION_SQL_FILE && cat $MIGRATION_SQL_FILE

echo "Execute diff to check for any discrepencies"
diff $MIGRATION_SQL_FILE $MASTER_SQL_FILE
