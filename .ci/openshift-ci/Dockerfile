FROM registry.ci.openshift.org/openshift/release:golang-1.17

# make sure Go doesn't use the vendors folder, unless we want to
ENV GOFLAGS=""

SHELL ["/bin/bash", "-c"]

# Install kubectl, postgresql-server
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/local/bin && \
    yum -y install postgresql-server
