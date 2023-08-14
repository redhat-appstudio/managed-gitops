FROM registry.ci.openshift.org/openshift/release:golang-1.19

# make sure Go doesn't use the vendors folder, unless we want to
ENV GOFLAGS=""

SHELL ["/bin/bash", "-c"]

# Install yq, kubectl, postgresql-server, argocd cli
RUN curl -sSL -o /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 && \
    chmod +x /usr/local/bin/yq && yq --version && \
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/local/bin && \
    yum -y install postgresql-server && \
    curl -sSL -o /usr/local/bin/argocd https://github.com/argoproj/argo-cd/releases/latest/download/argocd-linux-amd64 && chmod +x /usr/local/bin/argocd
