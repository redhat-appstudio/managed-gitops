# Install

This workflow will install the two controllers, [Backend] and [Cluster-Agent], by pulling their container images from the container registry respectively.
Thus, to deploy them into Kubernetes, make sure you have _already build and push_ the container image to your registry.
For example: `make docker-build docker-push IMG=quay.io/pgeorgia/gitops-service:latest`

As soon as you have a container image into your registry, you are ready to deploy the backend controller:
Given your `kube-config` is already pointing to your Kubernetes cluster, then do:

```bash
make install-all-k8s IMG=quay.io/pgeorgia/gitops-service:latest # replace the IMG var with yours
```

This will automatically install all the required components into your Kubernetes cluster, in the `gitops` namespace.
Notice that, the [Cluster-Agent] controller requires the `gitops-service-argocd` namespace which will also be created as well.

```shell
$ kubectl -n gitops get all
NAME                                                       READY   STATUS    RESTARTS   AGE
pod/gitops-postgresql-staging-postgresql-0                 1/1     Running   0          32s
pod/managed-gitops-backend-service-77c9648b5b-hgkt4        2/2     Running   0          17s
pod/managed-gitops-clusteragent-service-85d4db6968-gl98l   2/2     Running   0          13s

NAME                                                                     TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
service/gitops-postgresql-staging                                        ClusterIP   10.103.184.2   <none>        5432/TCP   33s
service/gitops-postgresql-staging-headless                               ClusterIP   None           <none>        5432/TCP   33s
service/managed-gitops-backend-controller-manager-metrics-service        ClusterIP   10.96.158.85   <none>        8443/TCP   17s
service/managed-gitops-clusteragent-controller-manager-metrics-service   ClusterIP   10.108.6.120   <none>        8443/TCP   14s

NAME                                                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/managed-gitops-backend-service        1/1     1            1           17s
deployment.apps/managed-gitops-clusteragent-service   1/1     1            1           13s

NAME                                                             DESIRED   CURRENT   READY   AGE
replicaset.apps/managed-gitops-backend-service-77c9648b5b        1         1         1       17s
replicaset.apps/managed-gitops-clusteragent-service-85d4db6968   1         1         1       13s

NAME                                                    READY   AGE
statefulset.apps/gitops-postgresql-staging-postgresql   1/1     33s
```

For local development, it's not practical to _push_ to the registry everytime you want to locally test your changes.
If this is your intention, then please follow the [development workflow](./development.md).

## Uninstall

To uninstall it completely, make sure you do not run any other important resources inside the `gitops` namespace, and then do:

```shell
make uninstall-all-k8s
```

This will remove everything the previous command installed, and finally it will also delete the `gitops` and `gitops-service-argocd` namespaces as well.

[Backend Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
