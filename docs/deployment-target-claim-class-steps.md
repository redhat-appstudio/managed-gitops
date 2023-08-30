
# DeploymentTarget/Claim/Class - step by step examples

This document was written by Jonathan West, and is an opinionated intepretation/companion to [ADR-8](https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md).

## Assumptions (formerly, open questions)

#### In the 'DTC Reconciliation' flowchart, and 'BYOC & Manual Environment Creation' flowchart, the DeploymentTargetClaim has a `.spec.targetName` field.
- However, `.spec.targetName` does not appear in any of the examples in the ADR markdown doc.
- I assume this works in a similar way to the `.spec.volumeName` field of the `PersistentVolumeClaim` resource.


#### There doesn't appear to be a way for a user to specify the name of the namespace they want in the `DeploymentTargetClaim`
- For example, if I wanted a namespace named 'staging', how would I include that value in `DeploymentTargetClaim`, so that it can be passed to the SpaceRequest provisioner?
- My assumption is that the ability to customize the namespace is currently not a necessary requirement.

## Introduction

The steps described here are visualized in the ['Manual Environment Creation - Sandbox' diagram](https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md#manual-environment-creation---sandbox).


The term 'our' refers to GitOps Service team members (et al) implementing the controller as part of this epic.

## Terminology

#### Controller: Deployment Target Binder (also referred to as Binding Controller) 
- As described here: https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md#deploymenttargetbinder
- Implemented by GitOps Service
- In some part of the document, also referred to as the 'Binder controller', or just 'Binder'

#### Controller: 'Dev Sandbox' Deployment Target Provisioner (also referred to as Sandbox Provisioner)
- As described here: https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md#deploymenttargetprovisioner
- Implemented as Sandbox provisioner, when targetting a DTClass of  `appstudio.redhat.com/devsandbox`
	- At present, for phase 1, the Sandbox provisioner is the only implemented DeploymentTargetProvisioner. In the future we are likely to have other Deployment Target Provisioners: for example, one that setup up a Hypershift cluster.
- As part of STONE-114, was implemented by RHTAP Integration team
- In some parts of the document, this is also known as the Sandbox Provisioner, or just 'Provisioner'

#### Controller: SpaceRequest
- Implemented by RHTAP Core team, as part of [ASC-243](https://issues.redhat.com/browse/ASC-243).
- Responsible for watching for SpaceRequests, and creating Namespaces/Spaces/Secrets.


#### A DeploymentTarget is bound to a DeploymentTargetClaim if:
- The DeploymentTarget references the DeploymentTargetClaim in the DeploymentTarget's `.spec.claimRef.name` field.
- There is a 1-1 relationship between the two (they do not bind to any other)
- Both are in a bound state.

#### A DeploymentTargetClaim is bound to a DeploymentTarget if:
- There exists a DeploymentTarget that references the DeploymentTargetClaim via the DeploymentTarget's `.spec.claimRef.name` field, OR the DeploymentTargetClaim's `.spec.targetName` field references the DeploymentTarget.
- There is a 1-1 relationship between the two.
- Both are in a bound state.


**Dynamic Provisioning**: Dynamic provisioning mean that a DT was created for a DTC automatically.


## Scenario: Dynamic creation of a new DT and new DTC

### Step 0: Prerequisites: There is already DeploymentTargetClass defined on the cluster, pointing to devsandbox provisioner.

For the purposes of the provisioning Epic ([GITOPSRVCE-385](https://issues.redhat.com/browse/GITOPSRVCE-385)), you can assume there already exists a DeploymentTargetClass (DTClass), pointing to the sandbox provisioner, as below. 

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTargetClass
metadata:
  name: isolation-level-namespace
spec:
  provisioner: appstudio.redhat.com/devsandbox
  parameters: {}
  reclaimPolicy: Delete
```

### Step 1: User (or RHTAP HAC Web UI, on user's behalf) creates a new DeploymentTargetClaim

Note: From hereon, in all cases where I say a User created a resource (as above), you can assume I also mean that the HAC web UI created it on the user's behalf, either directly or indirectly, via the user interacting with the web UI.
- As of this writing (June 2023), Namespace-backed Environments are only being used for testing purposes: temporary, ephemeral environments are created for the purpose of running integration tests.

The user wants a new Environment for staging purposes, so first creates a DTClaim to request the provisioning of a new Environment.

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTargetClaim
metadata:
  name: staging-dtc
spec:
  deploymentTargetClassName: isolation-level-namespace # for the purposes of GITOPSRVCE-385, we will only support DeploymentTargetClasses that point to a 'appstudio.redhat.com/devsandbox' provisioner
```


### Step 2: User creates a AppStudio/RHTAP Environment to match that new DTClaim

The AppStudio/RHTAP Environment is created, pointing to the DTClaim.

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Environment
metadata:
  name: prod
spec:
  displayName: “Staging”
  deploymentStrategy: AppStudioAutomated
  parentEnvironment: dev
  tags: [staging]
  configuration:
    target:
      deploymentTargetClaim:
        claimName: staging-dtc # ref to DTClaim from step 1
```

**Assumption / Open question**: should we wait for the DeploymentTargetClaim to have a status of 'bound' before the Environment is created? 
- IMHO no, we can create both at the same time, and the Environment controller should be smart enough to act on the DTC only once it becomes bound. 
- Potentially the Enviroment would have a status of 'unbound' until then, or some such thing.


### Step 3: Deployment Target Binder reconciles the DTC, and adds a status of phase Pending
- The Deployment Target Binder is to be implemented by GitOps Service, as part of GITOPSRVCE-385.
- Since there doesn't exist a DT yet, the status should be Pending

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTargetClaim
metadata:
  name: staging-dtc
  annotation: # We detect there is not already a DT for this DTC, so we add this annotation
    provisioner.appstudio.redhat.com/dt-provisioner: appstudio.redhat.com/devsandbox  
spec:
  deploymentTargetClassName: isolation-level-namespace
status:
  phase: Pending # This is added by Binder
```

### Step 4: 'Dev Sandbox' Deployment Target Provisioner sees the DeploymentTargetClaim w/ dynamic provisioning annotation, and creates a SpaceRequest

The 'Dev Sandbox' Deployment Target Provisioner was implemented by the RHTAP Integration team as part of [HACBS-1751](https://issues.redhat.com/browse/HACBS-1751), as merged into managed-gitops repo.

In the future, other Deployment Target Provisioners will likely exist, but at the moment only the 'Dev Sandbox' Deployment Target Provisioner exists.

The explanation of the SpaceRequest API is outside the scope of this document, but is well documented elsewhere. The short version is it's a way for a user to request access to deploy to a new Namespace on a RHTAP/AppStudio-managed cluster.
- For an (over-simplified) example: if a user 'jgwest' wanted a new 'staging' environment, they would call the Space Request API saying "give me a new 'staging' environment namespace'
- The Space Request API would create a new namespace named 'jgwest-staging' on the cluster, give the user access to that namespace (using ServiceAccount/Role/RoleBinding)
- The API would then inform the user of the new namespace.

See the SpaceRequest/Space APIs in the diagram [here](https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md#manual-environment-creation---sandbox) for an example of this.

In this step, the 'Dev Sandbox' Deployment Target Provisioner sees the DeploymentTargetClaim, and creates a SpaceRequest.


### Step 5: SpaceRequest controller sees the SpaceRequest, creates a Space, a Secret, and updates the status of the SpaceRequest

This step is likewise out of scope for this doc. SpaceRequest controller was implemented by the RHTAP Core team as part of [ASC-243](https://issues.redhat.com/browse/ASC-243).

In this step, the SpaceRequest controller sees the SpaceRequest, creates a Space, a Secret, and on success, updates SpaceRequest .status field with success (or fail).


### Step 6: 'Dev Sandbox' Deployment Target Provisioner sees the SpaceRequest has a completed .status, and creates a DeploymentTarget based on that

Back in scope for this document, the 'Dev Sandbox' Deployment Target Provisioner sees the SpaceRequest has a `.status.condition[Ready]` of status `true`.

The `.status` of the SpaceRequest contains the name of the Namespace that was created for the user, and a Secret that can be used to access that Namespace. 
- The Secret will be credentials for a ServiceAccount with access to that Namespace.
- This Secret will be used for deployment by GitOps Service/Argo CD (among others)

So now the 'Dev Sandbox' Deployment Target Provisioner will create the DeploymentTarget based on the information in the SpaceRequest/Space/Secret.

For example:
```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTarget
metadata:
  name: prod-dt
  annotations:
    provisioner.appstudio.redhat.com/provisioned-by: appstudio.redhat.com/devsandbox
spec:
  deploymentTargetClassName: isolation-level-namespace # same as DTC
  kubernetesCredentials:
    defaultNamespace: team-a--prod-dtc # From Space/SpaceRequest/Secret
    apiURL: …  # From Space/SpaceRequest/Secret
    clusterCredentialsSecret: team-a--prod-dtc--secret # From Secret
  claimRef:
    name: prod-dtc # Reference to the original DeploymentTargetClaim from step 3.
```

The 'Dev Sandbox' Deployment Target Provisioner does NOT update the status of the DeploymentTarget.

### Step 7: Deployment Target Binder sees the DeploymentTarget was created in the previous step, and the DeploymentTarget references this DTClaim via 'claimRef' and sets the DeploymentTarget status to bound.

The Deployment Target Binder sees the DeploymentTarget exists, and has valid values (Secret, etc), and sets the phase of the DeploymentTarget to bound.

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTarget
metadata:
  name: prod-dt
  annotations:
    provisioner.appstudio.redhat.com/provisioned-by: appstudio.redhat.com/devsandbox

status:
  phase: Bound # Since the DTC already exists, we are now Bound.
```


### Step 8: Deployment Target Binder sees the DeploymentTarget from the previous step was set to bound, and the DeploymentTarget references this DTClaim via 'claimRef', and sets the DTClaim status to bound.


```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: DeploymentTargetClaim
metadata:
  name: prod-dtc
  annotations: 
    dt.appstudio.redhat.com/bind-complete: true # jgw: A couple examples say 'yes', here, but it should be 'true'
    dt.appstudio.redhat.com/bound-by-controller: true # jgw: IMHO, likewise, should be 'true', not 'yes'.
spec:
  deploymentTargetClassName: isolation-level-namespace
status:
  phase: Bound # Updates the status to Bound
```


### Step 9: Environment controller of appstudio-controller should detect the Environment points to a valid DTClaim/DT/Secret, and create GitOpsDeploymentManagedEnvironments for them

This is outside the scope of this document, and is covered by [GITOPSRVCE-388](https://issues.redhat.com/browse/GITOPSRVCE-388).

In short, the appstudio-controller component of the GitOps Service should:
- Detect that that the `.spec.configuration.target.deploymentClaim` of the Environment points to a bound DeploymentTargetClaim (which itself is pointed to by a valid DeploymentTarget).
- Use the information contained in the Secret of the DeploymentTarget to create a GitOpsDeploymentManagedEnvironment based on the Environment
- This logic is similar to how we currently use the `.spec.unstableConfigurationFields.kubernetesCredentials.clusterCredentialsSecret`


## Scenario: Deprovisioning DT/DTC

*Prerequisite*: DeploymentTargetClaim/DeploymentTarget/SpaceRequest have finalizers on them. 
- The controllers should add these finalizers themselves: for example, the SpaceRequest controller adds the finalizer to the SpaceRequest, if not already present

Here is how I expect the deprovision/deletion logic for the DT/DTClaim/SpaceRequest API resources will work.

1) Event: User attempts to deletes `DeploymentTargetClaim`, which sets a deletion timestamp on that DeploymentTargetClaim (DTClaim).
2) DTClaim controller reconciler is called, the controller sees the deletion timestamp on the DTClaim:
    - DTClaim controller finds the `DeploymentTargetClass` that is referenced by the DTClaim's `.spec.deploymentTargetClassName` field:
        - If the DTClass has a `.spec.reclaimPolicy` of `Delete`:
            - Then the DTClaim controller should delete the bound DeploymentTarget (e.g. sets deletion timestamp on the DeploymentTarget resource)
            - Continue to the next step below.
        - However, if the DTClass instead has a `.spec.reclaimPolicy` of `Retain`:
            - Then the DTClaim controller should not delete the bounded DeploymentTarget.
            - Instead, the finalizer should be removed from the DTClaim, causing it to be deleted by K8s.
            - The DeploymentTarget controller should see that the DTClaim was deleted, and the DeploymentTarget's `.status.phase` field should be set to `Released`
            - Stop here.
3) DeploymentTarget controller reconciler is called, the controller sees that the `DeploymentTarget` has a deletion timestamp on it, and deletes the `SpaceRequest` (sets deletionTimestamp on SpaceRequest)
4) SpaceRequest controller acts as below.
5) DeploymentTarget controller waits for the SpaceRequest to have one of two states:
    - SpaceRequest no longer exists in namespace (has been deleted)
        - Success: ~~set DeploymentTarget's `.status.phase` to `Released`~~. *EDIT*: I don't believe `Released` should be used for this case; I don't think any phase update should be performed here.
        - Continue below.
    - Or, if the spaceRequest still exists after 2 minutes, and has a ``.status.condition[ConditionReady]`` of `UnableToTerminate` (from `SpaceTerminatingFailedReason`)
        - Set DeploymentTarget's `.status.phase` to `Failed`, indicating the cluster resources were not cleaned up.
        - Stop here: we don't remove the finalizer from the DeploymentTarget.
        - Note, my intepretation of this logic is: just because we set the .status.phase to `Failed`, here, it doesn't mean that we won't keep waiting for the SpaceRequest to be deleted.
        - For example: if, later on, the SpaceRequest is finally deleted (no longer exists), then we will proceed with the 'SpaceRequest no longer exists in namespace' logic above.
6) DeploymentTarget controller sees the SpaceRequest has been deleted (no longer exists), and removes the finalizer from the DeploymentTarget (causing it to be deleted by k8s)
7) DTClaim controller sees the DeploymentTarget has been deleted, and removes the finalizer from the DTClaim.



#### Other required logic, related to deletion:

A) When a DeploymentTarget sees that a bound DeploymentTargetClaim has been deleted:
- If DeploymentTarget does NOT have a deletion timestamp set:
    - The DeploymentTarget's `.status.phase` should be set to `Released`

B) If a DeploymentTarget is deleted, while it is bound to a DeploymentTargetClaim, then the DTClaim should set its `status.phase` to `Lost`:
- The DTC should not be deleted, after this happens.

C) 
When:
- The DeploymentTargetClass has a policy of Retain;
- A DeploymentTarget and DeploymentTargetClaim exist and are bound together via this Class
- If the DeploymentTargetClaim is deleted:
    - The DeploymentTarget should not be deleted.
    - The DeploymentTarget's claimRef field should be set to empty (that is, it should no longer reference the DT)


#### SpaceRequests controller deletion logic, from `host-operator` repo:

This is a brief summary of the `SpaceRequest` deletion logic from host-operator.

- *Prerequisite*: A finalizer is added to a SpaceRequest when it is first reconciled (toolchainv1alpha1.FinalizerName, `finalizer.toolchain.dev.openshift.com`)

1) When the reconciler detects that a SpaceRequest is being deleted (has a deletion timestamp set):
    - The controller attempts to delete the corresponding `Space`:
        - While the `Space` is being deleted, a condition is set on the `SpaceRequest` of `.status.condition[ConditionReady]` = `SpaceTerminatingReason ("Terminating")`
        - However, if the `Space` could not be deleted within 2 minutes: a condition is set on the `SpaceRequest` of `.status.condition[ConditionReady]` = `UnableToTerminate` (from `SpaceTerminatingFailedReason`)
2) If the Space was deleted successfully, remove the finalizer from the Space (which will cause the resource to be deleted by K8s)


## Scenario: Binding a DTC to an existing DT

Also described in the document is the case where an unbound DT already exists, and a new DTC is created which can be bound to that existing DTC. In this case, dynamic provisioning is not needed.

This is primarily covered in the [DeploymentTargetBinder](https://github.com/redhat-appstudio/book/blob/main/ADR/0008-environment-provisioning.md#deploymenttargetbinder) section and diagram. (A higher res version of this diagram can be bound in Miro link at the bottom of the page.)




#### Open Questions
- *"If the user doesn't care which DT will be used and the DTC.spec.Name is empty, how to find the best match DT for the DTC?"*
    - One of my concerns here is: what if there is a DeploymentTarget targeting a namespace 'jgwest-staging', and a DeploymentTargetClaim requests a 'production' namespace. We woudn't want the production DTC to bind to a staging DT. (But, I'm not sure if this is valid, due to other open questions, above.)
    - The flowchart just says *"Find the best match for the DTC"*




## What's not covered in this document:

The error cases are not covered here. 
- The details for this are primarily in the flowchart.

'Phase 2' requirements are not covered here. For example, DeploymentTarget fields of `.spec.resources` or `.spec.arch`.
- Likewise with 'Phase 3'.

'Manual Environment Creation - BYOC' and 'Manual Environment Creation - Cluster' are not covered here.

    
## Other Notes

DeploymentTargets/DeploymentTargetClaims are NOT immutable. We should expect some of the fields to change: we will need to know how to reconcile those changes correctly. 
- However, for the purposes of the current epic/story, I don't believe we will need to support any of the fields changing.

