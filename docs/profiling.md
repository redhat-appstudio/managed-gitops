# Profiling

[Profiling](https://go.dev/doc/diagnostics) is an important technique to diagnose logic and performance problems in a program. Profiling tools analyze the complexity and costs of a Go program such as its memory usage and frequently called functions to identify the expensive sections of a Go program.

GitOps Service exposes the pprof endpoints via the [http/pprof](https://pkg.go.dev/net/http/pprof) package and can be enabled with `ENABLE_PROFILING=true` environment variable. The profiling data can then be used by pprof visualization tools for analyzing the performance issues. The pprof endpoints for GitOps service components are listed below:

| Component                |    Endpoint                |
|--------------------------|----------------------------|
| Backend Controller       | localhost:6060/debug/pprof |
| Cluster-Agent Controller | localhost:6061/debug/pprof |
| AppStudio Controller     | localhost:6062/debug/pprof |

## Profiling using `go tool pprof`

Go provides in-built support for profiling via the go tool pprof command. You can see the full list of profiles at `localhost:<port>/debug/pprof`

To analyze a particular profile:

```shell
go tool pprof http://localhost:<pprof>/debug/pprof/<profile>
```

For example, run the below command to investigate heap allocations of the backend controller

```shell
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Continous Profiling using Parca

[Parca](https://www.parca.dev/) is an Open Source continous profiling tool to analyze the profiles of services deployed on Kubernetes. Follow the below steps to use Parca with GitOps Service

1. Create a `parca.yaml` to configure Parca to target GitOps Service pprof endpoints.  

```yaml
object_storage:
  bucket:
    type: "FILESYSTEM"
    config:
      directory: "./tmp"

scrape_configs:
  - job_name: "backend"
    scrape_interval: "3s"
    static_configs:
      - targets: [ '127.0.0.1:6060' ]
  - job_name: "cluster-agent"
    scrape_interval: "3s"
    static_configs:
      - targets: [ '127.0.0.1:6061' ]
  - job_name: "appstudio-controller"
    scrape_interval: "3s"
    static_configs:
      - targets: [ '127.0.0.1:6062' ]

```

2. Run Parca in a container

```shell
docker run --rm -it --network="host" -v $HOME/managed-gitops/parca.yaml:/parca.yaml ghcr.io/parca-dev/parca:v0.16.2 /parca --config-path="parca.yaml"
```

3. Navigate to `http://localhost:7070/targets` and verify if all the targets are in "Good" health status.

4. Select a profile from the dropdown menu to visualize the icicle graph.

