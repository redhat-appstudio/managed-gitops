# managed-gitops

## Overview

....For an introduction to the project, and project details, see [Google Drive](https://drive.google.com/drive/u/0/folders/1p_yIOJ1WLu-lqz-BVDn076l1K1pEOc1d).


### Components

There are 4 separated, tighly-coupled components contained within this repository:
- **Backend**: Simple Go module for listening for REST requests
- **Backend Shared**: Simple Go module for sharing backend schema between operator and backend
- **Cluster Agent**: A Kubernetes controller/operator that lives on the cluster and handles cluster operations.
    - Based on operator-sdk v1.11
- **Frontend**: [PatternFly](https://www.patternfly.org/)/React-based UI, based on PatternFly React Seed
- **Utilities**: Standalone utilities for working with the project.

### Development Environment

This repository contains scripts which may be used to setup/run the development environment:
- **`create-dev-env.sh`**: Start up PostgreSQL in a docker container, with the database initialized. 
    - Also starts `pg-admin`, a web-based tool for viewing and administering PostgreSQL database.
    - See `create-dev-env.sh` for login/password and connection info.
- **`(delete/stop)-dev-env.sh`**: Stop or delete the database.
- **`db-schema.sql`**: The database schema used by the components
- **`psql.sh`**: Allows you to interact with the DB from the command line. Requires `psql` CLI util to be installed on your local machine (for example, by installing PostgreSQL)

