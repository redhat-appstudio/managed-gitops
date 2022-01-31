# AppStudio GitOps Service M2 Demo

**Note: This demo hardcodes specific 'known good' commits of the managed-gitops repo and infra-deployments repo, for demo purposes.** If you wish to test on the latest code, remove the `git checkout` statements from `setup-on-openshift.sh`.

## Setup the demo

1) Acquire a disposable OpenShift cluster from cluster-bot. (Send `launch` command to *cluster-bot* user on Red Hat Slack). 
    - You _can_ use your own cluster, but a disposable cluster-bot cluster should prevent the setup script from affecting your personal cluster settings.

2) Wait for the cluster to orchestate, then `oc login` from the CLI.

3) Next, log in to the OpenShift cluster console UI.
    - This sets the OpenShift login credentials in your browser, which we will use to log in to Argo CD in a later step.

4) Run `setup-on-openshift.sh`
    - After a few moments, verify that:
        - OpenShift GitOps (Argo CD) is successfully running in `openshift-gitops` namespace.
        - 'managed-gitops-postgres'  Docker container is running successfully on your local machine (do a `docker ps` and look for this container)
        - The GitOps service processes start without error, and continue to run on your terminal.
            - Note: the `go vet` commands may take a few moments to download/compile dependencies (this only occurs on first run)
        - There are no error messages output by the GitOps service components (only _INFO_ and _DEBUG_ statments in the console output)

5) Log in to Argo CD Web UI
    - Get the URL for Argo CD by running: `kubectl get route -n openshift-gitops openshift-gitops-server`
    - Open the URL in your browser, then click *Login with OpenShift*. The login should use your OpenShift console credentials from step 2.
    - Verify that:
        - You see the Argo CD application lists (with no applications listed)

## Run the demo

This demo is fully built-around the `GitOpsDeployment` CR, which describes an active continous deployment from a GitOps repository, to a namespace (workspace):
```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment

metadata:
  name: gitops-depl
  namespace: jgw  # user jgw

spec:

  # Application/component to deploy
  source:
    repoURL: https://github.com/redhat-appstudio/gitops-repository-template
    path: environments/overlays/dev

  # destination: {}  # destination is user namespace if empty

  # Only 'automated' type is currently supported: changes to the GitOps repo immediately take effect (as soon as Argo CD detects them).
  type: automated
```



Create a GitOpsDeployment in the *jane* namespace (workspace), for our user 'jane':
```bash
kubectl apply -f jane-deployment.yaml
```

Observe that:
- The corresponding Argo CD Application CR has been created, and points to jane namespace
- It has synced to the jane namespace
- Within the 'jane' namespace, you can see all the deployed resources (Deployment, Service, ConfigMap, Route)
- Various DEBUG/INFO log messages are output to the console by backend and cluster-agent, corresponding to behind-the-scenes actions.

Create a GitOpsDeployment in the jgw namespace (workspace), for our second tenant 'jgw':
```bash
kubectl apply -f jgw-deployment.yaml
```

Same observations as above, but notice that the deployment target is different. Each user has their own deployments, specified in their own namespace, deployed to their own namespace, deployed using a shared Argo CD.

Next, you can change fields in the `GitOpsDeployment` CR, such as the path within the target GitOps repository:
````bash
# Edit jgw-deployment.yaml
vi jgw-deployment.yaml

# Next, change the path from 'dev' to 'staging':
#
# spec:
#   source:
#     path: environments/overlays/dev
# to
#
# spec:
#   source:
#     path: environments/overlays/staging

# Apply the change
kubectl apply -f jgw-deployment.yaml
````


Observe that:
- The GitOps service quickly detects the change, and updates the Argo CD Application
- In the Argo CD UI, the corresponding Argo CD application and its resources are updated.
- The deployed resources in 'jgw' namespace are updated.

You can likewise delete the GitOpsDeployment CR...

```
kubectl delete -f jgw-deployment.yaml
```

... and observe that:
- The Argo CD Application is likewise cleaned up, along with deployed resources.

At any step in the process, you can also view the database internals:
- Run `psql.sh` from the _managed-gitops_ repo.
- Some example queries:
    - See Argo CD Application entries: `select * from application;`
        - _(Don't forget the semi-colon at the end of these statements!)_
    - See completed operations: `select * from operation;` 
    - See Argo CD instance info: `select * from gitopsengineinstance;`
    - See deployment targets: `select * from managedenvironment;`
    - See relationship between GitOpsDeploymentSync CRs and their corresponding Application DB entry: `select * from deploymenttoapplicationmapping;`
    - (etc)
- As a rule: all changes made to Argo CD are first made to the database, with the cluster-agent responsible for reconciling database state -> Argo CD state.

## Cleanup the demo

To cleanup the demo, just remove the local containers from your workspace:
```bash
docker rm -f managed-gitops-pgadmin managed-gitops-postgres
```

## Questions and troubleshooting

Something went wrong? Let us know on [#forum-gitops-service](https://coreos.slack.com/archives/C02C3SE8QS2) on Red Hat Slack!
