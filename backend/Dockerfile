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
COPY config/ config/
COPY routes/ routes/
COPY util/ util/

# To compile/build the go applications
RUN make build
RUN ./dist/managed-gitops-backend

# After container is configured, commands to run
CMD [ "/gitops-backend" ]
