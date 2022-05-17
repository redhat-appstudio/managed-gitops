
# GitOps Service E2E Tests

The Managed GitOps Service aims to have several types of test coverage: both units tests (utilizing mocking to test individual functions), and E2E tests (running on an actual K8s cluster).

E2E tests are different from unit tests, in that they test against an actual K8s cluster (rather a mocked k8s fake client), often using actual container images and/or container workloads. E2E tests should aim to run in an environment that is as close to production as possible (unlike unit tests or a local development environment).

Our E2E tests (unlike our unit tests) are [black box tests](https://en.wikipedia.org/wiki/Black-box_testing): we invoke the GitOps Service external API (e.g. we create/modify/delete `GitOpsDeployment` resources), and watch the Kubernetes API for expected changes to resources.


## Usage

You should be able to run the E2E tests on your local development environment as follows:

1) Ensure you are logged into an OpenShift cluster (ideally, a disposable cluster, such as a 'clusterbot bot' cluster)
    - Using a non-OpenShift cluster to run these tests is left as an exercise to the reader.

2) Run the following commands from the `e2e-tests` folder:

    ```
    # Install Argo CD into gitops-service-argocd, using OpenShift GitOps operator
    make setup-e2e-openshift

    # Start GitOps Service (in E2E configuration)
    make start-e2e

    # In a separate terminal window, to run the tests:
    make test-e2e
    ```

## OpenShift CI

E2E tests should likewise run on each PR, via OpenShift CI.

