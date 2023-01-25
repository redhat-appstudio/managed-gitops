# Bump ArgoCD version

1. Determine the desired version of ArgoCD that you want to use. This can be done by visiting the ArgoCD releases page on GitHub (<https://github.com/argoproj/argo-cd/releases>) and selecting the desired version.

2. In the `Makefile`, update the `ARGO_CD_VERSION` variable to the desired version. This variable is used in several places in the `Makefile`, so you will need to update all instances of it.

3. In the `argocd.sh` script, update the reference to the ArgoCD installation manifest to use the desired version. This can be done by updating the version number in the following lines of code (see the following examples):

    - Line 31: `kubectl apply -n "$ARGO_CD_NAMESPACE" -f https://raw.githubusercontent.com/argoproj/argo-cd/$ARGO_CD_VERSION/manifests/install.yaml`
    - Line 107: `kubectl delete -n "$ARGO_CD_NAMESPACE" -f https://raw.githubusercontent.com/argoproj/argo-cd/$ARGO_CD_VERSION/manifests/install.yaml`

4. In the `load-test` utility and main script, update the reference to the ArgoCD installation manifest to use the desired version. This can be done by updating the version number in the following lines of code (see the following examples):

    - In `load-test/utility.go`, line 76: <https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml>

    - In `main.go`, line 9: <https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml>

5. In the `deploy.sh` script, update the reference to the ArgoCD installation manifest to use the desired version. This can be done by updating the version number in the following line of code (see the following example):

    - Line 6: <https://raw.githubusercontent.com/argoproj/argo-cd/$ARGO_CD_VERSION/manifests/install.yaml>

6. In the `manifests/k8s-argo-deploy/deploy.yaml` file, update the `image` field in the `argocd-server` and `argocd-repo-server` containers to use the desired version. This can be done by updating the version number in the following lines of code (see the example):

    - Line 16: `image: argoproj/argocd:$ARGO_CD_VERSION`
    - Line 26: `image: argoproj/argocd-repo-server:$ARGO_CD_VERSION`

7. In the `cluster-agent/go.mod` file, update the argo-cd/argocd dependency to use the desired version. This can be done by updating the version number in the following line of code:

    - Line 5: `argo-cd/argocd v2.4.0`

8. In the `tests-e2e/go.mod` and `utilities/load-test/go.mod` make sure you run `go mod tidy` but take into account the `replace` directives from ArgoCD. This means that when upgrading to the latest Argo CD, you will likely need to copy this replace directive (https://github.com/argoproj/argo-cd/blob/81630e6d5075ac53ac60457b51343c2a09a666f4/go.mod#L251) from Argo CD's `go.mod` into `cluster-agent`'s `go.mod`.

9. If during `go vet` you get an error similar to:

```shell
vet: utils/argocd_login_command_test.go:28:54: cannot use mockAppClient (variable of type *mocks.Client) as "github.com/argoproj/argo-cd/v2/pkg/apiclient".Client value in argument to argoCDLoginCommand: *mocks.Client does not implement "github.com/argoproj/argo-cd/v2/pkg/apiclient".Client (missing method NewApplicationSetClient)
```

this means the new ArgoCD version has introduced changes, for which you do not have the required mock files.

To regenerate the new mock files, go to `cd cluster-agent/utils/mocks` and follow a similar prodecure to this:

- Install the mockery tool using the following command:

```bash
go install github.com/vektra/mockery/v2@latest
```

- Clone the argo-cd repository:

```bash
git clone git@github.com:argoproj/argo-cd
```

- Check out the `release-2.5` branch (in your case, use the branch of the version you want to upgrade into) of the argo-cd repository:

```bash
cd argo-cd
git checkout -b remotes/origin/release-2.5
```

- Navigate to the `application` package in the argo-cd repository:

```bash
cd pkg/apiclient/application
```

- Generate a mock version of the `ApplicationServiceClient` interface using the `mockery` tool:

```bash
mockery --name ApplicationServiceClient
```

- Copy the generated mock version of the `ApplicationServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cd ../../../../
cp ../argo-cd/pkg/apiclient/application/mocks/ApplicationServiceClient.go .
```

- Navigate to the `session` package in the `argo-cd` repository:

```bash
cd argo-cd/pkg/apiclient/application
grep -rwn . -e 'SessionServiceClient'
cd session
```

- Generate a mock version of the `SessionServiceClient` interface using the `mockery` tool:

```bash
mockery --name SessionServiceClient
```

- Copy the generated mock version of the `SessionServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cp mocks/SessionServiceClient.go ../../../../mocks/SessionServiceClient.go
```

- Navigate to the `settings` package in the `argo-cd` repository:

```bash
grep -rwn . -e 'SettingsServiceClient'
cd settings
```

- Generate a mock version of the `SettingsServiceClient` interface using the `mockery` tool:

```bash
mockery --name SettingsServiceClient
```

- Copy the generated mock version of the `SettingsServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cp mocks/SettingsServiceClient.go ../../../../mocks/SettingsServiceClient.go
```

- Navigate to the `version` package in the `mocks` directory:

```bash
cd mocks
grep -rwn . -e 'VersionServiceClient'
cd version
```

- Generate a mock version of the `VersionServiceClient` interface using the `mockery` tool:

```bash
mockery --name VersionServiceClient
```

- Copy the generated mock version of the `VersionServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cp mocks/VersionServiceClient.go ../../../../mocks/VersionServiceClient.go
```

- Generate the new Client:

```bash
mockery --name Client
```

- Move it back to `cluster-agent/utils` as well: `cp mocks/Client.go ../../../mocks/Client.go`
