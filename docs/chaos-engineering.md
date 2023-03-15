# Chaos Engineering

## Introduction

Chaos Engineering is an [industry term](https://en.wikipedia.org/wiki/Chaos_engineering) used to describe testing software under degraded conditions, in order to ensure it continues to perform correctly, and/or degrades gracefully.

A number of existing tools (ChaosMonkey, Litmus Chaos, etc) are available that can integrate with Kubernetes, and simulate failure scenarios like container OOM, poor network condition, and more. However, within the GitOps Service, we are using custom code to simulate failures, rather than relying on these external tools.

## Within the GitOps Service

Within the GitOps Service, there are three types of chaos-engineering-based error scenarios that can be simulated:
- Simulate an unreliable database by randomly returning errors on database API calls before they can be completed.
- Simulate unreliable cluster connection by randomly returning errors on k8s client API calls
- Randomly restarting the gitops controller processes (either 'kill -9' the process, or kill the pod) during the test.

## How to test

The E2E test framework used to test the GitOps Service should be able to be run in a 'chaos engineering' mode, which simulates a variety of error conditions while the normal E2E tests are being run. The tests should still (eventually) pass, despite running in unreliable circumstances.

Furthermore, the test should consistently pass, when looped over-and-and in these conditions. For example, if one sets the E2E tests to run in a loop, for 100 runs, over and over, all 100 should pass, despite the above.

### Simulating an unreliable database

This functionality can be used to simulate unreliable database connections. This code will that randomly inject database errors, which allow us to verify that our service tolerates this and/or fails gracefully.

In order to enable this functionality, the `ENABLE_UNRELIABLE_DB=true` environment variable must be set, before running the GitOps Service controllers.

The `UNRELIABLE_DB_FAILURE_RATE=X` environment variable is used to control what % of database API calls will fail. 
- For example, `UNRELIABLE_DB_FAILURE_RATE=20` will cause 20% of database calls to fail.
- Increase this value to increase the simulated severity of failures

To run the E2E tests with this functionality enabled:
```shell
# Setup GitOps Service E2E tests pre-requisites on OpenShift cluster
make setup-e2e-openshift

# Start GitOps Service, in E2E configuration, with database chaos engineering enabled
ENABLE_UNRELIABLE_DB=true UNRELIABLE_DB_FAILURE_RATE=20  make start

# In a separate terminal window, to run the tests:
make test-e2e
```

Even with X% of the database requests failing, the E2E tests should still pass.


### Simulating an unreliable cluster connection

This functionaly can be used to simulate the impact of unreliable network conditions, or a flaky K8s API server, on the GitOps Service controllers.

This functionality is enabled via an environment variable, which we enable when running E2E tests with chaos engineering. By running E2E tests with this environment variable set, this will allow us to ensure that our product works even when running in less reliable conditions.

The unreliable K8s client functionality will randomly fail a certain percent of K8s API requests, and that % is controllable via environment variable.

In order to enable this functionality, the `ENABLE_UNRELIABLE_CLIENT=true` environment variable must be set, before running the GitOps Service controllers.

The `UNRELIABLE_CLIENT_FAILURE_RATE=X` environment variable is used to control what % of K8s requests will fail. 
- For example, `UNRELIABLE_CLIENT_FAILURE_RATE=20` will cause 20% of requests to fail.

To run the E2E tests with this functionality enabled:
```shell
# Setup GitOps Service E2E tests pre-requisites on OpenShift cluster
make setup-e2e-openshift

# Start GitOps Service, in E2E configuration, with K8s client chaos engineering enabled
ENABLE_UNRELIABLE_CLIENT=true  UNRELIABLE_CLIENT_FAILURE_RATE=20  make start

# In a separate terminal window, to run the tests:
make test-e2e
```


### Simulating an OOM-ing container process

This functionality can be used to simulate a container that is being restarted by K8s, for example due to the container intermittently OOMing, or panic-ing.

When running `make start-chaos`, the GitOps Service controllers will be started using a special shell script that will randomly kill and restart the controller process. The interval of restarts is controlled via two environment variables:
- `KILL_INITIAL="10"`: The minimum number of seconds to wait for killing and restarting the process. In this example, 10 seconds.
- `KILL_INTERVAL="45"`: A random value is chosen between 0, and the `KILL_INTERVAL`. `KILL_INTERVAL` is the maximum length of time to wait (after initial wait) before killing and restarting the process. In this example, 45 seconds.

The average length of time between restarts for any process is thus `KILL_INITIAL + rand(0, KILL_INTERVAL)`

To run the E2E tests with this functionality enabled:
```shell
# Setup GitOps Service E2E tests pre-requisites on OpenShift cluster
make setup-e2e-openshift

# Start GitOps Service, in E2E configuration, with process restart chaos engineering enabled
make start-chaos

# In a separate terminal window, to run the tests:
make test-e2e
```

You will see that every X seconds, the controller processes are killed and restarted. For example:
```shell
02:04:10 appstudio-controller | ../utilities/random-kill.sh: line 15: 109177 Killed                  $*
02:04:10 appstudio-controller | Killing process: 109280
02:04:10 appstudio-controller | New PID is 109388
02:04:10 appstudio-controller | Next kill command in 40s
02:04:12 appstudio-controller | INFO	setup	starting manager
02:04:12 appstudio-controller | INFO	Starting server	{"kind": "health probe", "addr": "[::]:8085"}
02:04:12 appstudio-controller | INFO	Starting server	{"path": "/metrics", "kind": "metrics", "addr": "[::]:8084"}
# (...)
```

Even with this going on, the E2E tests should still pass.
