# Usage of a relational database, as described in the architecture document, rather than storing configuration/state in Kubernetes resources

Written by: 
- Jonathan West (@jgwest)


**Using a relational database does not prevent users from managing the GitOps managed service with Kubernetes CRs**

Using a relational database to track the internal managed entities of the GitOps managed service does NOT prevent us from allowing users to interact with the service via Kubernetes CRs.

For example, you can imagine a case where the user creates something like this:

```yaml
apiVersion: redhat.com/managed-gitops/v1alpha1
kind: ManagedEnvironmentCredentials
metadata:
  name: my-cluster-credentials
spec:
  credentials:
    // (…)
```


E.g. they have a cluster they want to use Argo CD to deploy to, and so they provide us credentials using a custom CR.t

While support for this is not part of first managed-gitops iteration, it would be straightforward to add support for this:

- The gitops-backend service would watch user namespaces for the ManagedEnvironmentCredentials CR
- On creation, the gitops-backend sees this new CR
- It then updates the corresponding Managed Cluster Environment in the RDBMS database (and informs the cluster agent of the change).

Thus, using something like [kcp](https://github.com/kcp-dev/kcp), it is straightforward to extend the architecture to add a controller to watch K8S APIs on a kcp control plane, and to respond to these with events in the relational database and on the cluster. (In fact, this would just be a replacement for the REST API of the backend service, replacing it with a controller watching a KCP cluster)



**Suitability for providing fast responses to UI requests**

In our case, the persistence store (database) needs to be read by a number of different components, including the backend, and cluster operator. 

The backend, specifically, will be responsible for serving UI requests, and (as with any UI) we need the page load times for those UI requests to be as fast as possible. This means we need to store the content with a form of persistence which is lightweight, performant, and scalable. 

For instance, one case we have is we might need to retrieve the sync/health status of hundreds of user Argo CD Applications, at once, as quickly as possible. My expectation is that an RDBMS like Postgres (or Redis) would work better for this purpose than the Kubernetes API server. 

**Transactions**:

There are a number of resources that we will be required to manage, and these resources may necessarily contain references to one another that need to be kept in sync. 

The easiest way to ensure a set of resources are consistently updated together is with RDBMS transactions. In contrast, Kubernetes does not have the ability to bulk update a set of resources, which means you have to update them one at a time (perhaps over a number of threads). This ‘one at a time’  approach potentially leads to incomplete reads from consumers, where some of the resources have been updated and some have not (an inconsistent state).

**SQL (Efficient use of network bandwidth, efficient joins)**:

With an RDBMS, you may use SQL to exactly shape the specific SELECT query parameters you are looking for. For example, if you are only looking for the ‘username’ and ‘cluster id’ fields of a table, you may request these in a SELECT query, and discard the others.

In contrast, with Kubernetes you must acquire the entire resource contents (spec, status, etc) in order to see the contents of any individual fields (though you do have a limited ability to filter which resources are returned).

Likewise, with SQL JOINs, you can combine the results of multiple tables together; in contrast, with the Kubernetes API, you would need to retrieve each individual entry and join it yourself, which increases the CPU usage and network I/O.

- This is especially true, when defining secondary indexes on fields of a database, for speeding up queries that are not based (strictly) on the primary key.

**Backup / Disaster Recovery**:

There are many tools, documents, processes, and cloud services available for backing up and restoring a PostgreSQL database. While it is reasonably straightforward to backup and restore the YAML of a Kubernetes server, it requires a more hands on approach vs PostgreSQL (potentially introducing errors into the process).

**Foreign Keys**:

It’s beneficial to be able to rely on the consistency of relationships between entities in the database, as reinforced through foreign keys.

**Strong typing:**

I’m personally a fan of strongly typed schemas: using tables to define a strict set of data types, rather than unstructured, schema-less JSON, traditionally used in a NoSQLDB.

**RDBMS databases are the traditional tool for this job**

In all software engineering projects, we should aim to use ‘the right tool for the job’. We should ask: what technology is specialized for this purpose? And, which technology has the industry had success using to implement similar projects?

RDBMS databases are the traditional tool for creating, querying, and updating a list 

of interrelated entities, in a durable, consistent, reasonably performant manner.

Imagine I am a software developer working for a bank, which is creating a backend bank app. That app needs to store a list of customers, and a list of credit ratings. In all likelihood I’m going to do it using an RDBMS. I’m not going to use a Kubernetes namespace with CreditRating and Customer CRs. This use case is no different.

We as Kubernetes/OpenShift developers are very familiar with the ins and outs of Kubernetes resources, and the Kubernetes control plane, but as the saying goes, “When all you have is a hammer, every problem starts to look like a nail”.


**“Easier” to move from RDBMS, to other databases (NoSQL/Redis), than the vice versa**

Since RDBMS are strongly typed, with foreign key constraints, it is “easier” to move from an RDBMS, to another database technology (Redis, NoSQL), than to go in the opposite direction.

For example, if we wanted the benefits of better scaling offered by a key-value store database (such as Redis or etcd) it is straightforward to migrate the appropriate database tables with a simple translation script. 

Likewise, moving to NoSQL (Mongo) would be a matter of a simple translation script that iterated through the tables, to produce the corresponding JSON objects.

(I say ‘easier’, because databases also have difference semantics, for example ACID vs CAP and eventual consistency, which may break the product code in non-obvious ways)
