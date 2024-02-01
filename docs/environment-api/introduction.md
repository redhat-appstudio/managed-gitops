# Konflux Environment API Quick Start

Written by Jonathan West (@jgwest) on February 1st, 2024.

## Konflux Environment API

The Red Hat product/project that this is attached to has had many names: AppStudio, RHTAP (Red Hat Trusted Application Pipeline), Konflux, and more. 
- In the linked resources, you will see many of these names (mostly AppStudio). They all refer to the same code, but a succession of project/product names.
- For the purposes of this document, I will use 'Konflux', the latest name, as of this writing, February 2024.
- However, the code/API referenced in this document has been deprecated, will no longer be used, and will be removed from the Konflux project.

For more information on the Environment API, see this [long, detailed document I wrote](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/environment-api/environment-api-proposal-v3.md) which details the final cross-organization agreement on how this API would work within Konflux.

## Big Picture

- You define an `Application`. (e.g. `bank-loan-app`)
	- **NOTE**: ZERO relation to Argo CD's Application custom resource (CR)/concept, put this out of your mind here in this context. They just happen share a name 😀
	- In this document, I'll use "Argo CD Application" when I specifically mean that.
- You define `Components`, which are the child components of that Application (e.g. `frontend` in Node, `backend` in Java, `database` in PSQL)
- You define `Environment`s, which are the K8s clusters that you wish to deploy that Application to (only K8s clusters are supported)
    - e.g. `test`, `staging`, `prod-eu`, `prod-us`
	- A DAG, e.g. dev -> test -> staging -> prod. Can be single or multiple children (for example, see last deployment in this example.)
    - ![Relationships between resources](Stages.png)

- You define a `Snapshot`, which is a particular version of your Application (and specifically, of its constituent Components). (e.g. `bank-loan-app-v2-(commit id)`)
- The actual deployment of a Snapshot (specific version of an Application) to an Environment (specific k8s cluster) is performed via a `SnapshotEnvironmentBinding`
	- `SnapshotEnvironmentBinding` defines *(Application, Snapshot, Environment)* tuple
	- tuple: Application A, of version S, should be deployed to Environment E
- Finally, `SnapshotEnvironmentBinding` is translated (reconciled, via K8s controller) into 1 or more Argo CD Application
- Thus, all of these abstractions **ultimately boil down into Argo CD Application CRs** (custom resources) deploying to K8s clusters.



#### Relationships between the concepts:
![Relationships between resources](Relationships.png)
- `Binding` seen in the image is just a `SnapshotEnvironmentBinding` (using shorter name in the diagram)



## Central Konflux Concepts

All concepts mentioned have a corresponding CR (Kubernetes custom resource) of the same name. I use the two interchangeably, here.

#### Application
- NOTE: This does not refer to an Argo CD Application, neither the CR nor the overall concept. It is entirely different.
    - When the API was defined I pushed to have them use a different, non-conflicting name, but was outvoted. 😀
	- When thinking of an Application in the context of this document, it is best to put the Argo CD Application concept/CR entirely out of your mind. They are not compatible or related in any way.
- An `Application` is composed of 1 or more `Component`s
- Applications are the atomic unit of deployment:
	- All Components of an Application are deployed together.
	- Components cannot be deployed outside of an Application.
