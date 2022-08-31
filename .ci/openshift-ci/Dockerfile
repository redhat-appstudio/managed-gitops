FROM registry.ci.openshift.org/openshift/release:golang-1.18

# make sure Go doesn't use the vendors folder, unless we want to
ENV GOFLAGS=""

SHELL ["/bin/bash", "-c"]

# Install yq, kubectl, postgresql-server
RUN wget https://github.com/mikefarah/yq/releases/download/v4.20.2/yq_linux_amd64 -O /usr/local/bin/yq && \
    chmod +x /usr/local/bin/yq && yq --version && \
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/local/bin && \
    yum -y install postgresql-server
