# Initial AppStudio GitOps API and Deployment Workflows 

### Written by
- Jonathan West (@jgwest)

# Definitions

## Deployment

Deployment is an action that:

- Ensures that the resources on the cluster are consistent with the Application definition within AppStudio.

  - _Example_: If a user says they want a new environment variable 'A=B' in the UI, then clicks Deploy, the corresponding environment variable update should ultimately make it to the Deployment/Pod resources on the target cluster.

- Ensures that the Application resources are deployed to the cluster (Deployments/ConfigMaps/etc)

  - During deployment, resources defined in the GitOps repository are synchronized with the cluster (by Argo CD). This includes creation of missing resources, updating existing resources that are out-of-sync, and deleting removed resources that have been removed from Git.

  - _Example_: A user's K8s application resources (ConfigMaps/Secrets/Deployment/etc) will live in the GitOps repo. When the user makes a change (via AppStudio UI), deploy action should cause all of these resources to be deployed to the target cluster.

  - _Example_: Not supported at this stage, but if a user manually commits to the repo (‘git add ; git commit’), deploy should cause those committed resources to be updated on the corresponding K8s cluster resources.

- Ensures that the latest container image (from the build service) is referenced in the Deployment K8s resource

  - _Example_: If a user triggers a container build via a source code change, the deploy action should ensure that the built image is running on the cluster (eg as a container, referenced by name:tag, in the Deployment)

TLDR: Ensures that K8s resources on the cluster are synchronized with the AppStudio App definition, and synchronized with the K8s resources on the GitOps repository.


## Manual Deployment

A deployment (defined as above) that is triggered by a user clicking a 'Manual Deploy' button within the UI.

- It has been generally agreed that clicking the manual deploy button will both perform a build via the build service, and then deploy it. (ie there is no separate ‘Manual build’ button, alongside ‘Manual Deploy’. It is all one ‘(Manual) Deploy’ button).


## Automated Deployment

A deployment (defined as above) that is triggered by, at least one of:

- Directly:

  - A change to one of the K8s resources (ConfigMap, Deployment, etc) within the GitOps repo (For the MVP timeframe, it should only be AppStudio that is updating this repo).

    - GitOps service should detect this and create a GitOpsDeploymentSyncRun CR.

  - A manual deployment action is triggered from the UI (which causes a GitOpsDeploymentSyncRun CR to be created)

- Indirectly:

  - When a change is made to the Application CR via the AppStudio UI, this should cause the GitOps repository to be updated, which will cause a deployment.

  - When a build service completes a build for this application (indicating, eg, new source code that needs to be deployed), this container image should be upserted into the GitOps repository Deployment/Pod resource, which will cause a deployment to be triggered (if deployments are automated).

  - The user provides a container image to deploy (‘import from an image, not from source’). This container image should be upserted into the GitOps repository, which will cause a deployment to be triggered (if deployments are automated).

    

If a user opts into automated deployments, then it is expected that any manual changes they make to K8s resources (eg via kubectl/oc) outside of AppStudio UI, will be replaced by the deploy action (Argo CD synchronize operation)

- (There was significant discussion during the F2F over whether the user should _even be able_ to perform manual changes to the cluster; I’m not capturing that discussion here.)


# GitOps Service APIs

**GitOpsDeployment CR:**

Create this CR to enable synchronization between a Git repository, and a KCP workspace:


```yaml
apiVersion: v1alpha1
kind: GitOpsDeployment
metadata:
  name: jgwest-app
spec:

  # GitHub repository containing K8s resources to deploy
  source:
    repoURL: https://github.com/jgwest/app
    path: /
    revision: master
 
  # Target K8s repository
  destination:
    # optional: if not specified, defaults to same KCP workspace as CR
    namespace: my-namespace
    # ref to a managed environment:
    # managedEnvironment: some-non-kcp-managed-cluster 
    # This field will tend not to be used for AppStudio.
    # optional: if not specified, defaults to same KCP workspace as CR 

  type: manual # Manual or automated, a placeholder equivalent to Argo CD syncOptions.automated field

status:
  latestCommitId: ( commit SHA for most recently deployment)
  conditions:
  - (...) # status of deployment (health/sync status)
```


