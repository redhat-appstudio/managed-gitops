MAKEFILE_ROOT=$(shell pwd)
GOBIN=$(shell go env GOPATH)/bin
TAG ?= latest
BASE_IMAGE ?= gitops-service
USERNAME ?= redhat-appstudio
IMG ?= quay.io/${USERNAME}/${BASE_IMAGE}:${TAG}

# Default values match the their respective deployments in staging/production environment for GitOps Service, otherwise the E2E will fail.
ARGO_CD_NAMESPACE ?= gitops-service-argocd
ARGO_CD_VERSION ?= v2.5.1

help: ## Display this help menu
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

### --- P o s t g r e s --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-postgresql: ## Deploy postgres into Kubernetes and insert 'db-schema.sql'
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/manifests/postgresql-staging/postgresql-staging.yaml
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/manifests/postgresql-staging/postgresql-staging-secret.yaml	

undeploy-postgresql: ## Undeploy postgres from Kubernetes
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/manifests/postgresql-staging/postgresql-staging.yaml
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/manifests/postgresql-staging/postgresql-staging-secret.yaml

port-forward-postgress-manual: ## Port forward postgresql manually
	$(MAKEFILE_ROOT)/create-dev-env.sh kube

port-forward-postgress-auto: ## Port forward postgresql automatically
	$(MAKEFILE_ROOT)/create-dev-env.sh kube-auto
	
### --- B a c k e n d  -  S h a r e d--- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-backend-shared-crd: ## Deploy backend related CRDs
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentrepositorycredentials.yaml
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentmanagedenvironments.yaml

undeploy-backend-shared-crd: ## Remove backend related CRDs
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeployments.yaml
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentsyncruns.yaml
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentrepositorycredentials.yaml
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_gitopsdeploymentmanagedenvironments.yaml

### --- B a c k e n d --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-backend-rbac: kustomize ## Deploy backend related RBAC resouces
	kubectl create namespace gitops 2> /dev/null || true
	$(KUSTOMIZE) build  $(MAKEFILE_ROOT)/manifests/backend-rbac/ | kubectl -n gitops apply -f -

undeploy-backend-rbac: ## Remove backend related RBAC resouces
	$(KUSTOMIZE) build  $(MAKEFILE_ROOT)/manifests/backend-rbac/ | kubectl -n gitops delete -f -

deploy-backend: deploy-backend-shared-crd deploy-backend-rbac ## Deploy backend operator into Kubernetes -- e.g. make deploy-backend IMG=quay.io/pgeorgia/gitops-service:latest
	kubectl create namespace gitops 2> /dev/null || true
	ARGO_CD_NAMESPACE=${ARGO_CD_NAMESPACE} COMMON_IMAGE=${IMG} envsubst < $(MAKEFILE_ROOT)/manifests/controller-deployments/managed-gitops-backend-deployment.yaml | kubectl apply -f -

undeploy-backend: undeploy-backend-rbac undeploy-backend-shared-crd ## Undeploy backend from Kubernetes
	kubectl delete -f $(MAKEFILE_ROOT)/manifests/controller-deployments/managed-gitops-backend-deployment.yaml

build-backend: ## Build backend only
	cd $(MAKEFILE_ROOT)/backend && make build

test-backend: ## Run tests for backend only
	cd $(MAKEFILE_ROOT)/backend && make test

### --- c l u s t e r - a g e n t --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-cluster-agent-crd: ## Deploy cluster-agent related CRDs
	kubectl create namespace gitops 2> /dev/null || true
	kubectl -n gitops apply -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
	kubectl create ns "${ARGO_CD_NAMESPACE}" 2> /dev/null || true
	kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-cd/$(ARGO_CD_VERSION)/manifests/crds/application-crd.yaml

undeploy-cluster-agent-crd: ## Remove cluster-agent related CRDs
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
	kubectl delete -f https://raw.githubusercontent.com/argoproj/argo-cd/$(ARGO_CD_VERSION)/manifests/crds/application-crd.yaml
	kubectl delete ns "${ARGO_CD_NAMESPACE}" 2> /dev/null || true

