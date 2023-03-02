# Get the OS and ARCH values to be used for building the binary.
OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)

migration-script:
	cd migrations && migrate create -ext sql -seq $(filename)

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

.PHONY: build
build: fmt vet ## Build the DB migration binary
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o bin/db-migrate main.go
