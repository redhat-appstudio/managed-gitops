#!/usr/bin/env bash

set -x
set -o errexit
set -o nounset
set -o pipefail

SCRIPTPATH="$(
  cd -- "$(dirname "$0")" >/dev/null 2>&1 || exit
  pwd -P
)"

export ROOTPATH=$SCRIPTPATH/../../
BACKEND_SHARED_DIR=$ROOTPATH/backend-shared
cd ${BACKEND_SHARED_DIR}

go run ./hack/db-schema-sync-check