The contents of this CR roughly translates into an [Argo CD Application CR](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#applications).



**GitOpsDeploymentSyncRun**:

Create this to trigger a manual sync (if automated sync is not enabled above):

```yaml
apiVersion: v1alpha1
kind: GitOpsDeploymentSyncRun
spec:
  gitopsDeploymentName: jgwest-app # ref to GitOpsDeployment CR
  revisionId: (...)  # GitOps repo GitHub commit SHA

status: 
  health: Healthy # (enum from Argo CD Application health field: Healthy / Progressing / Degraded / Suspended / Missing / Unknown)
  syncStatus: Synced # (enum from Argo CD status: Synced / OutOfSync)
  conditions:
    - lastTransitionTime: "2020-10-04T02:19:14Z"
      message: "Successfully completed synchronize operation."
      reason: Succeeded
      status: true
      type: Succeeded
    # (Other useful conditions)
```

The contents of this CR has no equivalent in Argo CD: Argo CD does not currently support triggering synchronize operations via CR. Instead, with Argo CD, you trigger this via GRPC operation to the Argo CD API server pod (either via Argo CD CLI, or Web UI).

 **GitOpsDeploymentManagedEnvironment**:

At some point in the future, we are likely to want to manage clusters that are external to the GitOps service (e.g. they are independently operated via EKS, AKS,etc, and not virtually addressed by KCP). This would allow those cluster credentials to be defined, for use by GitOpsDeployment CR.

```yaml
apiVersion: v1alpha1
kind: GitOpsDeploymentManagedEnvironment
metadata:
  Name: some-non-kcp-managed-cluster
spec:
  clusterCredentialsSecret: cluster-creds-secret
  allowInsecureSkipTLSVerify: true/false 
---
apiVersion: v1
kind: Secret
metadata:
  name: cluster-creds-secret
```


The contents of this CR roughly translate into an [Argo CD Cluster Secret](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters).

**GitOpsDeploymentRepositoryCredentials**:

The contents of this CR are used to provide credentials for a private Git repository. 

```yaml
apiVersion: v1alpha1
kind: GitOpsDeploymentRepositoryCredentials
metadata:
  Name: private-repo-creds
spec:
  url: https://github.com/jgwest/app
  secret: private-repo-creds-secret

---
apiVersion: v1
kind: Secret
metadata:
  name: private-repo-creds-secret
stringData:
  url: https://github.com/jgwest/app
  # and:
  username: jgwest
  password: (my password)
  # or:
  sshPrivateKey: (...)
```



The contents of this CR roughly translates into an [Argo CD repository secret](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repository-credentials).


# Workflows

## Workflow: when an Application CR is first created (from the GitOps service perspective)

The _Application_ CR definition can be found in [GitHub](https://github.com/konflux-ci/application-service/blob/main/api/v1alpha1/hasapplication_types.go). From the Application Service, the contents of the _‘gitOpsRepository_’ field are used.

The AppStudio GitOps service watches for creation/update/deletion of an Application CR, and creates a corresponding _GitOpsDeployment_ CR and _GitOpsDeploymentRepositoryCredentials_ CR.

Next, the GitOps Service reconciles these CRs, by ensuring the following resources are created on the corresponding Argo CD instance:

- Argo CD Cluster Secret for the target KCP workspace

- Argo CD Repository Secret for the AppStudio GitOps repository

- Argo CD Application CR targeting the above 2 items

**Open question on environment definition**: The HAS folks have suggested that environment (‘staging’, ‘prod’) information should be delegated to the Workspace CR API. This would include information [on which sub-path within the Git repository corresponds to which environment](https://github.com/jgwest/gitops-repository-template#resources). Until the workspace API is defined, and all parties have agreed on its contents, this is still an open question between GitOps, HAS, and Workspace API team.


## How is the GitOps service informed that a deployment should begin (for example, via the UI or via automated trigger)?

A deployment (corresponding to an Argo CD sync operation) will occur when a **GitOpsDeploymentSyncRun** CR is created.

- This CR is the primary (and only) mechanism for informing the GitOps service that a deployment should occur.

- This CR is ephemeral: it will only exist for the lifetime of the underlying synchronize operation.

  - Example: So, if it takes \~10 seconds to update the K8s resources on the cluster, the DeploymentRun will exist for only slightly longer than that.

- It should not be used in order to report the overall health of the Application

  - See _GitOpsDeployment_

There are multiple triggers that will cause the GitOps service to create a GitOpsDeploymentSyncRun CR:

- A GitOpsDeploymentSyncRun CR is created if either one of the following is true:

  - A) The user clicks the manual deploy button within the UI (see manual deployment scenario below)

  - _OR_

  - B) 

    - The GitOps repository contents changes (new commits, webhook invoked)

    - _AND_

    - The Application has automated deployment enabled, and the environment has automated deployment enabled. (Or, said another way, do not perform automated deployment unless both the target environment, and the application, allow for automated deploys).



**Note**:  The _status_ field of the GitOpsDeploymentSyncRun is only accurate (up-to-date) during the actual deployment operation. Which is to say, if the application becomes unhealthy (pod goes down) after the deployment operation has completed, the _health_ field of the DeploymentRun will NOT change.

- Or, said another way, the status is accurate only up to the completion of the deployment operation. On deployment operation completion (it can either pass or fail), the status will be updated one last time.

**Open Questions:**

- Who is responsible for storing historic deployment status? Probably the GitOps service via RDBMS.


## Workflow: Manual deployment

1. User clicks a ‘manual deploy’ button within the HAC UI.

   - Q: Is a user going to pick a target environment for this in the UI? 

2. HAC updates the Application CR to ensure it is up-to-date with the UI.

3. HAC (or HAS?) triggers a build of the build service, and waits for the build to complete.

4. HAS is informed by some mechanism, and updates the GitOps repository

   - Ensures that the contents of the Application CR match the contents of resources within the GitOps repository, including:

     1. Application CR's environments variables/resource limits/etc are up-to-date with those same values in the application's Deployment in K8s

     2. Checks what the latest container image is from the build service, and ensures that the Deployment in the GitOps repository is targeting that _image:tag_ coordinate.

     3. Performs a Git commit for those items (if there are any changes, otherwise no-op)

5. A _GitOpsDeploymentSyncRun_ CR is created that points to the _GitOpsDeployment_ that needs to be deployed.

6. GitOps service reconciles the _GitOpsDeploymentSyncRun_ and triggers a sync operation on the corresponding Argo CD Application CR.

7. GitOps service continuously updates the _status_ field of the _GitOpsDeploymentSyncRun_ CR with the Argo CD Application health/sync status, and the conditions on the CR (whether the deploy is complete, etc).

8. UI should be able to get any information it needs on the status from the CR.


## Workflow: Automated deployment

If the workspace environment, and the application, both have automated deployment enabled (defined above):

1. (The user updates their source code).

2. The build runs, based on the new source code.

3. The build completes, and HAS is informed by some mechanism, and updates the GitOps repository

4. GitOps service detects the update to the GitOps repository via webhook, and verifies that automated deployment is enabled, and creates a _GitOpsDeploymentSyncRun_ CR

   - This step is different from above: for automated deployments, the GitOps service can watch the GitOps repository directly, rather than relying on another AppStudio service to inform us of the change.

5. (proceeds the same as steps, above) 


## Workflow: what happens when a user edits the Application definition via the AppStudio UI

1. AppStudio UI patches the changes into the Application CR

   - (As discussed, there will be OpenShift-Console-style collision logic to handle simultaneous user edits within the UI, but this is a specific concern of the GitOps service)

2. HAS controller will be watching the Application CR, and will be informed of the above change

3. HAS controller will then update the GitOps repository's Deployment resources to be consistent with the Application definition (env vars, replicas, etc)

   - As agreed during F2F discussions, changes to the Application CR are always,  immediately synced to the GitOps repository (regardless of whether the environment/application are set to manual or automated)

     1. (but see step 4 for whether a deployment occurs or not)

   - _Example_: User adds an env var in the UI and clicks Apply. This causes the Application CR to be updated. The HAS controller detects this update, and performs a Git commit to the corresponding Deployment resource in the repository.

4. The GitOps service will detect the GitOps repository commit, and if required (automated deploy is enabled for both app and environment) then a _GitOpsDeploymentSyncRun_ will be created.


## \[WIP] Workflow: how to handle when the user provides a ‘latest’-style image tag like ‘quay.io/coolstuff/myapp:latest’  (eg a mutable, non-SHA256 tag)

A GitOps tool (such as Argo CD) is responsible for ensuring that the K8s resources in a Git repository match what is on the cluster. So if you have a K8s Deployment pointing at _'quay.io/coolstuff/myapp:latest_', it will ensure the Deployment on the cluster references this value (via the container template).

BUT, the GitOps tool doesn't actually control how the K8s cluster processes this K8s resource: the tool can't tell the K8s cluster that it actually needs to pull a new version of the image and redeploy it. 

So the Deployment/Pod controller on the cluster will just keep using this old image until the pod goes down (or there is some rollout action).

**Note**: Ultimately, using a ‘latest’-style tag within a GitOps repository is considered bad practice when defining resources within a GitOps repository. 


### Solution 0: Don’t support updates to non-SHA-tagged container images

This may be unpalatable to some, but we can rely on the behaviour of the underlying cluster when updating non-SHA-tagged container images. The particular Pod that references this container image will only be refreshed when the cluster decides it should be (for example, the Pod dies). 


### Solution 1: When a deployment is performed via GitOpsDeploymentSyncRun, trigger an image refresh if the container tag is not a SHA256 value

The GitOps service can trigger a manual refresh on the Deployment image when needed. (For example, in K8s you can delete the Pod of a Deployment in order to cause its image to be refreshed -- but there is likely a more correct way to do this.)

Any time we perform a deployment (_GitOpsDeploymentSyncRun_ CR is created), if the user provides us with a non-SHA256 value within the GitOps repository, we can automatically refresh the Deployment image (note: this will cause the Pod to be destroyed and recreated).

So, an image refresh might be triggered when, at least one of the following is true:

- User asks us to deploy using a non-SHA256 tagged container image.

- HAS could watch the container registry for changes to ‘latest’ tag, and creates a DeploymentRun when it changes.

On trigger, the steps would be:

- _GitopsDeplomyentSyncRun_ is created, and the GitOps controller detects this (both as described above)

- GitOps triggers an Argo CD sync operation on the Argo CD Application CR as before, to ensure the K8s resources are in sync

- Finally, post sync, the GitOps controller deletes the Pods of any Deployment resources that exist on the application (or whatever the proper way of doing this is), to ensure they are properly refreshed. 

Properties of this solution:

- GitOps repository contains the image and tag exactly as the user provided.

  - Example: The Deployment container image field would be ‘quay.io/coolstuff/myapp\@latest’ within the Deployment resource YAML

  - (It would NOT be myapp@\[some sha256 hash])

- If there is some other mechanism for refreshing the Deployment container images, this mechanism will be respected.


### Solution 2: If the user gives us a container image tag that is not a SHA256 value, we can use the OCI/quay.io API to convert it to a SHA256 value, and use that value within the GitOps repo

If, via the AppStudio UI, the user provided us with a container image like 'quay.io/coolstuff/myapp\@latest', we can detect that 'latest' is not an SHA256 value (is not hexadecimal value of the appropriate length)

We should be able to translate this into a SHA256 value, but querying this via OCI/Docker registry API (but if necessary we could use the registry-specific API -- eg quay.io API).

Properties of this solution:

- GitOps repository does not contain the image and tag exactly as the user provided.

  - Instead, it would contain a refresh to the most recent SHA hash

  - Example: The Deployment container image field would be ‘quay.io/coolstuff/myapp\@latest’ within the Deployment resource YAML

  - (It would NOT be myapp@\[some sha256 hash])

- BUT, if the user deletes the image by that sha256 hash, the Deployment will fail on restart from then on.

- Easier to implement versus solution 1


# Open Questions

**Who is responsible for creating/updating/maintaining the workspaces/environment API?**

- This API might include, for example:

  - what environments exist whether they are automated or manual.

  - what environments should be targeted by the application

**Who is responsible for updating the GitOps repository based on changes to workspace/environments within the Workspaces/Environment API?**

- (Jonathan/Alexey assumed HAS, but no consensus here)

- And how should we handle this without KCP?

**How does the deployment workflow link to the build services?**

- Build service will need to inform one of the services that a build has successfully completed, so that the GitOps repository can be updated, and a DeploymentRun can be created.

**Some criteria that might be used to determine which components should be responsible for which capabilities:**

- Centralizing the logic of the structure of the GitOps repository into as few services as possible (necessary), to keep complexity low.

- Centralizing the logic of how to talk to Git, to as few services as possible (necessary)

- Which service(s) are aware of events that should trigger a deployment, in particular scenarios?

  - Example: build service knows when a build has completed. HAC knows when the UI has been updated. (HAS can only know about this changed by eg watching CRs with its controller)


# Other Discussions

It was brought up that the 'GitOps repository' might be used for other things besides GitOps:

- The GitOps service requires the GitOps repository to contain the desired state of K8s resources on the cluster (as per GitOps principles)

  - This would include both Application deployment resources (Deployment/ConfigMap/etc), but also potentially Tekton build resources (Pipelines, Tasks, etc)

- However, we don't want to overwhelm the user with too many, arbitrary repositories associated with their AppStudio application

- So it may make sense to use the 'GitOps repository' for other AppStudio purposes besides GitOps

  - In this scenario, GitOps would likely be scoped to a directory within the repository, to prevent collisions.

  - But note: Any change to the Git repository (whether affecting application resources, or not) will cause Argo CD to do a small amount of extra work. 


## Future requirements

In the future, we will want to open up the (currently private) GitOps repository to users, to allow them to perform commits (and/or allow users to use their own repository for this, so as to use their own CI tools or to enforce their own policies).

- Thus, as John D noted, we need to handle the case where users modify the K8s resources on the GitOps repository, and those changes should be synced back to the AppStudio Application CR (likely with custom conflict handling based on what the UX of this should be)

- This differs from the current model, which is a one way sync, ApplicationDefinition -> GitOps repo resources.


# Old Discussions

**In order to preserve Google Doc comments, I’ve moved old discussions to this section.**


## Workflow: when an Application CR is first created (from the GitOps service perspective)

- When an application CR is created:

  - The Application CR ideally contains the following information:

    - Reference to a GitOps repository and environments within that repository

    - Whether automated or manual deployment is enabled for the application.

      - Q: (Is this enabled at an application level, or an environment level)

    - Contains reference to a path within the repository for the kustomize base (not sure if this is needed if we are using environment overlays)

    -  Environments of the applications

      - Automated or manual deployment for the environments of the applications

    - Contains reference to a path within the repository (representing the overlay for the environment)

  - (Or at least is what would be helpful to GitOps service)

Example:
                                                             
```yaml
apiVersion: v1
# (this is just my simple example; Elson's team are the folks doing investigation into how this will look in practice)
kind: AppStudioApplication 
spec:
  # link to source code (but gitops service doesn't use this)
  sourceRepo: http://github.com/jgwest/source-city 
  gitopsRepo: # link to GitOps repo
    repoUrl: https://github.com/jgwest/app-city
    basePath: /apps/app1 # not sure if this would be used, since GitOps would use the environmentPath?
    branch: main

  environments:
  - name: staging
    deploymentType: automated
    environmentPath: /environments/staging/apps/app1
  - name: production
    deploymentType: manual
    environmentPath: /environments/production/apps/app1

```                                                             

- The GitOps Backend is a controller that is watching for this CR, and once detected, configures Argo CD to handle it (ensures Argo CD instance exists, and cluster/git credentials are configured on that instance, etc)

  - Likewise changes/deletions of application CR would be similarly handled.


**Open questions**:

- From the Oct 6 App Studio Arch Sync meeting:

  - We may not want to include environments in the AppStudioApplication CR, rather including them in the Workspace/Environment definition at the KCP level.

  - How this is to be scoped is in open discussion.


## How is the GitOps service informed that a deployment should begin (for example, via the UI or via automated trigger)?

Note: This hasn't been discussed yet, but I would expect this would be triggered by HAS (or HAC) creating a **DeploymentRun** CR. The GitOps service would then detect this CR, initiate a deployment, and post updates to the status field of the CR.

This is a new CR, DeploymentRun, which is created whenever a new deployment operation (either manual, or automated) needs to be performed.

- This CR is the primary (and sole) mechanism for informing the GitOps service that a deployment should occur.

- The DeploymentRun is ephemeral; it will only exist for the lifetime of the deployment operation. 

  - Example: So, if it takes \~10 seconds to update the K8s resources on the cluster, the DeploymentRun will exist for slightly longer than that.

- It should not be used in order to report the overall health of the Application health (something else should update this as a status on the Application CR)

  - (Why? Because the status field of the DeploymentRun is only briefly accurate. See below)




_Example:_


```yaml
apiVersion: v1
kind: DeploymentRun


spec: # This section populated by HAS/UI
  applicationName: my-app # reference to Application CR within the namespace
  targetEnvironments:  
    - staging # this should reference environment(s) within the Application CR
  type: Manual # how it was initiated (enum: Manual / Automated)

status: # GitOps service populates this section.
  environments:
    - name: staging
      health: Healthy # (enum from Argo CD Application health field: Healthy / Progressing / Degraded / Suspended / Missing / Unknown)
      syncStatus: Synced # (enum from Argo CD status: Synced / OutOfSync)
  conditions:
    - lastTransitionTime: "2020-10-04T02:19:14Z"
      message: "Successfully completed deployment."
      reason: Succeeded
      status: true
      type: Succeeded
	# (Other useful conditions)
```


**Note**:  The _status_ field of the Deployment run is only accurate (up-to-date) during the actual deployment operation. Which is to say, if the application becomes unhealthy (pod goes down) after the deployment operation has completed, the _health_ field of the DeploymentRun will NOT change.

- Or, said another way, the status is accurate only up to the completion of the deployment operation. On deployment operation completion (it can either pass or fail), the status will be updated one last time.

- Something else should be responsible for keeping track of the health of the Application (possibly still the GitOps service, if we are happy with Argo CD’s ‘health’ field being used for this)


My expectation is a **DeploymentRun** resource should be created whenever a deployment should be performed: this would be the primary way of signalling the GitOps service to begin the process.

**Open Questions:**

- From the Oct 6 App Studio Arch Sync meeting:

  - In order to avoid storing environment/deployment information in the Application definition, it was suggested we could create a ‘parent’ CR of DeploymentRun,  which would last longer than deployment operation (This sounds similar to how PipelineRun referencing Pipeline; PipelineRun contains the status and mutable params, Pipeline contains the steps/immutable configuration)

    - It would contain GitOps repository, environment path information (rather than storing it in the Application)

    - New CR name (brainstorming) DeploymentBinding, DeploymentTracker, DeploymentPipeline, DeploymentConfiguration, Deployment

  - Another suggestion was a CR that kept track of all deployments that were taking place, and had the sametime as the workspace (See Matous’ comments)

- Who is responsible for storing historic deployment status? 