deploy-cluster-agent-rbac: kustomize ## Deploy cluster-agent related RBAC resouces
	kubectl create namespace gitops 2> /dev/null || true	
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/cluster-agent-rbac/ | kubectl -n gitops apply -f -

undeploy-cluster-agent-rbac: kustomize ## Remove cluster-agent related RBAC resouces
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/cluster-agent-rbac/ | kubectl -n gitops delete -f -

deploy-cluster-agent: deploy-cluster-agent-crd deploy-cluster-agent-rbac ## Deploy cluster-agent operator into Kubernetes -- e.g. make deploy-cluster-agent IMG=quay.io/pgeorgia/gitops-service:latest
	kubectl create namespace gitops 2> /dev/null || true
	ARGO_CD_NAMESPACE=${ARGO_CD_NAMESPACE} COMMON_IMAGE=${IMG} envsubst < $(MAKEFILE_ROOT)/manifests/controller-deployments/managed-gitops-clusteragent-deployment.yaml | kubectl apply -f -

undeploy-cluster-agent: undeploy-cluster-agent-rbac undeploy-cluster-agent-crd ## Undeploy cluster-agent from Kubernetes
	kubectl delete -f $(MAKEFILE_ROOT)/manifests/controller-deployments/managed-gitops-clusteragent-deployment.yaml

build-cluster-agent: ## Build cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make build

test-cluster-agent: ## Run test for cluster-agent only
	cd $(MAKEFILE_ROOT)/cluster-agent && make test

### --- a p p s t u d i o - c o n t r o l l e r --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~ #
deploy-appstudio-controller-crd: ## Deploy appstudio-controller related CRDs
	# appstudio-shared CRs
	kubectl apply -f appstudio-shared/manifests/appstudio-shared-customresourcedefinitions.yaml
	# Application CR from AppStudio HAS
	kubectl apply -f https://raw.githubusercontent.com/redhat-appstudio/application-service/7a1a14b575dc725a46ea2ab175692f464122f0f8/config/crd/bases/appstudio.redhat.com_applications.yaml
	kubectl apply -f https://raw.githubusercontent.com/redhat-appstudio/application-service/7a1a14b575dc725a46ea2ab175692f464122f0f8/config/crd/bases/appstudio.redhat.com_components.yaml

undeploy-appstudio-controller-crd: ## Remove appstudio-controller related CRDs
	kubectl delete -f https://raw.githubusercontent.com/redhat-appstudio/application-service/7a1a14b575dc725a46ea2ab175692f464122f0f8/config/crd/bases/appstudio.redhat.com_applications.yaml
	kubectl delete -f https://raw.githubusercontent.com/redhat-appstudio/application-service/7a1a14b575dc725a46ea2ab175692f464122f0f8/config/crd/bases/appstudio.redhat.com_components.yaml

deploy-appstudio-controller-rbac: kustomize ## Deploy appstudio-controller related RBAC resouces
	kubectl create namespace gitops 2> /dev/null || true
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/appstudio-controller-rbac/ | kubectl -n gitops apply -f -

undeploy-appstudio-controller-rbac: kustomize ## Remove appstudio-controller related RBAC resouces
	kubectl -n gitops delete -f  $(MAKEFILE_ROOT)/manifests/appstudio-controller-rbac/
	$(KUSTOMIZE) build $(MAKEFILE_ROOT)/manifests/appstudio-controller-rbac/ | kubectl -n gitops delete -f -

deploy-appstudio-controller: deploy-appstudio-controller-crd deploy-appstudio-controller-rbac ## Deploy appstudio-controller operator into Kubernetes -- e.g. make appstudio-controller IMG=quay.io/pgeorgia/gitops-service:latest
	kubectl create namespace gitops 2> /dev/null || true
	COMMON_IMAGE=${IMG} envsubst < $(MAKEFILE_ROOT)/manifests/controller-deployments/appstudio-controller/managed-gitops-appstudio-controller-deployment.yaml | kubectl apply -f -

undeploy-appstudio-controller: undeploy-appstudio-controller-rbac undeploy-appstudio-controller-crd ## Undeploy appstudio-controller from Kubernetes
	kubectl delete -f $(MAKEFILE_ROOT)/manifests/controller-deployments/appstudio-controller/managed-gitops-appstudio-controller-deployment.yaml

