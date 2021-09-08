#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

# Map the docker data directory into a temporary directory
POSTGRES_DATA_DIR=`mktemp -d -t postgres-XXXXXXXXXX`

echo "* Starting postgresql"

# Add this server to pgadmin using:
# - hostname: managed-gitops-postgres
# - port: 5432
# - username: postgres
# - password: gitops
docker run --name managed-gitops-postgres \
	-v $POSTGRES_DATA_DIR:/var/lib/postgresql/data \
	-e POSTGRES_PASSWORD=gitops	\
	-p 5432:5432 \
	-d \
	postgres:13 \
	-c log_statement='all' \
	-c log_min_duration_statement=0
	
# -c options are from https://pg.uptrace.dev/faq/#how-to-view-queries-this-library-generates	

echo

echo "* Starting pgadmin"

# pgadmin login/password is the email/password below
docker run --link managed-gitops-postgres --name managed-gitops-pgadmin -p 80:80 \
    -e 'PGADMIN_DEFAULT_EMAIL=user@user.com' \
    -e 'PGADMIN_DEFAULT_PASSWORD=gitops' \
    -d dpage/pgadmin4

echo 

RETRIES=30

until "$SCRIPTPATH/psql.sh" -c "select 1" > /dev/null 2>&1 || [ $RETRIES -eq 0 ]; do
  echo "* Waiting for postgres server, $((RETRIES--)) remaining attempts..."
  sleep 1
done
echo

echo "* Initializing DB"
"$SCRIPTPATH/psql.sh" -q -f db-schema.sql
echo

