
# GitOps Service Metrics Dashboards

Contained within this folder are dashboards used to monitor the GitOps Service.

There are two use cases for monitoring the GitOps Service:
- **Stonesoup Clusters**: Monitoring the GitOps Service using the Grafana/Prometheus combination that is installed on the Stonesoup cluster(s)
- **Local/Dev Clusters**: When GitOps Service developers want to monitor the service on their own cluster, and/or want to create/modify/debug new metrics, or dashboard panels.

HOWEVER, the same dashboard cannot be used for both use cases, AFAICT: the Grafana datasource ID that is required differs between the two, which means a dashboard that works for one will not work for the other.
- managed-gitops datasource: PBFA97CFB590B2093
- stonesoup datasource: PF224BEF3374A25F8

My 'solution' is a shell script that converts the datasource ID from local/dev dashboard -> Stonesoup dashboard, using 'sed'.

## Steps to regenerate Stonesoup dashboard

The `managed-gitops/gitops-dashboard.json` can be developed/modified/checked in to the managed-gitops repo, with new changes. This dashboard is the 'single source of truth' for the GitOps Service dashboard.

Once a change is made to `managed-gitops/gitops-dashboard.json`, we will want to deploy that to the Stonesoup Grafana.

In order to update the Stonesoup version of the dashboard, run the `regenerate-stonesoup-dashboard.sh` script, to regenerate the `stonesoup/gitops-dashboard.json` file. 
- This will overwrite any changes in `stonesoup/gitops-dashboard.json`, replacing the contents with the latest from `managed-gitops/gitops-dashboard.json`
- The data source will be updated from the managed-gitops data source, to the Stonesoup datasource


