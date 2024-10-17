# Comparison: Argo CD Application API vs GitOpsDeployment API

### Written by
- Jonathan West (@jgwest)
- Originally written May 27, 2023


This document is a short comparison between the Argo CD Application CR, and the corresponding GitOpsDeployment CR. See [this document for an explanation/example of GitOpsDeployment](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/api.md#gitopsdeployment). 

**NOREQ** \= Any items marked as NOREQ means we haven’t had a requirement (‘no requirement’) to implement this API yet, but there is no technical reason we couldn’t. (It just hasn’t been at the top of our TODO list yet based on requests from leadership.)

# Argo CD Application API only

The following fields do not currently exist in the GitOpsDeployment API.

## .operation field

**‘.operation’ field**

* A top-level field (a peer of **.spec** and **.metadata**)  
  * And not to be confused with **.status.operationState**  
* Contains declarative information on current operation  
* *Why not included in GitOpsDeployment:*  
  * We don’t expose the concept of operations in RHTAP. Why? RHTAP UI doesn’t allow actions on operations, nor do we support the Argo CD CLI (which could likewise be used to perform actions on operations).

## .spec field

**.spec.source field**

* **.targetRevision**  
  * The name of the corresponding field in GitOpsDeployment is *branch* (I don’t recall any particular reason)  
* Various Source-type fields: **.helm/.kustomize/.directory/.chart**  
  * *Why not included in GitOpsDeployment*: NOREQ  
* **.plugin**  
  * *Why not included in GitOpsDeployment*: no support for Argo CD plugins  
* **.ref**  
  * *Why not included in GitOpDeployment*: no support for sources (at this time)

**.spec.destination field**

* name/server  
  * *Why not included in GitOpDeployment*: we instead use the ‘environment’ field, which is a reference to a GitOpsDeploymentManagedEnvironment.   
    * Since we don’t support Argo CD cluster secrets, the GitOps Service equivalent is ManagedEnvironment, and thus we have a reference to it. 

**.spec.project field**

* *Why not included in GitOpsDeployment:*  
  * RHTAP uses a different multitenancy model than AppProjects (although behind the scenes we do use AppProjects as a defense-in-depth to ensure individual users are segregated)  
  * RHTAP doesn’t allow users to define AppProjects.

**.spec.syncpolicy**

* Most fields not supported:  
  * **.automated**  
    * allowEmpty  
    * prune  
    * selfHeal  
  * **.retry**  
    * backoff (various)  
    * limit  
  * **.syncOptions** (various options)  
* *Why not included in GitOpsDeployment:*  
  * NOREQ  
  * Instead GitOps Service has the ability to switch between automated/manual sync, and sensible defaults for the automated case.

**.spec.ignoreDifferences field**

* *Why not included in GitOpsDeployment*: NOREQ

**.spec.sources**:

* *Why not included in GitOpsDeployment*: NOREQ

**.spec.revisionHistoryLimit**

* *Why not included in GitOpsDeployment*: NOREQ

**.spec.info**

* *Why not included in GitOpsDeployment*: NOREQ

## .status field

Conditions

**.status.operationState**

* *Why not included in GitOpsDeployment:*  
  * We don’t expose the concept of operations in RHTAP. Why? RHTAP UI doesn’t allow actions on operations, nor do we support the Argo CD CLI (which could likewise be used to perform actions on operations).  
* *BUT*: Argo CD does include information within this field that isn’t available elsewhere, so we are likely to still need to expose this field.

**.status.sync**

* **.revisions**  
  * *Why not included in GitOpsDeployment*: we don’t support multiple sources due to NOREQ.  
* **.comparedTo**:  
  * *Why not included in GitOpsDeployment*: this data is instead exposed via **.status.reconciledState**

**.status.history**

* *Why not included in GitOpsDeployment*: NOREQ

**.status.reconciledAt**:

* *Why not included in GitOpsDeployment*: NOREQ

**.status.sourceType/sourcetypes**:

* *Why not included in GitOpsDeployment*: NOREQ

**.status.summary:**

* *Why not included in GitOpsDeployment*: NOREQ

# GitOpsDeployment API only

**.spec.source.branch**

* The name of the corresponding field in GitOpsDeployment is *targetRevision* (I don’t recall any particular reason for the change)

**.spec.destination**

* For convenience, GitOpsDeployment allows .spec.destination to be empty, in which case the target is assumed to be the same namespace as the GitOpsDeployment CR

**.spec.destination.environment**

* See above re: differences between .spec.destination fields in Argo CD vs GitOpsDeployment

**.status.reconciledState**

* Argo CD instead exposes this via **.status.sync.comparedTo**

# Differences in APIs that are defined in both

**.status.conditions**

* The condition types that are supported are different.

