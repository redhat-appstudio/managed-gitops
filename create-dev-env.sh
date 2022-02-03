#!/usr/bin/env bash

# Checks if a binary is present on the local system
exit_if_binary_not_installed() {
  for binary in "$@"; do
    command -v "$binary" >/dev/null 2>&1 || {
      echo >&2 "Script requires '$binary' command-line utility to be installed on your local machine. Aborting..."
      exit 1
    }
  done
}

# Checks if a command is successfull within a timeframe
wait-until() {
  command="${1}"
  timeout="${2:-30}"

  i=1
  until eval "${command}"; do
    echo "$i try ..."
    ((i++))

    if [ "${i}" -gt "${timeout}" ]; then
      echo "postgress server never replied back, aborting due to ${timeout}s timeout!"
      exit 1
    fi

    sleep 1
  done
}

# Installs 'db-schema.sql' into the PostgreSQL running in Kubernetes cluster
if [ "$1" = "kube" ]; then
  exit_if_binary_not_installed "kubectl" "psql"

  # Get the secret
  counter=0
  until kubectl -n gitops get secrets | grep gitops-postgresql-staging
  do
    ((counter++))
    sleep 1
    if [ "$counter" -gt 5 ]; then
      echo " --> Error: Cannot find gitops-postgresql-staging secret."
      exit 1
    fi
  done
  echo " * Postgres secret has been created."

   # Wait until postgres pod is running
  echo " * Wait until Postgres pod is running"
  counter=0
  until kubectl -n gitops get pods | grep postgres | grep '1/1' | grep 'Running' &> /dev/null
  do
    ((counter++))
    sleep 1
    if [ "$counter" -gt 60 ]; then
      echo " --> Error: PostgreSQL pod cannot start. Quitting ..."
      exit 1
    fi
  done
  echo " * Postgres Pod is running."

  # Checks if 5432 is occupied
  if lsof -i:5432 | grep LISTEN; then
    echo " --> Error: Your local port TCP 5432 is already in use. Quit port-forward."
    exit 1
  fi
  echo " * Start port-fowarding PostgreSQL to localhost:5432 ..."


  # Port forward the PostgreSQL service locally, so we can access it
	kubectl port-forward --namespace gitops svc/gitops-postgresql-staging 5432:5432 &
  KUBE_PID=$!

  # Checks if 5432 is occupied
  counter=0
  until lsof -i:5432 | grep LISTEN
  do
    sleep 1
    if [ "$counter" -gt 10 ]; then
      echo ".. retry $counter ..."
      echo " --> Error: Port-forwarding takes too long. Quiting ..."
      if ! kill $KUBE_PID; then
        echo " --> Error: Cannot kill the background port-forward, do it yourself."
      fi
      exit 1
    fi
  done
  echo " * Port-Forwarding worked"

  # Decode the password from the secret
  POSTGRES_PASSWORD=$(kubectl get -n gitops secret gitops-postgresql-staging -o jsonpath="{.data.postgresql-password}" | base64 --decode)

  # Import the schema
	if psql postgresql://postgres:"${POSTGRES_PASSWORD}"@127.0.0.1:5432/postgres -q -f db-schema.sql; then
    echo " * db-schema.sql has been imported into PostgreSQL (running in Kubernetes)"
  else
    echo " --> Error: Cannot import 'db-schema.sql' into PostgreSQL (running in Kubernetes)"
    exit 1
  fi

  # Stop port-forwarding
	if ! kill $KUBE_PID; then
    echo " --> Error: Cannot kill the background port-forward, do it yourself."
    exit 1
  else
    echo " * Port-forwarding has been successfully stopped"
  fi

  if lsof -i:5432 | grep LISTEN; then
    echo " * Port forwarding is still active for some reason. Investigate further ..."
  fi

  # Exit now, do not continue with the rest of the bash script
  echo
  echo " ------------------------------------------------"
  echo "| To access the database outside of the cluster  |"
  echo " ------------------------------------------------"

  echo "  - Run:            kubectl port-forward --namespace gitops svc/gitops-postgresql-staging 5432:5432 &"
  echo "  - Credentials:    HOST=127.0.0.1:5432  USERNAME=postgres  PASSWORD=$POSTGRES_PASSWORD  DB=postgres"
  echo "  - Access Example: psql postgresql://postgres:$POSTGRES_PASSWORD@127.0.0.1:5432/postgres -c \"select now()\""
  echo
  echo "  - To run Backend & ClusterAgent Operators locally: export DB_PASS=$POSTGRES_PASSWORD && goreman start"
  echo " -------------------------------------------------"
  echo
  exit 0
fi

# Binary requirements
exit_if_binary_not_installed "docker" "mktemp"

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

