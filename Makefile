MAKEFILE_ROOT=$(shell pwd)
GOBIN=$(shell go env GOPATH)/bin
TAG ?= latest
BASE_IMAGE ?= gitops-service
USERNAME ?= redhat-appstudio
IMG ?= quay.io/${USERNAME}/${BASE_IMAGE}:${TAG}
help: ## Display this help menu
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

### --- P o s t g r e s --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-postgresql: ## Deploy postgres into Kubernetes and insert 'db-schema.sql'
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/manifests/postgresql-staging/postgresql-staging.yaml
	$(MAKEFILE_ROOT)/create-dev-env.sh kube

undeploy-postgresql: ## Undeploy postgres from Kubernetes
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/manifests/postgresql-staging/postgresql-staging.yaml

### --- B a c k e n d --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-backend-crd: ## Deploy backend related CRDs
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml

undeploy-backend-crd: ## Remove backend related CRDs
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml

deploy-backend-rbac: ## Deploy backend related RBAC resouces
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/manifests/backend-rbac/

undeploy-backend-rbac: ## Remove backend related RBAC resouces
	kubectl delete -f  $(MAKEFILE_ROOT)/manifests/backend-rbac/

deploy-backend: deploy-backend-crd deploy-backend-rbac ## Deploy backend operator into Kubernetes -- e.g. make deploy-backend IMG=quay.io/pgeorgia/gitops-service:latest
	kubectl create namespace gitops 2> /dev/null || true
	COMMON_IMAGE=${IMG} envsubst < $(MAKEFILE_ROOT)/manifests/managed-gitops-backend-deployment.yaml | kubectl apply -f -

undeploy-backend: undeploy-backend-rbac undeploy-backend-crd ## Undeploy backend from Kubernetes
	kubectl delete -f $(MAKEFILE_ROOT)/manifests/managed-gitops-backend-deployment.yaml

build-backend: ## Build backend only
	cd $(MAKEFILE_ROOT)/backend && make build

test-backend: ## Run tests for backend only
	cd $(MAKEFILE_ROOT)/backend && make test

### --- c l u s t e r - a g e n t --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-cluster-agent-crd: ## Deploy cluster-agent related CRDs
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
	kubectl create ns argocd 2> /dev/null || true
	kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-cd/release-2.2/manifests/crds/application-crd.yaml

undeploy-cluster-agent-crd: ## Remove cluster-agent related CRDs
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
	kubectl delete -f https://raw.githubusercontent.com/argoproj/argo-cd/release-2.2/manifests/crds/application-crd.yaml
	kubectl delete ns argocd 2> /dev/null || true

deploy-cluster-agent-rbac: ## Deploy cluster-agent related RBAC resouces
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/manifests/cluster-agent-rbac/

undeploy-cluster-agent-rbac: ## Remove cluster-agent related RBAC resouces
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/manifests/cluster-agent-rbac/

deploy-cluster-agent: deploy-cluster-agent-crd deploy-cluster-agent-rbac ## Deploy cluster-agent operator into Kubernetes -- e.g. make deploy-cluster-agent IMG=quay.io/pgeorgia/gitops-service:latest
	kubectl create namespace gitops 2> /dev/null || true
	COMMON_IMAGE=${IMG} envsubst < $(MAKEFILE_ROOT)/manifests/managed-gitops-clusteragent-deployment.yaml | kubectl apply -f -

undeploy-cluster-agent: undeploy-cluster-agent-rbac undeploy-cluster-agent-crd ## Undeploy cluster-agent from Kubernetes
	kubectl delete -f $(MAKEFILE_ROOT)/manifests/managed-gitops-clusteragent-deployment.yaml

build-cluster-agent: ## Build cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make build

test-cluster-agent: ## Run test for cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make test

### --- F A S T  &  F U R I O U S --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #

devenv-docker: deploy-backend-crd deploy-cluster-agent-crd deploy-backend-rbac deploy-cluster-agent-rbac ## Setup local development environment (Postgres via Docker & local operators)
	$(MAKEFILE_ROOT)/create-dev-env.sh

devenv-k8s: deploy-backend-crd deploy-cluster-agent-crd deploy-backend-rbac deploy-cluster-agent-rbac deploy-postgresql ## Setup local development environment (Postgres via k8s & local operators)

install-all-k8s: deploy-postgresql deploy-backend deploy-cluster-agent ## Installs e.g. make deploy-k8s IMG=quay.io/pgeorgia/gitops-service:latest

uninstall-all-k8s: undeploy-postgresql undeploy-backend undeploy-cluster-agent
	kubectl delete namespace gitops

### --- G e n e r a l --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~ #
start: ## Start all the components, compile & run (ensure goreman is installed, with 'go install github.com/mattn/goreman@latest')
	$(GOBIN)/goreman start

build: build-backend build-cluster-agent ## Build all the components - note: you do not need to do this before running start

docker-build: ## Build docker image -- note: you have to change the USER var. Optionally change the BASE_IMAGE or TAG
	docker build -t ${IMG} $(MAKEFILE_ROOT)

docker-push: ## Push docker image - note: you have to change the USER var. Optionally change the BASE_IMAGE or TAG
	docker push ${IMG}

test: test-backend test-backend-shared test-cluster-agent ## Run tests for all components

test-backend-shared: ## Run test for backend-shared only
	cd $(MAKEFILE_ROOT)/backend-shared && make test

download-deps: ## Download goreman to ~/go/bin
	go install github.com/mattn/goreman@latest

reset-db: ## Erase the current database, and reset it scratch; useful during development if you want a clean slate.
	$(MAKEFILE_ROOT)/delete-dev-env.sh
	$(MAKEFILE_ROOT)/create-dev-env.sh

vendor: ## Clone locally the dependencies - off-line
	cd $(MAKEFILE_ROOT)/backend-shared && go mod vendor
	cd $(MAKEFILE_ROOT)/backend && go mod vendor
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod vendor

tidy: ## Tidy all components
	cd $(MAKEFILE_ROOT)/backend-shared && go mod tidy
	cd $(MAKEFILE_ROOT)/backend && go mod tidy 
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod tidy 
