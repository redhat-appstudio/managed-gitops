# Rethinking Argo CD Architecture in GitOps Service

### Written by
- Jonathan West (@jgwest)
- Originally written in June 20, 2023

This document:

* Explores the advantages/disadvantages/tradeoffs of various possible Argo CD architectures, in the context of a GitOps Service  
* Excludes any considerations of interaction with AppStudio.

## A) Current Architecture:  what is currently implemented in managed-gitops repo

*How it works:* Two controllers that abstract over multiple Argo CD instances: a 'backend' controller that reads the user API and updates a database with desired state of Argo CD, a 'cluster-agent' controller that lives on a Argo CD cluster acts on that state to configure one or more Argo CD instances.

**Advantages:**

* Already implemented  
* A single 'pane of glass' to control deployment from multiple Argo CD instances   
  * e.g. CRs created in (AppStudio) user's namespace may target any Argo CD instance, even if that Argo CD instance is not on the same cluster, or even accessible from the public internet
* Ability to transparently scale a single user, or users generally, across multiple Argo CD instances  
* Ability to transparently scale users across Argo CD instances on different clusters  
* Support for user-administered cluster credentials and user-administered repository credentials (e.g. GitOpsDeploymentRepositoryCredentials and GitOpsDeploymentManagedEnvironments)  
* Straightforward to implement deploying to a cluster behind the user's firewall (link above)

**Disadvantages:**

