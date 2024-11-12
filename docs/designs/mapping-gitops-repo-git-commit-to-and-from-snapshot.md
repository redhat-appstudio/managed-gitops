# Mapping GitOps Repo Git Commit \<-\> Snapshot

### Written by
- Jonathan West (@jgwest)
- Originally written in May 27, 2022


I've been investigating the implementation details of the ApplicationPromotionRun CR, which is responsible for updating the ApplicationEnvironmentSnapshotBinding.

I'm looking for a way to determine which Git commits correspond to which Snapshots (or vice versa):  
e.g. snapshot name+environment \-\> git commits that contain that snapshot id for a particular environment, or git commit \-\> list of snapshot+env combinations that that git commit contains.

**TL;DR**: My hope is that after HAS generates the GitOps repo, HAS can add the generated commit id to a list in one of the Environment APIs (see examples below). That list would also include the environment/snapshots in that commit. This list would then be used by the ApplicationPromotionRun controller (part of the GitOps Service)

## The problem

The following YAML would tell the GitOps Service to do a manual promotion of 'snapshotA' to 'app1' for the 'staging' environment.

```yaml
kind: ApplicationPromotionRun
spec:
  snapshot: snapshotA
  application: app1
  manualPromotion:
    targetEnvironment: staging
```

Once the above is created, the Promotion controller of the GitOps service will then do the following:

1. Find the ApplicationSnapshotEnvironmentBinding that corresponds to Application 'app1' on Environment 'staging'  
2. Set the 'snapshot' field of that binding to 'snapshotA'  
3. HAS: HAS will see the updated snapshot field of the binding and regenerate the GitOps repository  
4. GitOps Service will deploy the updated changes from the GitOps repository.  
5. Once deployed, GitOps Service will update the \`status\` \`GitOpsDeployment\` field, indicating which commit was deployed by Argo CD:

```yaml
kind: GitOpsDeployment
spec:
  # (...)
status:
  healthy:
    status: Healthy/Unhealthy
  sync:
    status: Synced/OutOfSync
    revision: abc # (the particular Git commit that was deployed)
```

**Now, here's the open problem:**

The ApplicationPromotionRun controller needs to know if a particular Git revision (commit) contains the updated Snapshot and Environment.

Now:

* The ApplicationPromotionRun controller needs to be able to look at the GitOpsDeployment 'revision' field and know if it contains 'snapshotA' for the 'staging' environment.

The promotion controller wants to make sure that the Application still works when deployed with the new snapshot, and so it needs to know if the GitOps repository resources that have been deployed actually contains the new snapshot.

## Possible Solutions

Fortunately, the HAS component has all the information we need:

* What snapshot (name) was used to generate what Git commit (revision)

So the question is, where to put that information so that it can be consumed by the GitOpsService (and specifically the ApplicationPromotionRun controller).

Here are some ideas from brainstorming.

### Option A: Embed the information in the status field of the Snapshot

This option is my preference.

After HAS generates and pushed a Git commit, it would update the \`.status.gitopsRepository\` field of the ApplicationSnapshot, by adding a new entry to the gitRevisions list:

```yaml
kind: ApplicationSnapshot
metadata:
  name: snapshotA
spec:
  application: app1
  # (snapshot details)
status:
  gitOpsRepository:
    - environment: staging
      gitRevisions:
        - abc # in revision 'abc', snapshotA is targeted to the staging environment
        - def
    - environment: dev
      gitRevisions:
        - abc # a single commit can/will often reference multiple environments (if multiple environments were deploying the same snapshot)
        - ghi
        - jkl
```

Note:

* The gitRevisions field would be a historical list of all the last N git commits that contain the snapshot deployed to the particular environment (for some N)  
  * It would NOT be only the last gitRevision.  
* For example, if the snapshot had been used to generate 100 git commits for a particular environment, so far, the gitRevisions field would contain 100 entries.

### Option B: Embed the information into the status field of the Binding

This is an alternative option. The snapshot information is stored in the Binding, rather than in the Snapshot.

Whenever a Git commit is pushed by HAS, HAS would also update the Binding's '.status.snapshots' field.

```yaml
kind: ApplicationSnapshotEnvironmentBinding
spec:
  application: app1
  snapshot: snapshotA
  environment: staging
  # (...)  
status:
  
  # A historical list of all the snapshots that have been deployed to this binding and environment (for some N number of git revisions)
  snapshots:
    - snapshot: snapshotA
      gitRevisions:
        - revisions:
            - abc
            - def
    - snapshot: snapshotB
      gitRevisions:
        - revisions:
            - ghi
            - jkl
```

     	   
One of this disadvantages to this approach is that the '.status.snapshots' field might get very large (with a large number of snapshots/environments/revisions), and K8s resources have a maximum size of 1024KB (\<512KB if using apply).

### Option C: Another idea?

These are the two obvious places to put this information. Any other suggestions? (Application? Component? Environment?) Or other behaviour?

The general problem to solve is: how to tell if a particular commit includes a particular snapshot, for a particular environment.