build-appstudio-controller: ## Build only
	cd $(MAKEFILE_ROOT)/appstudio-controller && make build

test-appstudio-controller: ## Run test for appstudio-controller only
	cd $(MAKEFILE_ROOT)/appstudio-controller && make test

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

install-argocd-openshift: ## Using OpenShift GitOps, install Argo CD to the gitops-service-argocd namespace
	manifests/openshift-argo-deploy/deploy.sh

install-argocd-k8s: ## (Non-OpenShift): Install Argo CD to the gitops-service-argocd namespace
	ARGO_CD_VERSION=$(ARGO_CD_VERSION) manifests/k8s-argo-deploy/deploy.sh

uninstall-argocd: ## Uninstall Argo CD from gitops-service-argocd namespace (from either OpenShift or K8s)
	kubectl delete namespace "$(ARGO_CD_NAMESPACE)" || true
	kubectl delete -f manifests/openshift-argo-deploy/openshift-gitops-subscription.yaml || true

devenv-docker: deploy-backend-shared-crd deploy-cluster-agent-crd deploy-appstudio-controller-crd deploy-backend-rbac deploy-cluster-agent-rbac deploy-appstudio-controller-rbac ## Setup local development environment (Postgres via Docker & local operators)
	$(MAKEFILE_ROOT)/create-dev-env.sh

devenv-k8s: deploy-backend-shared-crd deploy-cluster-agent-crd deploy-backend-rbac deploy-cluster-agent-rbac deploy-postgresql port-forward-postgress-manual deploy-appstudio-controller ## Setup local development environment (Postgres via k8s & local operators)

devenv-k8s-e2e: deploy-backend-shared-crd deploy-cluster-agent-crd deploy-backend-rbac deploy-cluster-agent-rbac deploy-postgresql port-forward-postgress-auto deploy-appstudio-controller ## Setup local development environment (Postgres via k8s & local operators)

install-all-k8s: deploy-postgresql port-forward-postgress-manual deploy-backend deploy-cluster-agent deploy-appstudio-controller ## Installs e.g. make deploy-k8s IMG=quay.io/pgeorgia/gitops-service:latest

uninstall-all-k8s: undeploy-postgresql undeploy-backend undeploy-cluster-agent undeploy-appstudio-controller
	kubectl delete namespace gitops

### --- G e n e r a l --- ###
# ~~~~~~~~~~~~~~~~~~~~~~~~~ #
start: ## Start all the components, compile & run (ensure goreman is installed, with 'go install github.com/mattn/goreman@latest')
	$(GOBIN)/goreman start

clean: ## remove the bin and vendor folders from each component
	cd $(MAKEFILE_ROOT)/appstudio-shared && make clean
	cd $(MAKEFILE_ROOT)/backend-shared && make clean
	cd $(MAKEFILE_ROOT)/backend && make clean
	cd $(MAKEFILE_ROOT)/cluster-agent && make clean
	cd $(MAKEFILE_ROOT)/appstudio-controller && make clean
	cd $(MAKEFILE_ROOT)/tests-e2e && make clean
	cd $(MAKEFILE_ROOT)/utilities/db-migration && make clean

build: build-backend build-cluster-agent build-appstudio-controller ## Build all the components - note: you do not need to do this before running start

docker-build: ## Build docker image -- note: you have to change the USERNAME var. Optionally change the BASE_IMAGE or TAG
	docker build -t ${IMG} $(MAKEFILE_ROOT)

docker-push: ## Push docker image - note: you have to change the USERNAME var. Optionally change the BASE_IMAGE or TAG
	docker push ${IMG}

test: test-backend test-backend-shared test-cluster-agent test-appstudio-controller ## Run tests for all components

setup-e2e-openshift: install-argocd-openshift devenv-k8s-e2e ## Setup steps for E2E tests to run with Openshift CI

setup-e2e-local: install-argocd-openshift devenv-docker reset-db ## Setup steps for E2E tests to run with Local Openshift Cluster