* Not a part of upstream Argo CD project  
* Does not use Argo CD Application API (but nothing in the architecture us from moving to this API)  
* Cannot use ApplicationSets (since ApplicationSets don't support multi-tenancy), or any other Argo ecosystem project that expects to be running in the Argo CD namespace.  
* Cannot use Argo projects that expect to generate/modify Argo CD Application CRs  
* Does not support Argo CD web UI or CLI

## B) 'Application in any namespace' Argo CD

Each user is given their own Namespace on a (AppStudio) cluster, with that namespace being an ['Application in any namespace' Namespace](https://argo-cd.readthedocs.io/en/stable/operator-manual/app-any-namespace/). We would also need some additional controller logic for setting up AppProjects, and handling validating/mutating webhooks.

AppStudio (et al) would create Argo CD Application CRs within this namespace.

**Advantages:**

* Uses upstream Argo CD Application API  
* Code for this can be contributed to upstream Argo CD project (except for the additional controller logic)  
* Supports Argo CD web UI (and, I think, CLI?)

**Disadvantages:**

* See this [list of advanced use cases not supported by this model](advanced-use-cases-not-covered-by-application-in-any-namespace-gitops-service-model/advanced-use-cases-not-covered-by-application-in-any-namespace-gitops-service-model.md)  
* Beta feature  
* ~~Currently requires restart of Argo CD each time a new user (Namespace) is added.~~ For AppStudio, I think the ‘\*-tenant’ glob will work for this  
* Currently does not support namespace-scoped Argo CD, so it’s not possible to have multiple Argo CD instances on a single cluster, for example to allow dedicated Argo CD instances, or for scalability reasons (DevSandbox has 3000+ users for their service, split over 2-3 clusters, I believe)  
* No support for user namespace-scoped cluster secrets or namespace-scoped repository credentials, at this time.  
* Cannot use ApplicationSets (since ApplicationSets don't support multi-tenancy), or any other Argo projects that expect to be running in the Argo CD namespace. No “ApplicationSets in any Namespace” functionality.  
* A large customer may overwhelm the ability of a single Argo CD instance to handle all their deployments.   
  * This mechanism prevents more than 1 Argo CD instance from reconciling a Namespace at a time. E.g. there is necessarily 1-1 from namespace to a single Argo CD.  
  * In contrast, architecture A) could have multiple Argo CD instances reconciling a single user’s Namespace.  
* No single pane of glass (all Argo CD Applications a user creates will necessarily be reconciled by a single Argo CD instance: for example, you cannot have an Argo CD on another cluster reconcile that namespace)  
* You cannot scale across multiple Argo CD instances (without using an architecture like C): the user is tightly coupled to the cluster (and Argo CD instance) that their Argo CD instance is on.  
* Not clear how to use this mechanism to deploy to a cluster behind the user's firewall.  
* Not implemented in full: significant gaps that would need to be implemented, as described above. In aggregate, they are: repo creds, cluster creds, app project/webhooks, dedicated instances, namespace scoped ApplicationSets, restart of Argo CD.

## C) Fully dedicated Argo CD instance for users and (potentially) users have admin access over that Argo CD instance

Each user would have full access to their own Argo CD instance (via OpenShift GitOps). AppStudio (et al) would create Argo CD Application CRs within this namespace.

**Advantages:**

* Uses upstream Argo CD Application API  
* A part of upstream Argo CD project  
* Users can create any Argo CD Applications/AppProjects/Secrets they want  
* Users can take advantage of the full Argo CD ecosystem  
  * For example, they could use ApplicationSet controller, since ApplicationSet controller writes directly to argocd namespace.  
* No need to worry about multi-tenant Argo CD.  
* Supports Argo CD Web UI and CLI

**Disadvantages:**

* No single 'pane of glass to control deployment', using the same definition as above  
* A large customer may overwhelm the ability of a single Argo CD instance to handle all their deployments. (And this mechanism prevents more than 1 Argo CD instance from reconciling the user's Application at a time).  
  * Or, said another way, you cannot scale across multiple Argo CD instances: if a user overwhelms their single Argo CD instance (including replicas), we have no path forward for them.  
* Users can easily break their Argo CD instance, if they aren't careful  
  * Perhaps a variant of C is one where we let uses create Applications CRs (et al), but don't allow them to touch the Argo CD itself.  
* Significantly more memory intensive (costly) than other solutions (need to run controllers for application, applicationset, server, repo server, redis controllers per user)  
* The user is tightly coupled to the cluster that their Argo CD instance is on. 

## D) A custom controller that is a layer of abstraction between the user and Argo CD

We write a new custom controller to sync Argo CD Applications from user namespaces (e.g. ‘jgwest-tenant’) into the corresponding Argo CD instance (e.g. ‘gitops-service-argocd’).

* For example, as an AppStudio user I create a 'myapp' Argo CD Application in 'jgwest-tenant'.  
* The custom controller sees that, and ensures there exists a corresponding 'myapp-jgwest-tenant' in ‘gitops-service-argocd’ on that cluster.  
* The Argo CD Application in 'jgwest-tenant' is thus never reconciled directly by Argo CD.

With this architecture, a cluster might contain one or more Argo CD instances. But, those Argo CD instances are only configured to watch their own namespace: we are not using the 'Application in any namespace' feature.

Arguably, option A is just a variant of this architecture, but option A is splitting the controller in 2, and using an RDBMS as the representation of Argo CD's desired state.

**Advantages:**

* Uses upstream Argo CD Application API  
* A single 'pane of glass' to control deployment (e.g. CRs created in user's namespace may target any Argo CD instance, even if it is not on the same cluster)

**Disadvantages:**

* Not a part of upstream Argo CD project  
  * E.g. this would need to be contributed to argoproj-labs, not Argo CD itself. It is thus no different than A in this respect.  
* Not clear how to support Argo CD instances that are running inside the customer firewall (ACM agent has been floated as an option, but this idea seems like it is in its infancy, but likewise ACM is not a part of the Argo project.)  
* Would need to implement namespace-scoped cluster secrets, and namespace-scoped credentials  
* Cannot use ApplicationSets (since ApplicationSets does not support multi-tenancy), or any other Argo projects that expect to be running in the Argo CD namespace.  
* Does not supports Argo CD Web UI and CLI  
* Similar to option A, which already exists: the only major difference between A and D 1\) A uses an RDBMS, and 2\) a lack of a cluster-agent component (which makes customer firewall scenario more difficult)

