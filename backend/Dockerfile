# Build the manager binary
FROM golang:1.16 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Download the go module artificats in the WORKDIR (similar to go mod download working locally)
RUN go mod download

# Copy the go source
COPY *.go ./
COPY routes/ routes/
COPY util/ util/

# To compile/build the go applications
RUN CGO_ENABLED=0 go build -a -o ./dist/managed-gitops-backend main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.4-210
WORKDIR /
COPY --from=builder /workspace/dist/managed-gitops-backend .

# After container is configured, commands to run
CMD [ "/gitops-backend" ]
