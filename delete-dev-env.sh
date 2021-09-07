#!/bin/bash

docker rm -f managed-gitops-postgres
docker rm -f managed-gitops-pgadmin

#echo "* Deleting /tmp/datadir, may require sudo"
#sudo rm -rf /tmp/datadir
