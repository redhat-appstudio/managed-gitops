#!/usr/bin/env bash

# Tip: More info on psql - https://tomcam.github.io/postgres/

POSTGRES_CONTAINER="managed-gitops-postgres"
if docker ps | grep $POSTGRES_CONTAINER &>/dev/null; then
  docker exec \
    --user postgres \
    -e PGPASSWORD=gitops \
    -it "$POSTGRES_CONTAINER" "psql" \
    -h localhost -d postgres -U postgres -p 5432 "$*"
else
  echo "Error: cannot interact with the PostgreSQL server: '$POSTGRES_CONTAINER' container is not running."
  echo "Run './create-dev-env.sh script first"
  exit 1
fi
