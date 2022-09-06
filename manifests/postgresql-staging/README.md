# PostgreSQL Database for GitOps 

## Support

The Red Hat suported image `registry.redhat.io/rhel8/postgresql-12` is used in this setup via the `ImageStreamTag` made
available for all OpenShift Container Platform users.


## Installation

Then, to install PostgreSQL onto the cluster, all someone needs to do is:
```
kubectl apply -f postgresql-staging.yaml

# Edit the staging-secret to include a new base-64 encoded password

kubectl apply -f postgresql-staging-secret.yaml
```

The installation yaml could also be checked into a GitOps respository, subject to careful handling of credentials.


Note: kcp (till release-0.7) has no knowledge of default protocols, and hence we need to explicitly define them. In this case, make sure to run `.addProtocol.sh` if any changes are made to the postgres-staging.yaml. 