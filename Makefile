
MAKEFILE_ROOT=$(shell pwd)
GOBIN=$(shell go env GOPATH)/bin
TAG ?= latest
BASE_IMAGE ?= gitops-service
USERNAME ?= redhat-appstudio
IMG ?= quay.io/${USERNAME}/${BASE_IMAGE}:${TAG}
help: ## Display this help menu
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

# install: Ensure that the Argo CD namespace exists, that Argo CD is installed, and that CRDs we are using are applied to the current cluster
install-argo: ## Ensure that the Argo CD namespace exists, and that CRDs we are using are applied to the current cluster
	kubectl apply -f $(MAKEFILE_ROOT)/backend/config/crd/bases
	kubectl apply -f $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
	kubectl create ns argocd || true
	kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-cd/release-2.2/manifests/crds/application-crd.yaml

start: ## Start all the components, compile & run (ensure goreman is installed, with 'go install github.com/mattn/goreman@latest')
	$(GOBIN)/goreman start

build: build-backend build-cluster-agent ## Build all the components - note: you do not need to do this before running start

build-backend: ## Build backend only
	cd $(MAKEFILE_ROOT)/backend && make build

build-cluster-agent: ## Build cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make build

docker-build: ## Build docker image -- note you can change the vars 'BASE_IMAGE=gitops-service' and 'TAG=v0.0.1'
	docker build -t ${BASE_IMAGE}:${TAG} $(MAKEFILE_ROOT)

docker-push: ## Push docker image - note: you have to change the USER var. Optionally change the BASE_IMAGE or TAG.
	docker push ${IMG}

test: test-backend test-backend-shared test-cluster-agent ## Run tests for all components

test-backend: ## Run tests for backend only
	cd $(MAKEFILE_ROOT)/backend && make test

test-backend-shared: ## Run test for backend-shared only
	cd $(MAKEFILE_ROOT)/backend-shared && make test

test-cluster-agent: ## Run test for cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make test

download-deps: ## Download goreman to ~/go/bin
	go install github.com/mattn/goreman@latest

reset-db: ## Erase the current database, and reset it scratch; useful during development if you want a clean slate.
	$(MAKEFILE_ROOT)/delete-dev-env.sh
	$(MAKEFILE_ROOT)/create-dev-env.sh

vendor: ## Clone locally the dependencies - off-line
	cd $(MAKEFILE_ROOT)/backend-shared && go mod vendor
	cd $(MAKEFILE_ROOT)/backend && go mod vendor
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod vendor
