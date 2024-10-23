# GitOps Service in AppStudio Overview (2021)

### Written by
- Jonathan West (@jgwest)
- Originally written in Q4 2021/Q1 2022.


# Introduction

The goal is to deliver a GitOps experience, integrated into AppStudio, consistent with the AppStudio journeys and ongoing discussions of the technical architecture (KCP et al).

**Users will be able to use a GitOps process to manage deployments to their environments (backed by the GitOps service)**

* The deployment itself will be handled by an Argo CD instance, that we manage on behalf of the user.

**Other AppStudio services (eg Hybrid Application Console, HAC and/or Hybrid Application Service, HAS), will create a GitOps repository for the user at the same time that their AppStudio application is created. The GItOps service will use this repository to perform deployments.**

* See the journey workflow for how this is created, and other documents for the specific implementation.

**The GitOps service will ensure that Argo CD instances are configured to deploy from the user’s GitOps repository, to the user’s target environment(s) (based on environment data from other services/KCP).**

**In order to scale to a large number of users, the GitOps service must manage a fleet of Argo CD instances (controller pool), and ensure each of those instances is correctly configured for the particular user’s credentials.**

* This item is where most of the service complexity is introduced.

# Terminology

**Managed application environment**: The user’s cluster (for example, a logical cluster via KCP workspace) containing resources (deployments, services, etc) that are being managed by Argo CD. Argo CD control plane stack will not run here (eg the Argo CD application controller et al), rather Argo CD will live on Red Hat-managed infrastructure, and Argo CD’s push model will be used to manage customer/user clusters. 

**GitOps Control Plane clusters**: The OCP cluster/environment that hosts the product frontend/backend components, and any infrastructure (databases such as PostgreSQL, monitoring such as Prometheus/Grafana). Does not host any Argo CD workloads. Not customer accessible (no Routes are publicly exposed), fully behind the Red Hat ‘firewall’.

* As of this writing (Sept 2021), this is being handled by the AppStudio ‘staging’ cluster, which will contain all team’s AppStudio deployments.

**GitOps Engine clusters**: Clusters/environments that host the Argo CD instances that manage user workloads (with those user workloads running on their own managed environment). We say ‘GitOps Engine’ rather than ‘Argo CD’, so that we have the option of using a different GitOps-enabling technology in the future.

* These cluster(s) would be owned/operated by Red Hat, and not customer accessible (e.g. no public Routes)

**GitOps Product Backend**: Responsible for handling requests from the HAC frontend (by watching KCP namespace, or if not available then via REST API), and communicating user intent (via those requests) to the appropriate Argo CD instance environment.

**GitOps Cluster Agent**: A Kubernetes-controller responsible for ensuring that the Argo CD instances on the GitOps Engine clusters are conformant to user intent (as communicated via the product UI), and likewise ensuring that the state of resources in the shared RDBMS database is accurately reflected. 

# Connection with Other Services

This section is speaking broadly (details TBD), and accurate as of this writing (Sept 29, 2021):

### Hybrid Application Console (HAC)

* HAC (or another mediating component, such as HAS) will be responsible for providing details to the GitOps service (either directly, or indirectly via KCP) including:  
  * The user's application definition  
  * Set of user’s environments  
  * The user's GitOps repository URL  
  * Deployment environment information (eg target KCP Workspace, OR Kubernetes API Proxy, depending on whether KCP is ready)

### Build Service 

* When a new container image is available, the build service will need to notify the appropriate AppStudio component, and that component will need to update the GitOps repository to point to the new image.  
* If the GitOps service is the component that is responsible for updating the GitOps repository, we will need to handle that.

### Hybrid Application Service (HAS)

* Will be responsible for generating the GitOps repository used by the GitOps service.  
* Will be responsible for modifying the GitOps repository, in order to update:  
  * Update K8s resources, such as Deployments (increasing memory/CPU limits, updating environment variables, etc)  
  * Each environment targeted

### Service Provider Integration (SPI)

* Git credentials (and cluster credentials?) will be provided by this service, and the GitOps service will need to wrangle these in a form that can be used by Argo CD.

### KCP (presuming it is available  in time for AppStudio MVP)

* Argo CD to target KCP workspaces (KCP workspaces will be added as remote clusters in Argo CD)  
* GitOps backend component to watch the KCP workspace for configuration CRs (for example, AppStudio Application CRs, if/when they exist)

# API {#api}

## KCP scenario (or K8s-cluster-proxy scenario):

(*If KCP is fully available at the time of AppStudio MVP*)


## Non-KCP/non-cluster-proxy scenario: REST backend Endpoints: 

(*If KCP is not available at the time of AppStudio MVP, nor is our replacement for it, we fall back to REST endpoints.*)

In the non-KCP, non-cluster-proxy scenario, it will be the responsibility of the other services within AppStudio to inform the GitOps service of Applications and Managed Environments via a REST API.

Endpoints are essentially just a light wrapper over the RDMS. The details listed here are non-exhaustive, but should provide a good guideline for the shape of the API.

# Requirements

