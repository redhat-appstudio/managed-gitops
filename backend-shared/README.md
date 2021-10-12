
# Managed GitOps Backend Shared

## Overview

This repository is a simple go module created for sharing backend schema between operator and backend. 

The module consists of the following:
- go database structs
- postgres connection code

## Development

#### For local development, use the following to build and run the controller:
```bash

cd (repo root)/backend-shared

# Build the binary
make build

# Run the binary
./dist/managed-gitops-backend

```

#### To run unit tests:
```bash
make test
```
