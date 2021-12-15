
# Managed GitOps Backend


## Development

#### For local development, use the following to build and run the controller:
```bash

cd (repo root)/backend

# Build the binary
make build

# Run the binary
./dist/managed-gitops-backend

```

#### To run unit tests:
```bash
make test
```
## Bootstrap parameters

Bootstrapped using the following commands:
```
go mod init github.com/redhat-appstudio/managed-gitops/backend

operator-sdk-v1.11 init --domain redhat.com

# Next, edit the PROJECT file and add:
# multigroup: true
# to an empty line.

# Then, run:

operator-sdk-v1.11 create api --group managed-gitops --version v1alpha1 --kind GitOpsDeployment --resource --controller
operator-sdk-v1.11 create api --group managed-gitops --version v1alpha1 --kind GitOpsDeploymentSyncRun --resource --controller
 

```
