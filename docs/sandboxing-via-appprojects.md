# Sandboxing AppStudio user namespaces with AppProject

This document examines the implementation details of how to add support for constraining AppStudio/Core GitOps Service users within an AppProject.

Feature: [GITOPSRVCE-428](https://issues.redhat.com/browse/GITOPSRVCE-428)
Epic: [GITOPSRVCE-56](https://issues.redhat.com/browse/GITOPSRVCE-56)


# Introduction

An **API namespace** is a Kubernetes Namespace where users of AppStudio can create AppStudio API CRs, and deploy their Applications.


* These are the '*-tenant' Namespaces on the cluster.
* For example, ‘jgwest-tenant' is mine. (Based on your Red Hat ID)

Each API namespace has a corresponding **ClusterUser** row in the database.  ClusterUser is a simple mechanism for tracking individual users within the GitOps Service (this mechanism may evolve over time, for example, to support other ownership concepts,  but currently fits our needs in AppStudio).
* There is a 1-1 relationship between a user’s API Namespace and ClusterUser row
* For each user’s API namespace, there is a corresponding ClusterUser row that corresponds to that namespace, and vice versa.

Thus, when thinking about how to constrain users, we can use the ClusterUser row (and relationships to that row) to represent all of the permissions that a user should have.

The ClusterUser table is very simple:
* `User_name string` (reference to the user’s API namespace)
* `Created_on timestamp` (when the ClusterUser row was added added to the DB)
* ( ... plus other fields such as `display_name` ... )

# Our Goal

Since what we’re discussing here is how to constrain users with an AppProject, it makes sense that there should exist one Argo CD AppProject per user. 

An AppProject for a user would look something like this:


```yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: app-project-(cluster user uuid)
  annotation:
    username: (name of namespace of the user)
  namespace: gitops-service-argocd

spec:

  sourceRepos:
  # each entry in this list would correspond to a AppProjectRepository row
  - all of the repo URLs that a user has access to

  destinations:
  # each entry in this list would correspond to a AppProjectManagedEnvironment row
  - namespace: '*'
    server: (references to Argo CD cluster secret, which itself is related to a single ManagedEnv)

  # (there are other AppProject restrictions we can add, but I'm focused on the above 2 for now)
```


The GitOps Service (specifically the _cluster-agent_ component, which is the component which interacts with Argo CD) needs to ensure that such an AppProject is created for each user, and that the **sourceRepos** and **destinations** fields are consistent with that user’s current access.
* As above, there is a 1-1 relationship between a user, and an AppProject. 
* Each AppStudio user should have a single corresponding AppProject.

Next, once each user has a corresponding AppProject within the Argo CD namespace that restricts their access, we should ensure that all Argo CD Applications that are generated for that user have a `project:` field that references that AppProject.


```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: guestbook
  namespace: argocd
spec:
  project: (name of app project resource, above) # <==

  source:
    repoURL: (...)

  destination:
    server: (...)
    namespace: (...)
```



# Argo CD’s default Repository Credential behaviour is not a good fit for multitenancy, and AppProject doesn’t fix that

The way that Argo CD handles repository credentials does not fit well with multitenancy. This is covered [in this document](https://docs.google.com/document/d/1aYUFlOvGB0R5142PTg5Y4BQwp_1kugj-zveziErRwJY/edit#heading=h.5mbvxcu19jxr).

In short, if one Argo CD Application has the credentials to read from a private Git repository, then ANY Argo CD Application can read from that Git repository (and will use those credentials).

* Ostensibly this is because Argo CD was designed with the idea that Argo CD administrators would determine which users can/should access which repositories, which in a private organization (a large company) would be typical.

As of this writing, the current short-term solution to this (and which is unrelated to this feature) is to ensure that only a single user can ever reference a single repository URL. For example, this would be invalid:

* User A creates a GitOpsDeployment targeting private repo [https://github.com/user-a/private-repo](https://github.com/user-a/private-repo)
    * GitOps Service connects to the repo using the GitOpsDeploymentRepositoryCredentials, and configures Argo CD to use it.
* User B creates a GitOpsDeployment targeting the exact same URL, [https://github.com/user-a/private-repo](https://github.com/user-a/private-repo) (which they may or may not actually have valid credentials for)
    * GitOps Service rejects this repository, and does not configure Argo CD to use it, because another user is already targeting it
    * (But note there are race condition/ordering issues that need to be solved if we want to reject only the first instance of a repository)


# Implementation Details


## Changes to backend/backend-shared components

**New database table - AppProjectRepository:**

* `ClusterUser (foreign key)`
    * DB index on this field, to allow us to quickly retrieve all items for a particular user.
* `RepositoryCredential (foreign key, nullable)`
    * If there exist private credentials for this repository URL, they will be referenced by this field.
    * Note: there may not exist private credentials for a repository URL, because, for example, the Git repository is public (and therefore a GitOpsDeploymentRepositoryCredential is not needed).
* `Normalized repository URL (string, non-nullable)`
    * (Argo CD has a function we can use for normalization of the repo URL)

This row tracks which Git repository URLs a particular user can access. 

There should also exist a uniqueness constraint on the _(cluster user, normalized repository url) tuple_, to eliminate the case where multiple AppProjectRepository point to the same Git repository (which itself wouldn’t be the end of the world, but this is an easy way to eliminate this risk).

To generate a list of all of the repositories a user can access, so that we can insert those values into an AppProject, one would do: `SELECT * from AppProjectRepository WHERE user = (user id)`
* As above, ensuring we have an index on user that should ensure this query remains efficient.

**New database table - AppProjectManagedEnvironment:**

* `ClusterUser`
    * DB index on this field, to allow us to quickly retrieve all items for a particular user.
* `ManagedEnvironment ID (foreign key, non-nullable)`

This row tracks which ManagedEnvironments (Argo CD cluster secrets) a user has access to.

As above, SELECTing on the ClusterUser field would allow us to generate the list of all of the Argo CD cluster secrets (corresponding to ManagedEnvironments), so that we can insert these values into the AppProject resource.

**New behaviour - ManagedEnvironment, RepositoryCredential, and Application reconcilers:**

When reconciling a GitOpsDeploymentManagedEnvironment (in _sharedresourceloop_managedenv.go_) or a GitOpsDeploymentRepositoryCredential (in _repocred_reconciler.go_):



* When a ManagedEnvironment/RepoCred is created, ensure the corresponding _AppProjectRepository/AppProjectManagedEnvironment_ is created, pointing to that ManagedEnv/RepoCred.
* Likewise when a ManagedEnvironment/RepoCred is modified, or deleted, ensure the corresponding AppProject* row is removed.

When reconciling an Application (in application_event_runner_deployments.go):



* When an Application is created/modified, ensure there exists an AppProjectRepository for the source field of the repository URL.
    * Why? This handles the case where a user has created a GitOpsDeployment pointing to a public Git repository, and thus doesn’t need to create a GitOpsDeploymentRepositoryCredential for it (which is only required for private repos)

For example, when a user creates/modifies a GitOpsDeployment, it might look like this:


```yaml
apiVersion: managed-gitops.redhat.com/v1alpha1
kind: GitOpsDeployment

metadata:
  name: managed-environment-gitops-depl
  namespace: jane
 
spec:

  source:
    repoURL: https://github.com/redhat-appstudio/managed-gitops
    path: resources/test-data/sample-gitops-repository/environments/overlays/dev

  destination:  
    # (...)
```


The backend component will receive the create/modify event in application_event_runner_deployments.go, and the event will point to the GitOpsDeployment by name/namespace. 

* Normally, the next step once we receive the GitOpsDeployment event is to process it, which involves creating/modifying a row in the Application table: for example, converting the above into a new Application row, containing a spec_field with the above contents.
* But, new for this story is this: before we create the Application row, we should also first create an AppProjectRepository row containing these values:
    * **clusterUser:** clusteruser id based on the namespace that the GitOpsDeployment is in
    * **repositoryCredential:** empty (a “” string), in this case, because the this AppProjectRepository value is being generated based on an Application, and not from a RepositoryCredential
        * This field is nil (“”) if it was generated from a GitOpsDeployment, and should be a foreign key to a RepositoryCredential if it was generated from a RepositoryCredential.
    * **Normalized repository url:** https://github.com/redhat-appstudio/managed-gitops 
        * This value is based on the .spec.repoURL field of the GitOpsDeployment
* Next, after we continue as usual: we create/modify the Application row like we normally do in this file.


## Changes to cluster-agent component

**New behaviour - when an Operation pointing to an Application is reconciled:**

When we process an Operation that points to an Application, before we generate (or update) the corresponding Argo CD Application, we should do the following:

* Generate the expected AppProject resource:
    * Get the ClusterUser from the operation_owner_user_id
    * The name of the AppProject resource should be generated based on the clusteruser uid (something like e.g. ‘appproject-(clusteruser uid)’)
    * `Select * from AppProjectRepository`
    * `Select * from AppProjectManagedEnvironment`
    * Build an AppProject object using the above two values
* Compare with AppProject in the namespace: ensure it matches the generated AppProject 
    * If it matches, done.
    * If it doesn’t match, update it.
    * If it doesn’t exist, create it

And then, after we ensure the AppProject is consistent, when we generate the Argo CD Application, we should:



* (Always) ensure the ‘project’ field of the Application matches the ID of the AppProject
    * This includes ensuring that existing Applications are updated, if they previously has ‘project: default’

If an Application is deleted (e.g. we get an Operation that points to a deleted Application):



* For example, in `deleteArgoCDApplicationOfDeletedApplicationRow`
* We should:
    * `select count(*) from AppProjectRepository where user = (operation user id)`
    * (and)
    * `select count(*) from AppProjectManagedEnvironment where user = (operation user id)`
    * This will tell us the number of these rows that exist for that user, in those 2 tables
* If `(# of appprojectrepository for the user + # of appprojectmanagedenvironment) == 0`, then delete the AppProject resource from the gitops-service-argocd namespace
* Otherwise, don’t need the AppProject resource from the gitops-service-argocd namespace
* Basically: the AppProject should be deleted if the user doesn’t have any GitOpsDeployments/GitOpsDeploymentRepositoryCredentials/GitOpsDeploymentManagedEnvironments defined in their API namespace, otherwise it should not be deleted.