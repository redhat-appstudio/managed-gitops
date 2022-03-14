#!/bin/bash

set -x
set -o errexit
set -o nounset
set -o pipefail

BACKEND_SHARED_DIR=$(cd $(dirname "$0")/.. ; pwd)

go build -o ./dist/ci-check ${BACKEND_SHARED_DIR}/hack/ci-check
./dist/ci-check
