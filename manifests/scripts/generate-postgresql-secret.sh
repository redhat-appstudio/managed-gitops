#!/bin/bash

kubectl create namespace gitops 2> /dev/null || true

# Auto-generate DB Secret
if ! kubectl get secret -n gitops gitops-postgresql-staging &>/dev/null; then
	kubectl create secret generic gitops-postgresql-staging \
	--namespace=gitops \
	--from-literal=postgresql-password=$(openssl rand -base64 20)
fi

