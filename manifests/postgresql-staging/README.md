# PostgreSQL GitOps Staging YAML

For Postgresql, there are a few ways to install it on Kubernetes, so we have some options for how we want to install it to the staging environment. We could use a PostgreSQL operator, Helm chart, or just flat YAML.

Here, I'm suggesting we just use plain YAML to install PostgreSQL, but for sake of simplicity that plain YAML file will be rendered from the Postgresql Helm Chart from Bitnami.

Thus I have created a script to convert Helm chart (plus values.yaml file) into a YAML file, and then we check that YAML file into the repository (rather than invoking Helm directly in order to install postgres).


## To regenerate the YAML to the latest Helm chart

The 'regenerate-manifests.sh' file will download the Helm chart, look in the values.yaml for parameters to use, and then output the result to `postgres-staging.yaml`.

## Installation

Then, to install PostgreSQL onto the cluster, all someone needs to do is:
```
kubectl create namespace gitops-postgresql
kubectl apply -n gitops-postgresql postgresql-staging.yaml
```

