# # VERSION defines the project version for the bundle.
# # Update this value when you upgrade the version of your project.
# # To re-generate a bundle for another specific version without changing the standard setup, you can:
# # - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# # - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
# VERSION ?= 0.0.1

# # CHANNELS define the bundle channels used in the bundle.
# # Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# # To re-generate a bundle for other specific channels without changing the standard setup, you can:
# # - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# # - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
# ifneq ($(origin CHANNELS), undefined)
# BUNDLE_CHANNELS := --channels=$(CHANNELS)
# endif

# # DEFAULT_CHANNEL defines the default channel used in the bundle.
# # Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# # To re-generate a bundle for any other default channel without changing the default setup, you can:
# # - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# # - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
# ifneq ($(origin DEFAULT_CHANNEL), undefined)
# BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
# endif
# BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# # IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# # This variable is used to construct full image tags for bundle and catalog images.
# #
# # For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# # redhat.com/cluster-agent-bundle:$VERSION and redhat.com/cluster-agent-catalog:$VERSION.
# IMAGE_TAG_BASE ?= redhat.com/cluster-agent

# # BUNDLE_IMG defines the image:tag used for the bundle.
# # You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
# BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# # Image URL to use all building/pushing image targets
# IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Get the OS and ARCH values to be used for building the binary.
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."








shared-manifests: controller-gen ## Cluster-agent consumes the custom resources from backend-shared, so ensure the manifests are updated there.
	cd ../backend-shared; make manifests; cd ../cluster-agent

shared-generate: controller-gen ## Cluster-agent consumes the resource code from backend-shared, so ensure the model code is generate there.
	cd ../backend-shared; make generate; cd ../cluster-agent

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: fmt vet envtest ## Run tests.
	ACK_GINKGO_DEPRECATIONS=1.16.4 KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true go test -p=1 ./... -coverpkg=./... -timeout=3m -coverprofile cover.out

##@ Build

build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
	ENABLE_APPPROJECT_ISOLATION=true DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true go run ./main.go --zap-log-level info --zap-time-encoding=rfc3339nano
# more on controller log level configuration: https://sdk.operatorframework.io/docs/building-operators/golang/references/logging/

run-no-self-heal: manifests generate fmt vet ## Run a controller from your host.
	SELF_HEAL_INTERVAL=0 ENABLE_APPPROJECT_ISOLATION=true DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true  KUBECONFIG=${WORKLOAD_KUBECONFIG} go run ./main.go --zap-log-level info --zap-time-encoding=rfc3339nano

runexec: ## Run a controller from your host using exe in current folder
ifeq (,$(wildcard ./main))
runexec: manifests generate fmt vet
	@echo Building and running cluster-agent
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build ./main.go
	ENABLE_APPPROJECT_ISOLATION=true DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true main --zap-log-level info --zap-time-encoding=rfc3339nano
else
runexec:
	@echo Running cluster-agent using existing main executable.
	 ENABLE_APPPROJECT_ISOLATION=true DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true main --zap-log-level info --zap-time-encoding=rfc3339nano
endif

chaos-run: manifests generate fmt vet ## Chaos Engineering: Simulate a controller that restarts every ~33 seconds.
	go build -o /tmp/cluster-agent-manager-chaos main.go
	ENABLE_APPPROJECT_ISOLATION=true KILL_INTERVAL="45" KILL_INITIAL="10" ../utilities/random-kill.sh /tmp/cluster-agent-manager-chaos --zap-log-level info --zap-time-encoding=rfc3339nano
	rm /tmp/cluster-agent-manager-chaos

# docker-build: test ## Build docker image with the manager.
# 	docker build -t ${IMG} .

# docker-push: ## Push docker image with the manager.
# 	docker push ${IMG}

##@ Deployment

# install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
# 	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
# 	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
# 	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
# 	$(KUSTOMIZE) build config/default | kubectl apply -f -

# undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
# 	$(KUSTOMIZE) build config/default | kubectl delete -f -


CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

GO_SEC = $(shell pwd)/bin/gosec
go_sec: ## Download gosec locally if necessary.
	$(call go-get-tool,$(GO_SEC),github.com/securego/gosec/v2/cmd/gosec@latest)


ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

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

# .PHONY: bundle
# bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
# 	operator-sdk generate kustomize manifests -q
# 	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
# 	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
# 	operator-sdk bundle validate ./bundle

# .PHONY: bundle-build
# bundle-build: ## Build the bundle image.
# 	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

# .PHONY: bundle-push
# bundle-push: ## Push the bundle image.
# 	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: lint
lint:
	golangci-lint --version
	GOMAXPROCS=2 golangci-lint run --fix --verbose --timeout 300s
	
# .PHONY: opm
# OPM = ./bin/opm
# opm: ## Download opm locally if necessary.
# ifeq (,$(wildcard $(OPM)))
# ifeq (,$(shell which opm 2>/dev/null))
# 	@{ \
# 	set -e ;\
# 	mkdir -p $(dir $(OPM)) ;\
# 	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
# 	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
# 	chmod +x $(OPM) ;\
# 	}
# else
# OPM = $(shell which opm)
# endif
# endif

# # A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# # These images MUST exist in a registry and be pull-able.
# BUNDLE_IMGS ?= $(BUNDLE_IMG)

# # The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
# CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# # Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
# ifneq ($(origin CATALOG_BASE_IMG), undefined)
# FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
# endif

# # Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# # This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# # https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
# .PHONY: catalog-build
# catalog-build: opm ## Build a catalog image.
# 	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# # Push the catalog image.
# .PHONY: catalog-push
# catalog-push: ## Push a catalog image.
# 	$(MAKE) docker-push IMG=$(CATALOG_IMG)

.PHONY: gosec
gosec: go_sec
	$(GO_SEC) ./...


# Remove the vendor and bin folders
.PHONY: clean
clean:
	rm -rf vendor/ bin/

# Remove the vendor and bin folders
.PHONY: clean-exec
clean-exec:
	rm main

mocks: ##  generate mocks
	mockery --srcpkg=github.com/argoproj/argo-cd/v2/pkg/apiclient/application --name=ApplicationServiceClient --output $(shell pwd)/utils/mocks
	mockery --srcpkg=github.com/argoproj/argo-cd/v2/pkg/apiclient --name=Client --output $(shell pwd)/utils/mocks
	mockery --srcpkg=github.com/argoproj/argo-cd/v2/pkg/apiclient/session --name=SessionServiceClient --output $(shell pwd)/utils/mocks
	mockery --srcpkg=github.com/argoproj/argo-cd/v2/pkg/apiclient/settings --name=SettingsServiceClient --output $(shell pwd)/utils/mocks
	mockery --srcpkg=github.com/argoproj/argo-cd/v2/pkg/apiclient/version --name=VersionServiceClient --output $(shell pwd)/utils/mocks
