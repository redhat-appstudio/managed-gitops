
# Managed GitOps Cluster Agent


## Development

#### For local development, use the following to build and run the controller:
```bash

cd (repo root)/cluster-agent

# Install the CustomResourceDefinitions (CRDs) to your cluster
# - Run this every time the CRD changes
make install

# Build the controller, storing it in bin/manager
make build

# Run the controller
make run

```

#### To run unit tests:
```bash
make test
```



## Bootstrap parameters

Bootstrapped using the following commands:
```
go mod init github.com/jgwest/managed-gitops/cluster-agent

operator-sdk-v1.11 init --domain redhat.com

operator-sdk-v1.11 create api --group managed-gitops --version v1alpha1 --kind Operation --resource --controller

```


