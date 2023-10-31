MAKEFILE_ROOT=$(shell pwd)
GOBIN=$(shell go env GOPATH)/bin
TAG ?= latest
BASE_IMAGE ?= gitops-service
USERNAME ?= redhat-appstudio
IMG ?= quay.io/${USERNAME}/${BASE_IMAGE}:${TAG}
APPLICATION_API_COMMIT ?= f25d47ce749967013a7f13dcb9e65ede36b96f18

# Default values match the their respective deployments in staging/production environment for GitOps Service, otherwise the E2E will fail.
ARGO_CD_NAMESPACE ?= gitops-service-argocd
ARGO_CD_VERSION ?= v2.8.3

# Tool to build the container image. It can be either docker or podman
DOCKER ?= docker

OS ?= $(shell go env GOOS)
# Get the ARCH value to be used for building the binary.
ARCH ?= $(shell go env GOARCH)
$(info OS is ${OS})
$(info Arch is ${ARCH})
ifeq (${OS},darwin)
ifeq (${ARCH},arm64)
  $(info Mac arm64 detected)
  ARCH=amd64
endif
endif

help: ## Display this help menu
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'


### --- D e p l o y m e n t --- T a r g e t s ###

# Deploy only the bare minimum to K8s: CRDs

pre-deploy-crds: kustomize
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/base/crd/overlays/local-dev  | kubectl apply -f -

pre-undeploy-crds: kustomize
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/local-dev-env | kubectl delete -f - 


deploy-local-dev-env: pre-deploy-crds  ## Only deploy CRDs to K8s
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/local-dev-env | kubectl apply -f - 

undeploy-local-dev-env: pre-undeploy-crds ## Remove CRDs from K8s
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/local-dev-env | kubectl delete -f - 

# Deploy bare minimum + postgres: CRDs, and postgresql, but not controllers

deploy-local-dev-env-with-k8s-db: pre-deploy-crds postgresql-secret-on-k8s ## Only deploy CRDs, postgres workload/secret, to K8s
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/local-dev-env-with-k8s-db | kubectl apply -f - 

undeploy-local-dev-env-with-k8s-db: pre-undeploy-crds ## Remove CRDs, postgres workload/secret  from K8s
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/local-dev-env-with-k8s-db | kubectl delete -f - 

# Deploy everything to K8s, including the controllers and postgres workloads

deploy-k8s-env: pre-deploy-crds postgresql-secret-on-k8s ## Deploy all controller/DB workloads to K8s, use e.g. IMG=quay.io/pgeorgia/gitops-service:latest to specify a specific image
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/k8s-env |  COMMON_IMAGE=${IMG} envsubst | kubectl apply -f - 

undeploy-k8s-env: pre-undeploy-crds ## Remove all controller/DB workloads from K8s.
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/k8s-env | kubectl delete -f - 

deploy-k8s-env-e2e: pre-deploy-crds postgresql-secret-on-k8s ## Deploy all controller/DB workloads to K8s for e2e tests, use e.g. IMG=quay.io/pgeorgia/gitops-service:latest to specify a specific image
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/k8s-env-e2e | COMMON_IMAGE=${IMG} envsubst | kubectl apply -f -

undeploy-k8s-env-e2e: pre-undeploy-crds ## Remove all controller/DB e2e test workloads from K8s.
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/overlays/k8s-env-e2e | kubectl delete -f -


### --- P o s t g r e s --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~ #

postgresql-secret-on-k8s: ## Auto-generate the postgres Secret in the gitops namespace
	$(MAKEFILE_ROOT)/manifests/scripts/generate-postgresql-secret.sh

port-forward-postgres-manual: ## Port forward postgresql manually
	$(MAKEFILE_ROOT)/create-dev-env.sh kube

port-forward-postgres-auto: ## Port forward postgresql automatically
	$(MAKEFILE_ROOT)/create-dev-env.sh kube-auto
	
### --- B a c k e n d --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~ #

build-backend: ## Build backend only
	cd $(MAKEFILE_ROOT)/backend && make build

test-backend: ## Run tests for backend only
	cd $(MAKEFILE_ROOT)/backend && make test

### --- c l u s t e r - a g e n t --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #

build-cluster-agent: ## Build cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make build

test-cluster-agent: ## Run test for cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make test

### --- a p p s t u d i o  -  c o n t r o l l e r --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #

build-appstudio-controller: ## Build only
	cd $(MAKEFILE_ROOT)/appstudio-controller && make build

test-appstudio-controller: ## Run test for appstudio-controller only
	cd $(MAKEFILE_ROOT)/appstudio-controller && make test


### --- i n i t  -  c o n t a i n e r --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #

build-init-container-binary: ## Build init-controller binary
	cd $(MAKEFILE_ROOT)/utilities/init-container && make build

test-init-container-binary: ## Run test for init-controller binary only
	cd $(MAKEFILE_ROOT)/utilities/init-container && make test

