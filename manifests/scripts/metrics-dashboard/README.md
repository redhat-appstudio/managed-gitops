# Enable Prometheus logging of GitOps Service in 'gitops' namespace, and install Grafana 

This shell script will:
- Enable (Prometheus) user monitoring of projects on the cluster. This allows us to use the OpenShift cluster's prometheus, rather than installing our own
- Install a new Grafana instance into 'grafana' namespace. OpenShift doesn't have a built-in Grafana instance (it has its own metrics graphing mechanism via OpenShift Console)
- Enables Prometheus to scrape resources from OpenShift GitOps
- Creates the [Argo CD default Grafana Dashboard](https://argo-cd.readthedocs.io/en/stable/operator-manual/metrics/#dashboards)


### How to Use
1) Acquire an OpenShift Cluster (for example, cluster bot) and log into it.
2) Install OpenShift GitOps operator to it. (This is required because the install script will attempt to enable Prometheus integration on GitOps Operator)
    - For example, by running `make install-argocd-openshift`
3) Run the `run.sh` install script in this directory.