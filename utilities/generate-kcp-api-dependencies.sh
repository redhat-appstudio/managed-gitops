#!/bin/sh

ARGO_CD_VERSION=release-2.3
# This script focuses on generation of APIResourceSchema for GitOps Service dependencies
# We have appstudio and argocd applications CRDs as dependencies to run the service
set -e

if ! which kubectl-kcp; then
  echo "kubectl-kcp required on path"
  echo "you can install it running:"
  echo "    $ git clone https://github.com/kcp-dev/kcp && cd kcp && make install"
  exit 1
fi

THIS_DIR="$(dirname "$(realpath "$0")")"
CRD_DIR="$(realpath ${THIS_DIR}/../manifests/dependencies/crd)"
KCP_API_DIR="$(realpath ${THIS_DIR}/../manifests/dependencies/kcp)"

rm -rf ${CRD_DIR}/*.yaml
wget -P ${CRD_DIR} https://raw.githubusercontent.com/argoproj/argo-cd/${ARGO_CD_VERSION}/manifests/crds/application-crd.yaml
wget -P ${CRD_DIR} https://raw.githubusercontent.com/redhat-appstudio/application-service/7a1a14b575dc725a46ea2ab175692f464122f0f8/config/crd/bases/appstudio.redhat.com_applications.yaml


KCP_API_SCHEMA_FILE_CURRENT="${KCP_API_DIR}/apiresourceschema_gitopsrvc_dependencies.yaml"
KCP_API_SCHEMA_FILE_NEW="${KCP_API_DIR}/apiresourceschema_gitopsrvc_dependencies.yaml_new"
cat << EOF > ${KCP_API_SCHEMA_FILE_NEW}
# This file is generated from CRDs by ./utilities/generate-kcp-api-dependencies.sh script.
# Please do not modify!

EOF

# APIResourceSchema is immutable so when we want to update something, we actually have to create new version.
# Version is defined by this prefix, which is taken from date. This will allow us to do new version each minute, which
# should be hopefully enough granularity :)
PREFIX=$( TZ="Etc/UTC" date +%Y%m%d%H%M )

I=0
for CRD in $(ls ${CRD_DIR}); do
  kubectl-kcp crd snapshot -f "${CRD_DIR}/${CRD}" --prefix v${PREFIX} >> ${KCP_API_SCHEMA_FILE_NEW}
done

# If there are some changes in new generated file, we replace old one. Otherwise just remove new file.
# The regex is there to ignore name change, because we're updating date there so it is expected to change.
# Ignored line looks like this:
# '  name: v202206151654.applications.managed-gitops.redhat.com'
if ! diff -I '^  name: v[0-9]\{12\}\.applications\..*$' ${KCP_API_SCHEMA_FILE_CURRENT} ${KCP_API_SCHEMA_FILE_NEW} > /dev/null; then
  mv ${KCP_API_SCHEMA_FILE_NEW} ${KCP_API_SCHEMA_FILE_CURRENT}
  echo "updated KCP APIResourceSchema for GitOps Service dependencies is saved at '${KCP_API_SCHEMA_FILE_CURRENT}'"
else
  echo "no changes in KCP API"
  rm ${KCP_API_SCHEMA_FILE_NEW}
fi


# now create APIExport and link all created APIResourceSchemas there
KCP_API_EXPORT_FILE="${KCP_API_DIR}/apiexport_gitopsrvc_dependencies.yaml"
cat << EOF > ${KCP_API_EXPORT_FILE}
apiVersion: apis.kcp.dev/v1alpha1
kind: APIExport
metadata:
  name: gitopsrvc
spec:
  permissionClaims:
  - group: ""
    resource: "secrets"
  - group: ""
    resource: "namespaces"
  latestResourceSchemas:
EOF

# match APIResourceSchema name pattern with date prefix
for SCHEMA in $( cat ${KCP_API_SCHEMA_FILE_CURRENT} | grep -o "v[0-9]\{12\}\.applications\..*" ); do
  echo "    - ${SCHEMA}" >> ${KCP_API_EXPORT_FILE}
done

cat << EOF > ${KCP_API_EXPORT_FILE}
# This file is generated from CRDs by ./utilities/generate-kcp-api-depedencies.sh script.
# Please do not modify!

$( cat ${KCP_API_EXPORT_FILE} )
EOF