### --- A r g o C D    W e b   U I --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-argocd: ## Install ArgoCD vanilla Web UI
	ARGO_CD_NAMESPACE=$(ARGO_CD_NAMESPACE) ARGO_CD_VERSION=$(ARGO_CD_VERSION) $(MAKEFILE_ROOT)/argocd.sh install

undeploy-argocd: ## Remove ArgoCD vanilla Web UI
	ARGO_CD_NAMESPACE=$(ARGO_CD_NAMESPACE) ARGO_CD_VERSION=$(ARGO_CD_VERSION) $(MAKEFILE_ROOT)/argocd.sh remove

### --- F A S T  &  F U R I O U S --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #

e2e: build setup-e2e-openshift ## Run e2e tests
	$(GOBIN)/goreman start &> goreman.log &
	cd $(MAKEFILE_ROOT)/tests-e2e && ginkgo -v ./... run

e2e-reset: ## Kills the port-forwarding and the controllers
	pkill goreman
	pkill kubectl

install-argocd-openshift: kustomize ## Using OpenShift GitOps, install Argo CD to the gitops-service-argocd namespace
	PATH=$(MAKEFILE_ROOT)/bin:$(PATH) $(MAKEFILE_ROOT)/manifests/scripts/openshift-argo-deploy/deploy.sh

install-argocd-k8s: ## (Non-OpenShift): Install Argo CD to the gitops-service-argocd namespace
	ARGO_CD_VERSION=$(ARGO_CD_VERSION) manifests/scripts/k8s-argo-deploy/deploy.sh

uninstall-argocd: ## Uninstall Argo CD from gitops-service-argocd namespace (from either OpenShift or K8s)
	kubectl delete namespace "$(ARGO_CD_NAMESPACE)" || true
	kubectl delete -f manifests/scripts/openshift-argo-deploy/openshift-gitops-subscription.yaml || true

devenv-docker: deploy-local-dev-env ## Setup local development environment (Postgres via Docker & local operators)
	$(MAKEFILE_ROOT)/create-dev-env.sh

devenv-k8s: deploy-local-dev-env-with-k8s-db port-forward-postgres-manual  ## Setup local development environment (Postgres via k8s & local operators)

devenv-k8s-e2e: deploy-local-dev-env-with-k8s-db port-forward-postgres-auto ## Setup local development environment (Postgres via k8s & local operators)

install-all-k8s: deploy-k8s-env port-forward-postgres-manual ## Installs e.g. make install-all-k8s IMG=quay.io/pgeorgia/gitops-service:latest

install-all-k8s-e2e: deploy-k8s-env-e2e port-forward-postgres-manual ## Installs for e2e tests e.g. make install-all-k8s-e2e IMG=quay.io/pgeorgia/gitops-service:latest

uninstall-all-k8s: undeploy-k8s-env
	kubectl delete namespace gitops

### --- G e n e r a l --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~ #
start: ## Start all the components, compile & run (ensure goreman is installed, with 'go install github.com/mattn/goreman@latest')
	$(GOBIN)/goreman start

start-chaos: ## Start all the components, compile & run (ensure goreman is installed, with 'go install github.com/mattn/goreman@latest')
	$(GOBIN)/goreman -f Procfile.chaos start

start-execs: ## Start all the components, compile & run using execs in component folders (ensure goreman is installed, with 'go install github.com/mattn/goreman@latest')
	$(GOBIN)/goreman -f Procfile.runexecs start

clean: ## remove the bin and vendor folders from each component
	cd $(MAKEFILE_ROOT)/backend-shared && make clean
	cd $(MAKEFILE_ROOT)/backend && make clean
	cd $(MAKEFILE_ROOT)/cluster-agent && make clean
	cd $(MAKEFILE_ROOT)/appstudio-controller && make clean
	cd $(MAKEFILE_ROOT)/tests-e2e && make clean
	cd $(MAKEFILE_ROOT)/utilities/db-migration && make clean

clean-execs: ## remove the main executables in each component
	cd $(MAKEFILE_ROOT)/backend && make clean-exec
	cd $(MAKEFILE_ROOT)/cluster-agent && make clean-exec
	cd $(MAKEFILE_ROOT)/appstudio-controller && make clean-exec

build: build-backend build-cluster-agent build-appstudio-controller build-init-container-binary ## Build all the components - note: you do not need to do this before running start

docker-build: ## Build docker image -- note: you have to change the USERNAME var. Optionally change the BASE_IMAGE or TAG
	$(DOCKER) build --build-arg ARCH=$(ARCH) -t ${IMG} $(MAKEFILE_ROOT)

docker-push: ## Push docker image - note: you have to change the USERNAME var. Optionally change the BASE_IMAGE or TAG
	$(DOCKER) push ${IMG}

test: test-backend test-backend-shared test-cluster-agent test-appstudio-controller test-init-container-binary ## Run tests for all components

