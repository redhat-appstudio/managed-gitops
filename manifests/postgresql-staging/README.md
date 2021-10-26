# PostgreSQL Database for GitOps 

## Support

The Red Hat suported image `registry.redhat.io/rhel8/postgresql-10:latest` is used in this setup via the `ImageStreamTag` made
available for all OpenShift Container Platform users.


## Installation

Then, to install PostgreSQL onto the cluster, all someone needs to do is:
```
kubectl apply postgresql-staging.yaml
```

The installation yaml could also be checked into a GitOps respository, subject to careful handling of credentials.