- Example: If you're a bank that gives loans to customers, your Application might be 'loan-app', and your components might be 'front-end', 'backend', 'database'.
- Application and Component are both CRs. 
- [Application CR code](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/application_types.go#L27)

#### Component
- A `Component` is a single container running as part of an `Application`.
- A Component has a single, mandatory Application as a parent.
    - Components are not shared between different Applications.
- Each Component is a single container/container image.
- Example: a bank might have a 'loan-app' Application, whose job is to provide bank loans. The Components of that Application might be:
	- Application 'loan-app'
	    - Component 'frontend': React frontend, served into the browser via a lightweight HTTP server
    	- Component 'backend': Java backend application
	    - Component 'database': Postgresql image
- Application and Component are both CRs.
- [Component CR code](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/component_types.go#L75)


## Central Environment API Concepts, building on on top of Konflux concepts

All concepts mentioned have a corresponding CR of the same name. I use the two interchangeably, here.

#### Environment
- One or more `Application`s can be deployed to an `Environment`.
- An Environment is just a CR describing credentials for deploying to a target K8s cluster.
    - Example Environments: 'staging K8s cluster NA', 'production K8s cluster NA', 'staging K8s cluster EU', 'dev K8s cluster AU', etc.
- Environments form a directed acyclic graph (DAG), which describes the order in which a new version of an Application is promoted between environments.
	- For example: dev NA -> test NA -> staging NA -> prod NA
    - See diagram above.
- Example workflow:
	- Snapshot 'v2' of an Application would first be tested on Environment 'dev NA' (North America)
	- Once it passes some integration tests, Snapshot 'v2' would be promoted to run on Environment 'test NA'
    - Next, once successful, v2 would be promoted to 'staging NA' Environment. 
	- And so on, dev -> test -> staging, along the DAG.
- [API description](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/api.md#environment)
- [Environment CR code](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/environment_types.go#L24)

#### Snapshot
- `Application`s are deployed to `Environments` via `Snapshot`s.
- `Snapshot` represent a single, atomic version of an Application.
    - A Snapshot is only useful in the context of the Application it references: a Snapshot is not shared between multiple Applications.
- A Snapshot is a set of container images, one for each component of an Application.
	- If an Application has 3 components, the Snapshot will have 3 container images.
- Example Snapshot:
    - Application: loan-app
	- Component 'frontend': `quay.io/my-bank/loan-app-frontend:(version, git commit id, or date built, for example)`
	- Component 'backend': `quay.io/my-bank/loan-app-backend:(version, git commit id or date built, for example)`
	- Component 'database': `postgres:16.1`
- Snapshots can be annotated with additional information, for example, whether or not the Snapshot passed integration tests (thus making the Snapshot ready for promotion to next child in the DAG).
	- This ensures a particular version of an Application has necessarily been tested with a particular set of constituent container images
- Snapshots are not shared between different Applications. A Snapshot will have a single Application as a mandatory parent.
- [API description](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/api.md#snapshot)
- [Snapshot CR code](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/snapshot_types.go#L25)

#### SnapshotEnvironmentBinding
- This is the CR/concept that performs the actual deployment of everything described above. This is where the 'rubber meets the road'.
- Behind the scenes, a single `SnapshotEnvironmentBinding` is reconciled into X corresponding Argo CD Applications, where X is the number of components of the Snapshot (Application).
- Describes what Application/Snapshot (version) should be deployed to which Environment.
- A SnapshotEnvironmentBinding (SEB) binds the following into a concrete deployment:
	- Application
	- Environment
	- Snapshot
    - tuple: Application A, of version S, should be deployed to Environment E    

- Example SnapshotEnvironmentBinding:
    - We want to deploy (snapshot) 'v2' of our Application to 'staging' Environment, and 'v1' (snapshot) to 'production' Environment.
    - That would look like this:
        - SEB 'loan-app-staging':
            - application: loan-app
            - environment: staging
            - snapshot: v2
        - SEB 'loan-app-production':
            - application: loan-app
            - environment: production
            - snapshot: v1
    - In this example, the `SnapshotEnvironmentBinding` controller would reconcile the above into 6 Argo CD Applications:
        - 'loan-app-staging' would become:
            - Argo CD Application 'loan-app-staging-frontend': deploy v2 of 'frontend' component to staging
            - Argo CD Application 'loan-app-staging-backend': deploy v2 of 'backend' component to staging
            - Argo CD Application 'loan-app-staging-database': deploy v2 of 'database' component to staging
        - 'loan-app-prod' would become:
            - Argo CD Application 'loan-app-prod-frontend': deploy v1 of 'frontend' component to prod
            - Argo CD Application 'loan-app-prod-backend': deploy v1 of 'backend' component to prod
            - Argo CD Application 'loan-app-prod-database': deploy v1 of 'database' component to prod
- [API description](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/api.md#snapshotenvironmentbinding)
- [SnapshotEnvironmentBinding CR code](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/snapshotenvironmentbinding_types.go#L33)


#### PromotionRun
- A single use CR which will promote a particular Application/Snapshot to a particular Environment
- For example: I could use a `PromotionRun` to promote Snapshot 'loan-app-v2' from 'staging NA' to 'prod NA'
- [API description](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/promotionrun_types.go#L24)
- [PromotionRun CR code](https://github.com/redhat-appstudio/application-api/blob/18f545e48a03cbc6df71fb0468dac9aa66209c4c/api/v1alpha1/promotionrun_types.go#L24)

## Similarity to others: Kargo

From my cursory look at Kargo,the Konflux Environment API is somewhat similar to Kargo (probably because we're working in the same problem space), but with Kargo having more support for advanced use cases.

#### For example, API similarities:
- Konflux `Environment` CR => Kargo `Stage` CR
    - Both are a DAG of K8s clusters, with promotion between, and customizable logic to trigger.
- Konflux `Snapshot` CR => Kargo `Freight` CR
    - `Snapshot` is just a set of container images, while `Freight` is container images but can ALSO be other resources (e.g. helm chart versions)
- Konflux `PromotionRun` CR => `Promotion` CR 
    - Both are a CR which manually triggers a promotion between environments.
- Custom Konflux controllers => `Warehouse` CR
    - Konflux has custom controllers that enable specific, supported workflows, implemented by other Konflux teams. These custom controllers produce `Snapshot`. In contrast, Konflux has the `Warehouse` concept, which generalizes the concept into a CR.
- _(N/A)_ => `PromotionPolicy` CR / `Verification` CR 
    - No specific equivalent in Konflux, but only because we never had specfic requirement requests, here. The Kargo concepts appear sound and would fit right in.

## External Resources

[Environment API CR descriptions and concepts](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/api.md#gitops-service-app-studio-environment-apis)

[Konflux Application/Component/Environment Go APIs live here](https://github.com/redhat-appstudio/application-api/tree/main/api/v1alpha1)

[K8s controllers that implement the Environment API live here](https://github.com/redhat-appstudio/managed-gitops/tree/main/appstudio-controller/controllers/appstudio.redhat.com)

See this [long, detailed document](https://github.com/redhat-appstudio/managed-gitops/blob/main/docs/environment-api/environment-api-proposal-v3.md) which details the final agreement on how this API would work within Konflux.
