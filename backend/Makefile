IMAGE_NAME?=quay.io/redhat-appstudio/gitops-service
IMAGE_TAG?=latest

.PHONY: build
build: fmt vet
	CGO_ENABLED=0 go build -o ./dist/managed-gitops-backend .

.PHONY: test
test: fmt vet
	go test -coverpkg=./... -v -race -count=1 -coverprofile=coverage.out `go list ./...`

.PHONY: gosec
gosec: 
	# Run this command, to install gosec, if not installed:
	# go get -u github.com/securego/gosec/v2/cmd/gosec
	gosec ./...

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

# Run Dockerfile for automating the container build
.PHONY: image
image: test
	docker build -t ${IMAGE_NAME}:${IMAGE_TAG} .

.PHONY: image-push
image-push: image
	docker push ${IMAGE_NAME}:${IMAGE_TAG}