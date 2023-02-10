# Development

## Build the components

Use either the main Makefile or the Makefile of each individual component (you can also call those from the main Makefile as well).
See the following _build_ targets:

```shell
make build                          # Build all the components
make build-backend                  # Build backend only
make build-cluster-agent            # Build cluster-agent only
make build-appstudio-controller     # Build appstudio-controller only
make docker-build                   # Build docker image
```

#### Support for multiple platforms

The `make` targets can be configured to build for other operating systems and/or architectures,
by explicitly setting the `OS` and `ARCH` environment variables.

```shell
OS=linux ARCH=amd64 make build      # Build all components for platform linux/amd64
OS=linux ARCH=arm64 make build      # Build all components for platform linux/arm64
```
```shell
OS=darwin ARCH=amd64 make build     # Build all components for platform darwin/amd64
OS=darwin ARCH=arm64 make build     # Build all components for platform darwin/arm64
```

### Build Container images

The project uses just one single base image from which you can select which `binary` you would like to run.
It basically clones the repository and builds each individual component using their own Makefile respectively.
Then it copies over their binaries into `/usr/local/bin`.
For more information, look at the Dockerfile.
By default it will run `/bin/bash` shell and it uses a `non-root` user.

#### Build and Push
##### Using Docker (default)

```shell
# Login to the contaier registry (e.g. quay.io)
docker login quay.io

# Optional:
# You can override the name with provide your own username, image name, and tag
# by adding the 'IMG' variable: IMG=quay.io/USERNAME/IMAGE_NAME:TAG
# By default, the name would be 'gitops-service:latest' (defined in the Makefile)
# ----
# The IMG variable consists of other variables, such as:
#  * by adding 'USERNAME' variable you need to be logged into your registry.
#  * by adding 'BASE_IMAGE' variable you can change only the name of the image.
#  * by adding 'TAG' variable you can change only the version tag of the image

# Build the container
make docker-build

# Push to registry
make docker-push -USERNAME=drpaneas

# Or combine them
make docker-build docker-push IMG=quay.io/$USERNAME/$IMAGE_NAME:$TAG

# Other optional variables
make docker-build BASE_IMAGE="foo" TAG="bar"
```

##### Using Podman

```shell
# Login to the contaier registry (e.g. quay.io)
podman login quay.io

# Build the container
DOCKER=podman make docker-build

# Push to registry
DOCKER=podman make docker-push -USERNAME=drpaneas

# Or combine them
DOCKER=podman make docker-build docker-push IMG=quay.io/$USERNAME/$IMAGE_NAME:$TAG

# Other optional variables
DOCKER=podman make docker-build BASE_IMAGE="foo" TAG="bar"
```
#### Run the containers

```shell
docker run --rm -it gitops-service:latest gitops-service-backend
docker run --rm -it gitops-service:latest gitops-service-cluster-agent
```

or if you need to debug the image:

```shell
docker run --rm -it gitops-service:latest # to get /bin/bash
```

[Backend Shared]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend-shared
[Backend]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend
[Cluster-Agent]: https://github.com/redhat-appstudio/managed-gitops/tree/main/cluster-agent
[Load Test]: https://github.com/redhat-appstudio/managed-gitops/tree/main/utilities/load-test#argo-cd-load-test-utility
[Manifests]: https://github.com/redhat-appstudio/managed-gitops/tree/main/manifests
[KinD]: https://kind.sigs.k8s.io/docs/user/quick-start/
[k3s]: https://k3s.io/
[EventLoop]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/eventloop
[ArgoCD Application CR]: https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/
[Another Event-Loop]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/managed-gitops/eventloop
[GitOps Operation Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/managed-gitops/operation_controller.go
[ArgoCD Application Controller]: https://github.com/redhat-appstudio/managed-gitops/blob/main/cluster-agent/controllers/argoproj.io/application_controller.go
[Docker]: https://www.docker.com/
[db-schema]: https://github.com/redhat-appstudio/managed-gitops/blob/main/db-schema.sql
[psql.sh]: https://github.com/redhat-appstudio/managed-gitops/blob/main/psql.sh
[Operation CRD]: https://github.com/redhat-appstudio/managed-gitops/blob/main/backend-shared/config/crd/bases/managed-gitops.redhat.com_operations.yaml
[routes]: https://github.com/redhat-appstudio/managed-gitops/tree/main/backend/routes
[Design]: https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.s0hdo22ap5cp
