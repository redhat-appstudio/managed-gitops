# Bump ArgoCD version

This document describes the process that is used to upgrade the GitOps Service' Argo CD go.mod dependencies to a newer version.

1. Determine the desired version of ArgoCD that you want to use. This can be done by visiting the ArgoCD releases page on GitHub (<https://github.com/argoproj/argo-cd/releases>) and selecting the desired version.

2. In the `Makefile`, update the `ARGO_CD_VERSION` variable to the desired version.

3. In the `load-test` main file, update the reference to the ArgoCD installation manifest to use the desired version. This can be done by updating the version number on the following line (see the following examples):

    - In `main.go`, line 9: `const ARGO_CD_VERSION = "v2.7.11"`

4. In the `cluster-agent/go.mod`, `tests-e2e/go.mod` and `utilities/load-test/go.mod` file, update the argo-cd/argocd dependency to use the desired version. This can be done by updating the version number in the following line of code:

    - Line 7: `github.com/argoproj/argo-cd/v2 v2.7.11`

5. In `cluster-agent/go.mod`, `tests-e2e/go.mod` and `utilities/load-test/go.mod` make sure to take into account the `replace` directives from ArgoCD. This means that when upgrading to the latest Argo CD, you will likely need to copy this replace directive (https://github.com/argoproj/argo-cd/blob/ec195adad84c61c6151d553b9fdce3c258b1325d/go.mod#L261) from Argo CD's `go.mod` into  `cluster-agent/go.mod`, `tests-e2e/go.mod` and `utilities/load-test/go.mod`.

6. Run `make tidy` from the root of the repo.

7. If during `go vet` you get an error similar to:

```shell
vet: utils/argocd_login_command_test.go:28:54: cannot use mockAppClient (variable of type *mocks.Client) as "github.com/argoproj/argo-cd/v2/pkg/apiclient".Client value in argument to argoCDLoginCommand: *mocks.Client does not implement "github.com/argoproj/argo-cd/v2/pkg/apiclient".Client (missing method NewApplicationSetClient)
```

this means the new ArgoCD version has introduced changes, for which you do not have the required mock files.

8. Finally, you may need to regenerate the mocks from the new Argo CD version. You can run `make mocks` from the repository root. Make sure you are using the latest version of mockery (i.e. `go install github.com/vektra/mockery/v2@latest`)


# Deprecated: Old mechanism for regenerating mocks

To regenerate the new mock files, go to `cd cluster-agent/utils/mocks` and follow a similar prodecure to this:

- Install the mockery tool using the following command:

```bash
go install github.com/vektra/mockery/v2@latest
```

- Clone the argo-cd repository:

```bash
git clone git@github.com:argoproj/argo-cd
```

- Check out the `release-2.7` branch (in your case, use the branch of the version you want to upgrade into) of the argo-cd repository:

```bash
cd argo-cd
git switch release-2.7
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
cp argo-cd/pkg/apiclient/application/mocks/ApplicationServiceClient.go .
```

- Navigate to the `session` package in the `argo-cd` repository:

```bash
cd argo-cd/pkg/apiclient
grep -rwn . -e 'SessionServiceClient'
cd session
```

- Generate a mock version of the `SessionServiceClient` interface using the `mockery` tool:

```bash
mockery --name SessionServiceClient
```

- Copy the generated mock version of the `SessionServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cd ../../../../
cp argo-cd/pkg/apiclient/session/mocks/SessionServiceClient.go .
```

- Navigate to the `settings` package in the `argo-cd` repository:

```bash
cd argo-cd/pkg/apiclient
grep -rwn . -e 'SettingsServiceClient'
cd settings
```

- Generate a mock version of the `SettingsServiceClient` interface using the `mockery` tool:

```bash
mockery --name SettingsServiceClient
```

- Copy the generated mock version of the `SettingsServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cd ../../../../
cp argo-cd/pkg/apiclient/settings/mocks/SettingsServiceClient.go .
```

- Navigate to the `version` package in the `mocks` directory:

```bash
cd argo-cd/pkg/apiclient
grep -rwn . -e 'VersionServiceClient'
cd version
```

- Generate a mock version of the `VersionServiceClient` interface using the `mockery` tool:

```bash
mockery --name VersionServiceClient
```

- Copy the generated mock version of the `VersionServiceClient` interface to the `mocks` directory in the `cluster-agent/utils` directory:

```bash
cd ../../../../
cp argo-cd/pkg/apiclient/version/mocks/VersionServiceClient.go .
```

- Generate the new Client:

```bash
cd argo-cd/pkg/apiclient
mockery --name Client
```

- Move it back to `cluster-agent/utils/mocks` as well

```bash
cd ../../../
cp argo-cd/pkg/apiclient/mocks/Client.go .
```
- Delete the argo-cd directory

```bash
rm -fr argo-cd
```
