ARG BASE_IMAGE=registry.access.redhat.com/ubi8/ubi-minimal:8.5-218
################################################################################################
# Builder image
# Initial stage which pulls and prepares any required build dependencies for the whole monorepo
# Clones the whole monorepo and builds all the GitOps Service binaries using the Makefile
#
# Note: Make sure you use '.dockerignore' to avoid local copy of binaries (e.g. controller-gen)
################################################################################################
FROM golang:1.17 as builder

WORKDIR /workspace

COPY Makefile ./Makefile
COPY backend ./backend
COPY backend-shared ./backend-shared
COPY cluster-agent ./cluster-agent
COPY appstudio-controller ./appstudio-controller
COPY appstudio-shared ./appstudio-shared

# Perform the build for all components
RUN make build

################################################################################################
# GitOps Service image
# Provides both the 'gitops-service-backend' and 'gitops-service-cluster-agent' components
################################################################################################
FROM $BASE_IMAGE as gitops-service

# Install the 'shadow-utils' which contains `adduser` and `groupadd` binaries
RUN microdnf install shadow-utils \
	&& groupadd --gid 65532 nonroot \
	&& adduser \
		--no-create-home \
		--no-user-group \
		--uid 65532 \
		--gid 65532 \
		nonroot

WORKDIR /

# Copy both the controller binaries into the $PATH so they can be invoked
COPY --from=builder workspace/backend/bin/manager /usr/local/bin/gitops-service-backend
COPY --from=builder workspace/cluster-agent/bin/manager /usr/local/bin/gitops-service-cluster-agent
COPY --from=builder workspace/appstudio-controller/bin/manager /usr/local/bin/appstudio-controller

# Run as non-root user
USER 65532:65532
