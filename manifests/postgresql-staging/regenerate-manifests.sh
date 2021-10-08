#!/bin/bash

# Call this script file in order to regenerate the postgresql file to the latest version from the Helm chart

helm repo add bitnami https://charts.bitnami.com/bitnami

helm template gitops-postgresql-staging bitnami/postgresql -f values.yaml > postgresql-staging.yaml

