#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"


# Tip: More info on psql - https://tomcam.github.io/postgres/

PGPASSWORD=gitops "psql" -h localhost -d postgres -U postgres -p 5432 $*

