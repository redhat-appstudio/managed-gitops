
.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/init-container main.go


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

test: fmt vet ## Run tests.
	DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL=true go test -timeout=2m -p=1 ./... -coverprofile cover.out -coverpkg=./...


# Remove the vendor and bin folders
.PHONY: clean
clean:
	rm -rf vendor/ bin/
