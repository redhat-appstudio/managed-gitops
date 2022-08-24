# GitOps Service on KCP

Please follow the below steps to start GitOps Service and AppStudio controllers against a KCP workspace.

1. Access to a running KCP/CPS instance is a prerequisite to proceed further.
2. Create a source workspace and service-provider workspace. We start our controllers against the service-provider workspace and export the APIs via APIExport. The source workspace will subscribe to these exported APIs with the help of APIBindings. The controllers react to the events in the source workspace via Virtual Workspaces.

    ```shell
    kubectl kcp ws create source
    kubectl kcp ws create service-provider
    ```

3. Enter the service-provider workspace and create APIExport and APIResourceSchema manifests. Note down the service-provider workspace path.

    ```shell
    kubectl kcp ws use service-provider
    make apply-kcp-api-all

    kubectl kcp ws . 
    ```

4. Enter the source workspace and create APIBindings

    ```shell
    kubectl kcp ws ..
    kubectl kcp use source

    cat <<EOF | kubectl apply -n $ARGO_CD_NAMESPACE -f -
    apiVersion: apis.kcp.dev/v1alpha1
    kind: APIBinding
    metadata:
    name: gitopsrvc-backend-shared
    spec:
    acceptedPermissionClaims:
    - group: ""
        resource: "secrets"
    - group: ""
        resource: "namespaces"
    reference:
        workspace:
        path: root:rh-sso-15850190:service-provider-qrqtx <------ Update the path from the Step 4
        exportName: gitopsrvc-backend-shared

    ---

    apiVersion: apis.kcp.dev/v1alpha1
    kind: APIBinding
    metadata:
    name: gitopsrvc-appstudio-shared
    spec:
    acceptedPermissionClaims:
    - group: ""
        resource: "secrets"
    - group: ""
        resource: "namespaces"
    reference:
        workspace:
        path: root:rh-sso-15850190:service-provider-qrqtx <------ Update the path from the Step 4
        exportName: gitopsrvc-appstudio-shared
    EOF
    ```

5. In the source namespace verify if the APIBIndings have recognized our AppStudio and GitOps service resources.

    ```shell
    kubectl api-resources
    ```

6. Enter the service-provider workspace and start our controllers.

    ```shell
    kubectl kcp ws ..
    kubectl kcp use service-provider
    make start
    ```

7. Verify if the controllers start without any errors.