The goal here is to define a set of requirements that may be realistically delivered, while avoiding areas of complexity that will disproportionately drive up the complexity of iteration delivery. This includes:

- No private clusters (must be publicly internet accessible)  
- We will only support the AppStudio/RH-created GitOps repository, which is created on behalf of the user during the application creation process

**Requirement: User/customer Argo CD instances to be hosted on a single cluster, with each Argo CD instance getting its own namespace.**

* For example, multiple namespaces with each namespace containing a single Argo CD instance  
* *Forward looking*: Expand to a “fleet” of clusters.

**Requirement: Argo CD is the tool responsible for managing user clusters/environments**

* We're not rolling our own GitOps tool (for now)  
* We're not exploring other GitOps tools at this time.  
* But: the Argo CD Web UI/CLI/API will not be exposed to the user.

**Requirement: GitOpsDeployment CR as a single source of truth for the Argo CD Application Resource**

* The Argo CD Application CRs will be based on Application definitions coming from other AppStudio services. Thus we only need to handle one-way synchronization between AppStudio Applications (and GitOpsDeployment CR), and Argo CD Applications.  
  * Or, said another way, the Argo CD Application CR will always be based on the AppStudio App definition (and NOT vice versa)  
* See [Architecture page](./gitops-service-internal-architecture-appstudio) re: resource.  
* *Corollary*: PostgreSQL database will be used as an eventually consistent mirror for what Applications exist (in Argo CD), and what their sync status and health are.

**Requirement: Product backend (via the RDBMS) is the single source of truth for Managed Application Environment, GitOps Engine instance, Operation resources**

* See [Architecture page](./gitops-service-internal-architecture-appstudio) re: resources, operations  
* Managed Application Environment resource is credential’s for a user’s cluster, that they wish to manage with Argo CD  
* GitOps Engine instance (Argo CD Instance) resource is a cluster/namespace where Argo CD is installed  
* Operation is a synchronization and task tracking mechanism, see Architecture Page.  
* *Corollary*: The product frontend/backend will update the RDBMS representation of these resources, then inform the cluster agent of the change via an Operation.

**Requirements: Put security guidelines in place, and enforce them via the PR review process**

* Security (sanitizing inputs, restricted networks/containers, static linting, unit tests, etc) is not afterthought, it should be baked in from the beginning.

**Non-requirements (for this iteration):**

* No support for private or disconnected clusters: all clusters must be accessible by the GitOps product backend (for example, they are on the public internet or are RH internal)  
* No support for Argo CD CLI/UI  
* No Helm repository support (users may still create Argo CD Applications that reference charts, but we won’t track these within the product UI)  
* No API-level ApplicationSet support

 

## Infrastructure/Test/Docs Requirements

Implement, and gather feedback from, functional requirements.  This will help us decide if we are heading in the right direction, and allow us to course correct.

Focus on non-functional/process requirements.

**Requirement: Build E2E test infrastructure, and add E2E tests**

* Add tests that simulate managed workloads

**Requirements: Onboard onto ROMS process, and/or the delivery process agreed to for AppStudio.**

**Requirements: Process/tools for partial rollout, staging, for all components (especially Argo CD)**

**Requirements: Process/tools to deliver security updates for managed Argo CD clusters**

**Requirements: Prometheus/Grafana monitoring of all components**

* I expect this will be driven at the AppStudio level, rather than by us.

**Requirements: Backups/Disaster Recovery process/tools**

* I expect this will be driven at the AppStudio level, rather than by us.

**Requirements: Process/tools to handle database version migration, with eg flyway/liquibase**

**Requirements: Initial user-focused product documentation**

## Future Requirements

This list is a grab bag for any subsequent iterations:

**ArgoCD instance rebalancing: What happens if one user overwhelms the ability of a single Argo CD instance? Need to create a new Argo CD instance and rebalance (shard) the user’s Applications/Cluster across them.**

**Kafka message queue for messaging passing between GitOps backend service and GitOps cluster agent, rather than direct k8s api connection (should be more secure/efficient versus other mechanisms)**

**Redis to store Application status/health (should be more efficient versus using RDBMS for this)**

# Technologies

The technologies I’ve chosen here are modern, mature, and likely uncontroversial due to their existing prevalence within Red Hat/OpenShift.

**Product backend:**

* Simple REST-based GO backend to serve HTTP request from the frontend, and handle other required cluster/environment management tasks  
* Persists to and queries PostgreSQL database (via eg bun, go-pg, many other options)

**Cluster Backend \- Kubernetes controller (Go operator):**

* A Kubernetes controller (likely implemented with the operator framework)  
* Persists and queries PostgreSQL database (via eg bun, go-pg, many other options)

**Database Infrastructure:**

* PostgreSQL, ideally a managed service (eg Amazon RDS), which is consistent with what other Red Hat teams have done when they need a managed database (as discussed during ROMS Process presentation during F2F)  
* *Future considerations*: To improve scaling \- Kafka for passing messages between components, Redis for caching of resource state for UI (instead of using postgres for this purpose), and/or Application health/status