# Exit if docker is not running (or cannot interact with it)
if ! docker info >/dev/null 2>&1; then
  echo "Error: docker is not running"
  exit 1
fi

# Variables
NETWORK="gitops-net"
POSTGRES_CONTAINER="managed-gitops-postgres"
PGADMIN_CONTAINER="managed-gitops-pgadmin"
RETRIES=30                                            # in seconds
POSTGRES_DATA_DIR=$(mktemp -d -t postgres-XXXXXXXXXX) # Map the docker data directory into a temporary directory
POSTGRES_SERVER_IS_UP="docker exec --user postgres -e PGPASSWORD=gitops -i \"$POSTGRES_CONTAINER\" \"psql\" -h localhost -d postgres -U postgres -p 5432 -c \"select 1\" | grep '1 row' >/dev/null 2>&1"

# Create docker network if one doesn't exist yet
echo "* Creating docker network '$NETWORK'"
ID=$(docker network ls --filter "name=$NETWORK" -q 2>/dev/null)
if [ "$(docker network inspect "$ID" -f '{{.Name}}')" == "$NETWORK" ]; then
  echo "  Skip creation: '$NETWORK' has already been created."
else
  docker network create "$NETWORK"
fi

# TEST IT
if ! docker network ls | grep "$NETWORK" &>/dev/null; then
  echo "Error: Docker network $NETWORK cannot be created. Aborting ..."
  exit 1
fi

echo
echo "* Starting postgresql container: $POSTGRES_CONTAINER"
# Add this server to pgadmin using:
# - hostname: managed-gitops-postgres
# - port: 5432
# - username: postgres (default, but we also explicitly set this)
# - password: gitops
if [ "$(docker container inspect -f '{{.State.Status}}' $POSTGRES_CONTAINER 2>/dev/null)" == "running" ]; then
  echo "  Skip creation: '$POSTGRES_CONTAINER' container is already running."
else
  docker run --name "$POSTGRES_CONTAINER" \
    -v "$POSTGRES_DATA_DIR":/var/lib/postgresql/data:Z \
    -e POSTGRES_PASSWORD=gitops \
    -e POSTGRES_USER=postgres \
    -p 5432:5432 \
    --network gitops-net \
    -d \
    postgres:13 \
    -c log_statement='all' \
    -c log_min_duration_statement=0
fi

if ! docker ps | grep "$POSTGRES_CONTAINER" >/dev/null 2>&1; then
  echo "Container '$POSTGRES_CONTAINER' is not running. Aborting ..."
  exit 1
fi

# -c options are from https://pg.uptrace.dev/faq/#how-to-view-queries-this-library-generates

echo
echo "* Starting pgadmin container: $PGADMIN_CONTAINER"
if [ "$(docker container inspect -f '{{.State.Status}}' $PGADMIN_CONTAINER 2>/dev/null)" == "running" ]; then
  echo "  Skip creation: '$PGADMIN_CONTAINER' container is already running."
else
  # pgadmin login/password is the email/password below
  docker run --name "$PGADMIN_CONTAINER" -p 8080:80 \
    -e 'PGADMIN_DEFAULT_EMAIL=user@user.com' \
    -e 'PGADMIN_DEFAULT_PASSWORD=gitops' \
    --network gitops-net \
    -d dpage/pgadmin4
fi

if ! docker ps | grep "$PGADMIN_CONTAINER" >/dev/null 2>&1; then
  echo "Container '$PGADMIN_CONTAINER' is not running. Aborting ..."

  exit 1
fi

echo
echo "* Waiting $RETRIES seconds until postgress server is up..."
wait-until "$POSTGRES_SERVER_IS_UP" "${RETRIES}"
echo "  Done"
echo

echo "* Initializing DB"
echo "  Copying db-schema.sql into the postgress container."
if ! docker cp "$SCRIPTPATH/db-schema.sql" $POSTGRES_CONTAINER:/ >/dev/null 2>&1; then
  echo "db-schema.sql cannot be copied into the '$POSTGRES_CONTAINER' container"
  exit 1
fi
docker exec \
  --user postgres \
  -e PGPASSWORD=gitops \
  -i "$POSTGRES_CONTAINER" "psql" \
  -h localhost \
  -d postgres \
  -U postgres \
  -p 5432 \
  -q -f db-schema.sql

echo
echo "== Dev environment initialized =="
echo "  Postgres username: 'postgres'"
echo "  Postgres password: 'gitops'"
echo "  Postgres ip address: $(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $POSTGRES_CONTAINER)"
echo "  Pgadmin username: 'user@user.com'"
echo "  Pgadmin password: 'gitops'"
echo
