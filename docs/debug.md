# Debugging

## Issues with the Database

## Run PostgreSQL in Docker

You can run the `./create-dev-env.sh` script which will start two containers:

* **managed-gitops-postgres** - running the database
* **managed-gitops-pgadmin** - PGAdmin Dashboard (optional)

To reset the database to a clean state, run `./reset-db.sh`.


## Run PostgreSQL in Kubernetes

* **Note**: When developing in your local development environment, it is usually easier to run PostgreSQL on your local machine using `create-dev-env.sh`, as described above.

### Deploy PostgreSQL

To install it into the Kubernetes cluster:

```shell
 ./create-dev-env.sh kube
```

### Access PostgreSQL outside the cluster

**Note**: See also the `make port-forward-postgres-auto` and `make port-forward-postgres-manual` Makefile targes.

To be able to access the database from outside the cluster you need to forward it locally:

```shell
$ kubectl port-forward --namespace gitops-postgresql svc/gitops-postgresql-staging 5432:5432 &
# Press Enter
# Don't forget to kill it when you are finished: 'fg' and then 'CTRL+C'
```

Then run a `psql` command, to verify your access:

```shell
psql postgresql://postgres:$PASSWORD@127.0.0.1:5432/postgres -c "select now()"
```

_Note: If you are using a plain-text hardcoded password, replace `$PASSWORD` var with yours_.

Expected Output:

```shell
Handling connection for 5432
              now
-------------------------------
 2022-01-25 15:37:07.826248+00
(1 row)
```



### Access PostgreSQL within the cluster

From within the cluster, the `coredns` pod will be able to resolve the _service_ name, and you will also need to fetch the `secret`.
For example, the `gitops-service-backend` has the following configuration snippet into its deployment YAML:

```yaml
        env:
          - name: DB_ADDR
            value: gitops-postgresql-staging.gitops-postgresql
          - name: DB_PASS
            valueFrom:
              secretKeyRef:
                name: gitops-postgresql-staging
                key: postgresql-password
        image: quay.io/pgeorgia/gitops-service:latest
```

When the `manager` container is running, it fetches those ENV variables and uses them to connect to the database.
For more information, read the [connectToDatabase] function.

#### Verify the DNS is working

The Pods within the cluster should be able to resolve the name of the services.

```shell
kubectl apply -f https://k8s.io/examples/admin/dns/dnsutils.yaml
```

To make sure the coredns works correctly within the cluster, try this:

```shell
$ kubectl exec -i -t dnsutils -- nslookup gitops-postgresql-staging-headless.gitops-postgresql

Server:		10.96.0.10
Address:	10.96.0.10#53

Name:	gitops-postgresql-staging-headless.gitops-postgresql.svc.cluster.local
Address: 10.1.0.6
```

```shell
$ kubectl exec -i -t dnsutils -- nslookup gitops-postgresql-staging.gitops-postgresql

Server:  10.96.0.10
Address: 10.96.0.10#53

Name: gitops-postgresql-staging.gitops-postgresql.svc.cluster.local
Address: 10.96.236.173
```

#### Connect to the database

Spawn a Postgres Pod and try to access the database using the service.

```shell
kubectl run pgsql-postgresql-client --rm --tty -i --restart='Never' --namespace gitops-postgresql --image docker.io/bitnami/postgresql:11.7.0-debian-10-r9 --env="PGPASSWORD=$PASSWORD" --command -- psql testdb --host gitops-postgresql-staging-headless.gitops-postgresql -U postgres -d postgres -p 5432
```

If you don't see a command prompt, try pressing **Enter** key.
