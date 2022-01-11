
MAKEFILE_ROOT=$(shell pwd)

# install: Ensure that the Argo CD namespace exists, that Argo CD is installed, and that CRDs we are using are applied to the current cluster
install:
	kubectl apply -f $(MAKEFILE_ROOT)/backend/config/crd/bases
	kubectl apply -f $(MAKEFILE_ROOT)/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
	kubectl create ns argocd || true
	kubectl apply -f https://raw.githubusercontent.com/argoproj/argo-cd/release-2.2/manifests/crds/application-crd.yaml

# start: start all the components
# ensure goreman is installed, with 'go install github.com/mattn/goreman@latest'
start:
	~/go/bin/goreman start

# build: build all the components (note: you do not need to do this before running start; 'start' will compile and run)
build:
	cd $(MAKEFILE_ROOT)/backend && make build
	cd $(MAKEFILE_ROOT)/cluster-agent && make build

# test: run tests for each component
test:
	cd $(MAKEFILE_ROOT)/backend && make test
	cd $(MAKEFILE_ROOT)/backend-shared && make test	
	cd $(MAKEFILE_ROOT)/cluster-agent && make test

# download-deps: Download goreman to ~/go/bin
download-deps:
	go install github.com/mattn/goreman@latest


# reset-db: Erase the current database, and reset it scratch; useful during development if you want a clean slate.
reset-db:
	$(MAKEFILE_ROOT)/delete-dev-env.sh
	$(MAKEFILE_ROOT)/create-dev-env.sh

vendor:
	cd $(MAKEFILE_ROOT)/backend-shared && go mod vendor
	cd $(MAKEFILE_ROOT)/backend && go mod vendor
	cd $(MAKEFILE_ROOT)/cluster-agent && go mod vendor

