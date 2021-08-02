#!/bin/bash

# Start postgresql

docker run --name managed-gitops-postgres -v /tmp/datadir:/var/lib/postgresql/data  -e POSTGRES_PASSWORD=mysecretpassword	-p 5432:5432  -d postgres:13

docker run --link managed-gitops-postgres --name managed-gitops-pgadmin -p 80:80 \
    -e 'PGADMIN_DEFAULT_EMAIL=user@user.com' \
    -e 'PGADMIN_DEFAULT_PASSWORD=mysecretpassword' \
    -d dpage/pgadmin4

#PGPASSWORD-mysecretpassword psql -h localhost -d postgres -U postgres -p 5432 -a -q -f filepath


