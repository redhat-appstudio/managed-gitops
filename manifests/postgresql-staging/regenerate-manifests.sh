#!/bin/bash

echo "* This script is deprecated: You can still use it to get the latest Bitnami chart resources, but our YAML is now different from the generated resources."

exit 1

# Call this script file in order to regenerate the postgresql file to the latest version from the Helm chart

helm repo add bitnami https://charts.bitnami.com/bitnami

helm template gitops-postgresql-staging bitnami/postgresql -f values.yaml -n gitops > postgresql-staging-bitnami.yaml

