#!/usr/bin/env bash

# Tip: More info on psql - https://tomcam.github.io/postgres/


# Run psql in its own disposable container

PGPASSWORD="${POSTGRES_PASSWORD:-gitops}"
POSTGRESQL_DATABASE="${POSTGRESQL_DATABASE:=postgres}"

docker run -e PGPASSWORD=$PGPASSWORD --network=host --rm -it postgres:13 \
  psql -h localhost -d $POSTGRESQL_DATABASE -U postgres -p 5432