setup-e2e-openshift: install-argocd-openshift devenv-k8s-e2e ## Setup steps for E2E tests to run with Openshift CI

setup-e2e-local: install-argocd-openshift devenv-docker reset-db ## Setup steps for E2E tests to run with Local Openshift Cluster

start-e2e: ## Start the managed gitops processes for E2E tests.
	$(GOBIN)/goreman -f Procfile.no-self-heal start

test-e2e: ## Kick off the E2E tests. Ensure that 'start-e2e' and 'setup-e2e-openshift' have run.
	cd $(MAKEFILE_ROOT)/tests-e2e && make test

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
	cd $(MAKEFILE_ROOT)/appstudio-controller && go mod vendor	
	cd $(MAKEFILE_ROOT)/tests-e2e && go mod vendor	
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go mod vendor	
	cd $(MAKEFILE_ROOT)/utilities/init-container && go mod vendor

tidy: ## Tidy all components
	cd $(MAKEFILE_ROOT)/backend-shared && go mod tidy
	cd $(MAKEFILE_ROOT)/backend && go mod tidy 
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod tidy
	cd $(MAKEFILE_ROOT)/appstudio-controller && go mod tidy
	cd $(MAKEFILE_ROOT)/tests-e2e && go mod tidy
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go mod tidy
	cd $(MAKEFILE_ROOT)/utilities/init-container && go mod tidy
	cd $(MAKEFILE_ROOT)/utilities/load-test && go mod tidy
	cd $(MAKEFILE_ROOT)/utilities/gitopsctl && go mod tidy		
	 
fmt: ## Run 'go fmt' on all components
	cd $(MAKEFILE_ROOT)/backend-shared && make fmt
	cd $(MAKEFILE_ROOT)/backend && make fmt
	cd $(MAKEFILE_ROOT)/cluster-agent && make fmt
	cd $(MAKEFILE_ROOT)/appstudio-controller && make fmt
	cd $(MAKEFILE_ROOT)/utilities/db-migration && make fmt
	cd $(MAKEFILE_ROOT)/utilities/init-container && make fmt
	cd $(MAKEFILE_ROOT)/utilities/gitopsctl && make fmt	

lint: ## Run lint checks for all components
	cd $(MAKEFILE_ROOT)/backend-shared && make lint
	cd $(MAKEFILE_ROOT)/backend && make lint
	cd $(MAKEFILE_ROOT)/cluster-agent && make lint
	cd $(MAKEFILE_ROOT)/appstudio-controller && make lint
	cd $(MAKEFILE_ROOT)/tests-e2e && make lint
	cd $(MAKEFILE_ROOT)/utilities/db-migration && make lint
	cd $(MAKEFILE_ROOT)/utilities/init-container && make lint
	cd $(MAKEFILE_ROOT)/utilities/gitopsctl && make lint

gosec: ## Run gosec checks for all components
	cd $(MAKEFILE_ROOT)/backend-shared && make gosec
	cd $(MAKEFILE_ROOT)/backend && make gosec
	cd $(MAKEFILE_ROOT)/cluster-agent && make gosec
	cd $(MAKEFILE_ROOT)/appstudio-controller && make gosec
	cd $(MAKEFILE_ROOT)/tests-e2e && make gosec
#	cd $(MAKEFILE_ROOT)/utilities/db-migration && make gosec
#	cd $(MAKEFILE_ROOT)/utilities/init-container && make gossec
#	cd $(MAKEFILE_ROOT)/utilities/gitopsctl && make gossec


generate-manifests: ## Call the 'generate' and 'manifests' targets of every project
	cd $(MAKEFILE_ROOT)/backend-shared && make generate manifests
	cd $(MAKEFILE_ROOT)/backend && make generate manifests
	cd $(MAKEFILE_ROOT)/cluster-agent && make generate manifests
	cd $(MAKEFILE_ROOT)/appstudio-controller && make generate manifests

### --- D a t a b a s e  --- ###

db-migrate:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go

db-drop:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go drop

db-drop_smtable:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true go run main.go drop_smtable

db-migrate-downgrade:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go downgrade_migration

db-migrate-upgrade:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go upgrade_migration

db-schema: ## Run db-schema varchar tests
	cd $(MAKEFILE_ROOT)/backend-shared && go run ./hack/db-schema-sync-check

### --- CI Tests ---

check-backward-compatibility: ##  test executed from OpenShift CI
	cd $(MAKEFILE_ROOT)/tests-e2e && make test-backward-compatibility


### --- Utilities for other makefile targets ---

ensure-gitops-ns-exists:
	kubectl create namespace gitops 2> /dev/null || true

ensure-workload-gitops-ns-exists:
	kubectl create namespace gitops 2> /dev/null || true

### --- M O C K S ---

mocks:
	cd $(MAKEFILE_ROOT)/backend-shared && make mocks
	cd $(MAKEFILE_ROOT)/backend && make mocks
	cd $(MAKEFILE_ROOT)/cluster-agent && make mocks

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
