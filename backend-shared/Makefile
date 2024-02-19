help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.5)

GO_SEC = $(shell pwd)/bin/gosec
go_sec: ## Download gosec locally if necessary.
	$(call go-get-tool,$(GO_SEC),github.com/securego/gosec/v2/cmd/gosec@latest)

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

.PHONY: gosec
gosec: go_sec
	$(GO_SEC) ./...

.PHONY: lint
lint:
	golangci-lint --version
	GOMAXPROCS=2 golangci-lint run --fix --verbose --timeout 300s

# Run go fmt against code
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

# Remove the vendor and bin folders
.PHONY: clean
clean:
	rm -rf vendor/ bin/

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.21

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

test: fmt vet envtest ## Run tests.
	ACK_GINKGO_DEPRECATIONS=1.16.4 KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true go test -p=1 ./... -coverpkg=./... -coverprofile cover.out

mocks: ##  generate mocks
	cd $(shell pwd)/util/mocks && mockgen -build_flags=--mod=mod -destination=mock_client.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client Client
	cd $(shell pwd)/util/mocks && mockgen -build_flags=--mod=mod -destination=mock_database_queries.go -package=mocks github.com/redhat-appstudio/managed-gitops/backend-shared/db DatabaseQueries
	cd $(shell pwd)/util/log/mocks && mockgen -build_flags=--mod=mod -destination=log_sink_mock.go -package=mocks github.com/go-logr/logr LogSink
	cd $(shell pwd)/apis/managed-gitops/v1alpha1/mocks && mockgen -build_flags=--mod=mod -destination=status-writer.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client StatusWriter
	cd $(shell pwd)/apis/managed-gitops/v1alpha1/mocks && mockgen -build_flags=--mod=mod -destination=cr-client.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client Client

