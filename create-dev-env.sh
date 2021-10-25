#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

# Create docker network if one doesn't exist yet
if [ $(docker network ls --filter "name=gitops-net" -q |wc -l) == 0 ]; then
    echo "* Creating docker network 'gitops-net'"
    docker network create gitops-net
	echo 
fi


# Map the docker data directory into a temporary directory
POSTGRES_DATA_DIR=`mktemp -d -t postgres-XXXXXXXXXX`

echo "* Starting postgresql"

# Add this server to pgadmin using:
# - hostname: managed-gitops-postgres
# - port: 5432
# - username: postgres
# - password: gitops
docker run --name managed-gitops-postgres \
	-v $POSTGRES_DATA_DIR:/var/lib/postgresql/data:Z \
	-e POSTGRES_PASSWORD=gitops	\
	-p 5432:5432 \
	--network gitops-net \
	-d \
	postgres:13 \
	-c log_statement='all' \
	-c log_min_duration_statement=0
	
# -c options are from https://pg.uptrace.dev/faq/#how-to-view-queries-this-library-generates	

echo

echo "* Starting pgadmin"

# pgadmin login/password is the email/password below
docker run --name managed-gitops-pgadmin -p 8080:80 \
    -e 'PGADMIN_DEFAULT_EMAIL=user@user.com' \
    -e 'PGADMIN_DEFAULT_PASSWORD=gitops' \
    --network gitops-net \
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

echo "== Dev environment initialized"
echo "  Postgres username: 'postgres'"
echo "  Postgres password: 'gitops'"
echo "  Postgres ip address: $(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' managed-gitops-postgres)"
echo "  Pgadmin username: 'user@user.com'"
echo "  Pgadmin password: 'gitops'"
echo
