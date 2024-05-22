################################################################################################
# Builder image
# Initial stage which pulls and prepares any required build dependencies for the whole monorepo
# Clones the whole monorepo and builds all the GitOps Service binaries using the Makefile
#
# Note: Make sure you use '.dockerignore' to avoid local copy of binaries (e.g. controller-gen)
################################################################################################
FROM golang:1.22 as builder

ARG OS=linux
ARG ARCH=amd64

WORKDIR /workspace

COPY Makefile ./Makefile
COPY backend ./backend
COPY backend-shared ./backend-shared
COPY cluster-agent ./cluster-agent
COPY appstudio-controller ./appstudio-controller
COPY utilities/db-migration/ ./utilities/db-migration/
COPY utilities/init-container/ ./utilities/init-container/

# Perform the build for all components
RUN make build

################################################################################################
# GitOps Service image
# Provides both the 'gitops-service-backend' and 'gitops-service-cluster-agent' components
################################################################################################
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10-896 as gitops-service

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

RUN mkdir -p /migrations
RUN mkdir -p /init-container

# Add Amazon public CA certs to system root CA, to allow us to validate Amazon RDS TLS connections.
# - More information on Amazon TLS certs is available here: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html
RUN curl -o /etc/pki/ca-trust/source/anchors/global-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem \
	&& update-ca-trust

# Copy both the controller binaries into the $PATH so they can be invoked
COPY --from=builder workspace/backend/bin/manager /usr/local/bin/gitops-service-backend
COPY --from=builder workspace/cluster-agent/bin/manager /usr/local/bin/gitops-service-cluster-agent
COPY --from=builder workspace/appstudio-controller/bin/manager /usr/local/bin/appstudio-controller
COPY --from=builder workspace/utilities/init-container/bin/init-container /init-container

# Copy the database migration versions
COPY --from=builder workspace/utilities/db-migration/migrations /migrations

# Run as non-root user
USER 65532:65532
