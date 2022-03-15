#!/bin/bash

set -x
set -o errexit
set -o nounset
set -o pipefail

BACKEND_SHARED_DIR=${GITHUB_WORKSPACE}/backend-shared

go build -o ${BACKEND_SHARED_DIR}/hack/dist/ci-check ${BACKEND_SHARED_DIR}/hack/ci-check
${BACKEND_SHARED_DIR}/dist/ci-check