start-e2e: start ## Start the managed gitops processes for E2E tests. At the moment this is just a wrapper over 'start' target

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
	cd $(MAKEFILE_ROOT)/appstudio-shared && go mod vendor
	cd $(MAKEFILE_ROOT)/backend-shared && go mod vendor
	cd $(MAKEFILE_ROOT)/backend && go mod vendor
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod vendor
	cd $(MAKEFILE_ROOT)/appstudio-controller && go mod vendor	
	cd $(MAKEFILE_ROOT)/tests-e2e && go mod vendor	
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go mod vendor	

tidy: ## Tidy all components
	cd $(MAKEFILE_ROOT)/appstudio-shared && go mod tidy
	cd $(MAKEFILE_ROOT)/backend-shared && go mod tidy
	cd $(MAKEFILE_ROOT)/backend && go mod tidy 
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod tidy
	cd $(MAKEFILE_ROOT)/appstudio-controller && go mod tidy
	cd $(MAKEFILE_ROOT)/tests-e2e && go mod tidy
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go mod tidy
	 
fmt: ## Run 'go fmt' on all components
	cd $(MAKEFILE_ROOT)/appstudio-shared && make fmt
	cd $(MAKEFILE_ROOT)/backend-shared && make fmt
	cd $(MAKEFILE_ROOT)/backend && make fmt
	cd $(MAKEFILE_ROOT)/cluster-agent && make fmt
	cd $(MAKEFILE_ROOT)/appstudio-controller && make fmt
	cd $(MAKEFILE_ROOT)/utilities/db-migration && make fmt

generate-manifests: ## Call the 'generate' and 'manifests' targets of every project
	cd $(MAKEFILE_ROOT)/appstudio-shared && make generate manifests
	cd $(MAKEFILE_ROOT)/backend-shared && make generate manifests
	cd $(MAKEFILE_ROOT)/backend && make generate manifests
	cd $(MAKEFILE_ROOT)/cluster-agent && make generate manifests
	cd $(MAKEFILE_ROOT)/appstudio-controller && make generate manifests

db-migrate:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go

db-drop:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go drop

db-drop_smtable:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go drop_smtable

db-migrate-downgrade:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go downgrade_migration

db-migrate-upgrade:
	cd $(MAKEFILE_ROOT)/utilities/db-migration && go run main.go upgrade_migration

db-schema: ## Run db-schema varchar tests
	cd $(MAKEFILE_ROOT)/backend-shared && go run ./hack/db-schema-sync-check

### --- K C P --- ###

gen-kcp-api-backend-shared: ## Runs utilities/generate-kcp-api-backend-shared.sh to generate kcp api resource schema and export
	cd $(MAKEFILE_ROOT)/utilities && ./generate-kcp-api-backend-shared.sh

gen-kcp-api-appstudio-shared: ## Runs utilities/generate-kcp-api-appstudio-shared.sh to generate kcp api resource schema and export
	cd $(MAKEFILE_ROOT)/utilities && ./generate-kcp-api-appstudio-shared.sh

kcp-test-local-e2e: ## Initiates a ckcp within openshift cluster and runs e2e test
	cd $(MAKEFILE_ROOT)/kcp && ./ckcp/setup-ckcp-on-openshift.sh

gen-kcp-api-all: gen-kcp-api-appstudio-shared gen-kcp-api-backend-shared ## Creates all the KCP API Resources for all comfig/crds

apply-kcp-api-all: ## Apply all APIExport to the cluster
	$(MAKEFILE_ROOT)/utilities/create-apiexports.sh

setup-e2e-kcp-virtual-workspace: ## Sets up the necessary KCP virtual workspaces
	$(MAKEFILE_ROOT)/kcp/kcp-e2e/setup-ws-e2e.sh

start-e2e-kcp-virtual-workspace: ## Starts gitops service in service-provider virtual KCP workspace
	$(MAKEFILE_ROOT)/kcp/kcp-e2e/start-ws-e2e.sh

test-e2e-kcp-virtual-workspace: ## Test E2E against KCP virtual workspaces
	KUBECONFIG_SERVICE_PROVIDER=/tmp/service-provider-workspace.yaml KUBECONFIG_USER_WORKSPACE=/tmp/user-workspace.yaml  make test-e2e


### --- Utilities for other makefile targets ---

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
