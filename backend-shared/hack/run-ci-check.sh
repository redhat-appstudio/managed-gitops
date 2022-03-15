#!/bin/bash

set -x
set -o errexit
set -o nounset
set -o pipefail

BACKEND_SHARED_DIR=$GITHUB_WORKSPACE/backend-shared
cd ${BACKEND_SHARED_DIR}
go build -o ./hack/dist/ci-check ./hack/ci-check
./dist/ci-check
