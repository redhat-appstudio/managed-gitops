# managed-gitops

## Overview

For an introduction to the project, and project details, see [Google Drive](https://drive.google.com/drive/u/0/folders/1p_yIOJ1WLu-lqz-BVDn076l1K1pEOc1d).


### Components

There are 4 separated, tighly-coupled components contained within this repository:
- **Backend**: Simple Go module for listening for REST requests
- **Backend Shared**: Simple Go module for sharing backend schema between operator and backend
- **Cluster Agent**: A Kubernetes controller/operator that lives on the cluster and handles cluster operations.
    - Based on operator-sdk v1.11
- **Frontend**: [PatternFly](https://www.patternfly.org/)/React-based UI, based on PatternFly React Seed
- **Utilities**: Standalone utilities for working with the project.

### Development Environment

To setup and run your development environment:
- Ensure you are targetting a local Kubernetes cluster (I personally use [kubectx/kubens](https://github.com/ahmetb/kubectx) to manage Kubernetes context)
- Run `make install` to ensure that the target cluster has the appropriate GitOps service CRs installed, and that Argo CD is installed in the 'argocd' namespae.
- Run `create-dev-env.sh` to start Postgresql (see `make reset-db` if you want to reset this to a clean slate)

This repository contains scripts which may be used to setup/run the database environment:
- **`create-dev-env.sh`**: Start up PostgreSQL in a docker container, with the database initialized. 
    - Also starts `pg-admin`, a web-based tool for viewing and administering PostgreSQL database.
    - See `create-dev-env.sh` for login/password and connection info.
- **`(delete/stop)-dev-env.sh`**: Stop or delete the database.
- **`db-schema.sql`**: The database schema used by the components
- **`psql.sh`**: Allows you to interact with the DB from the command line. Requires `psql` CLI util to be installed on your local machine (for example, by installing PostgreSQL)
    - Once inside the psql CLI, you may issue SQL statements, such as `select * from application;` (don't forget the semi-colon at the end, `;`!)

See also, the targets in `Makefile` for additional available operations. Plus, within each of the components is a `Makefile`, which can be used for local development of that component.     


