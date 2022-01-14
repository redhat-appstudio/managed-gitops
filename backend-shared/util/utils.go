package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Whenever a new Argo CD Application needs to be created, we need to find an Argo CD instance
// that is available to use it. In the future, when we have multiple instances, there would
// be an algorithm that intelligently places applications -> instances, to ensure that
// there are no Argo CD instances that are overloaded (have too many users).
//
// However, at the moment we are using a single shared Argo CD instance in 'argocd', so we will
// just return that.
//
// This logic would be improved by https://issues.redhat.com/browse/GITOPS-1455 (and others)
const GitOpsEngineSingleInstanceNamespace = "argocd"

// TODO: GITOPS-1564 - Add tests for all scenarios, for each of these.
// See https://issues.redhat.com/browse/GITOPS-1564

func UncheckedGetOrCreateManagedEnvironmentByNamespaceUID(ctx context.Context, workspaceNamespace v1.Namespace, dbq db.DatabaseQueries, actionLog logr.Logger) (*db.ManagedEnvironment, error) {

	workspaceNamespaceUID := string(workspaceNamespace.UID)

	var err error

	dbResourceMapping := &db.KubernetesToDBResourceMapping{
		KubernetesResourceType: db.K8sToDBMapping_Namespace,
		KubernetesResourceUID:  string(workspaceNamespaceUID),
		DBRelationType:         db.K8sToDBMapping_ManagedEnvironment}
	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, dbResourceMapping)

	if err == nil {
		// If there already exists a managed environment for this workspace, return it

		managedEnvironment := db.ManagedEnvironment{Managedenvironment_id: string(dbResourceMapping.DBRelationKey)}
		err = dbq.UncheckedGetManagedEnvironmentById(ctx, &managedEnvironment)

		if err == nil {
			// mapping exists, and so does the environment it points to
			return &managedEnvironment, nil
		}

		if !db.IsResultNotFoundError(err) {
			return nil, fmt.Errorf("unable to retrieve resource mapping: %v", err)
		}

		// Since the managed environment doesn't exist, delete the mapping; it will be recreated below.
		if _, err := dbq.UncheckedDeleteKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
			return nil, fmt.Errorf("unable to delete K8s resource to DB mapping: %v", dbResourceMapping)
		}

	} else if !db.IsResultNotFoundError(err) {
		return nil, fmt.Errorf("unable to retrieve resource mapping: %v", err)
	}

	// At this point in the function, both the managed environment and mapping necessarily don't exist

	// Create cluster credentials for the managed env
	// TODO: GITOPS-1564 - Cluster credentials placeholder values - we will need to create a service account on the target cluster, which we can store in the database.
	// See https://issues.redhat.com/browse/GITOPS-1564

	clusterCreds := db.ClusterCredentials{
		Host:                        "host",
		Kube_config:                 "kube_config",
		Kube_config_context:         "kube_config_context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "serviceaccount_ns",
	}
	if err := dbq.CreateClusterCredentials(ctx, &clusterCreds); err != nil {
		return nil, fmt.Errorf("unable to create cluster creds for managed env: %v", err)
	}

	managedEnvironment := db.ManagedEnvironment{
		Name:                  "Managed Environment for " + workspaceNamespace.Name,
		Clustercredentials_id: clusterCreds.Clustercredentials_cred_id,
	}
	if err := dbq.CreateManagedEnvironment(ctx, &managedEnvironment); err != nil {
		return nil, fmt.Errorf("unable to create managed env: %v", err)
	}

	dbResourceMapping = &db.KubernetesToDBResourceMapping{
		KubernetesResourceType: db.K8sToDBMapping_Namespace,
		KubernetesResourceUID:  string(workspaceNamespaceUID),
		DBRelationType:         db.K8sToDBMapping_GitopsEngineCluster,
		// TODO: This seems wrong: ?????
		DBRelationKey: managedEnvironment.Managedenvironment_id,
	}

	if err := dbq.CreateKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
		return nil, fmt.Errorf("unable to create KubernetesResourceToDBResourceMapping: %v", err)
	}

	return &managedEnvironment, nil
}

// UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID gets (or creates it if it doesn't exist) a GitOpsEngineInstance database entry that
// corresponds to an GitOps engine instance running on the cluster.
func UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx context.Context,
	gitopsEngineNamespace v1.Namespace, kubesystemNamespaceUID string,
	dbq db.DatabaseQueries, log logr.Logger) (*db.GitopsEngineInstance, *db.GitopsEngineCluster, error) {

	// First create the GitOpsEngine cluster if needed; this will be used to create the instance.
	gitopsEngineCluster, err := UncheckedGetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, kubesystemNamespaceUID, dbq, log)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create GitOpsEngineCluster for '%v', error: '%v'", kubesystemNamespaceUID, err)
	}

	var gitopsEngineInstance *db.GitopsEngineInstance

	expectedDBResourceMapping := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: db.K8sToDBMapping_Namespace,
		KubernetesResourceUID:  kubesystemNamespaceUID,
		DBRelationType:         db.K8sToDBMapping_GitopsEngineInstance,
	}

	var dbResourceMapping *db.KubernetesToDBResourceMapping
	{
		var expectedDBResourceMappingCopy db.KubernetesToDBResourceMapping = expectedDBResourceMapping
		dbResourceMapping = &expectedDBResourceMappingCopy
	}

	if err := dbq.GetDBResourceMappingForKubernetesResource(ctx, dbResourceMapping); err != nil {

		if !db.IsResultNotFoundError(err) {
			return nil, nil, fmt.Errorf("unable to get DBResourceMapping for getOrCreateGitopsEngineInstanceByInstanceNamespaceUID: %v", err)
		}

		dbResourceMapping = nil

	} else {
		// If there exists a db resource mapping for this cluster, see if we can get the GitOpsEngineCluster
		gitopsEngineInstance = &db.GitopsEngineInstance{
			Gitopsengineinstance_id: dbResourceMapping.DBRelationKey,
		}

		if err := dbq.UncheckedGetGitopsEngineInstanceById(ctx, gitopsEngineInstance); err != nil {
			if !db.IsResultNotFoundError(err) {
				return nil, nil, err
			}

			log.V(LogLevel_Warn).Error(nil, "UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID found a resource mapping, but no engine instance.")

			// We have found a mapping without the corresponding mapped entity, so delete the mapping.
			// (We will recreate the mapping below)
			if _, err := dbq.UncheckedDeleteKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
				return nil, nil, err
			}

			gitopsEngineInstance = nil
			dbResourceMapping = nil
		} else {

			if gitopsEngineInstance.EngineCluster_id != gitopsEngineCluster.Gitopsenginecluster_id {
				return nil, nil, fmt.Errorf("able to locate engine instance, and engine cluster, but they mismatched: instance id: %v, cluster id: %v",
					gitopsEngineInstance.Gitopsengineinstance_id, gitopsEngineCluster.Gitopsenginecluster_id)
			}

			// Success: both existed.
			return gitopsEngineInstance, gitopsEngineCluster, nil
		}
	}

	if dbResourceMapping == nil && gitopsEngineInstance == nil {
		// Scenario A) neither exists: create both

		gitopsEngineInstance = &db.GitopsEngineInstance{
			Namespace_name:   gitopsEngineNamespace.Name,
			Namespace_uid:    string(gitopsEngineNamespace.UID),
			EngineCluster_id: gitopsEngineCluster.Gitopsenginecluster_id,
		}

		if err := dbq.CreateGitopsEngineInstance(ctx, gitopsEngineInstance); err != nil {
			return nil, nil, fmt.Errorf("unable to create engine instance, when neither existed: %v", err)
		}

		expectedDBResourceMapping.DBRelationKey = gitopsEngineInstance.Gitopsengineinstance_id
		if err := dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &expectedDBResourceMapping); err != nil {
			return nil, nil, fmt.Errorf("unable to create mapping when neither existed: %v", err)
		}

		return gitopsEngineInstance, gitopsEngineCluster, nil

	} else if dbResourceMapping != nil && gitopsEngineInstance == nil {
		// Scenario B) this shouldn't happen: the above logic should ensure that dbResourceMapping is always nil, if gitopsEngineInstance is nil
		return nil, nil, fmt.Errorf("SEVERE: the dbResourceMapping existed, but the gitops engine instance did not")

	} else if dbResourceMapping == nil && gitopsEngineInstance != nil {
		// Scenario C) this will happen if the instance exists, but there is no mapping for it

		expectedDBResourceMapping.DBRelationKey = gitopsEngineInstance.Gitopsengineinstance_id
		if err := dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &expectedDBResourceMapping); err != nil {
			return nil, nil, fmt.Errorf("unable to create mapping when dbResourceMapping didn't exist: %v", err)
		}

		return gitopsEngineInstance, gitopsEngineCluster, nil

	} else if dbResourceMapping != nil && gitopsEngineInstance != nil {
		// Scenario D) both exist, so just return the cluster

		if gitopsEngineInstance.EngineCluster_id != gitopsEngineCluster.Gitopsenginecluster_id {
			return nil, nil, fmt.Errorf("able to locate engine instance, and engine cluster, but they mismatched: instance id: %v, cluster id: %v",
				gitopsEngineInstance.Gitopsengineinstance_id, gitopsEngineCluster.Gitopsenginecluster_id)
		}

		return gitopsEngineInstance, gitopsEngineCluster, nil

	} else {
		return nil, nil, fmt.Errorf("unexpected state in UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID")
	}

}

// UncheckedGetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID gets (or creates it if it doesn't exist) a GitOpsEngineCluster
// database entry that corresponds to an GitOps engine cluster.
//
// In order to determine which cluster we are on, we use the UID of the 'kube-system' namespace.
func UncheckedGetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx context.Context, kubesystemNamespaceUID string, dbq db.DatabaseQueries, log logr.Logger) (*db.GitopsEngineCluster, error) {

	// Can we create the cluster credentials here?
	// - either we add cluster credentials to our k8sToDBMapping
	// - or we just assume that cluster credentials don't exist if the k8sToDBMapping doesn't exist.

	var gitopsEngineCluster *db.GitopsEngineCluster

	expectedDBResourceMapping := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: db.K8sToDBMapping_Namespace,
		KubernetesResourceUID:  kubesystemNamespaceUID,
		DBRelationType:         db.K8sToDBMapping_GitopsEngineCluster,
	}

	var dbResourceMapping *db.KubernetesToDBResourceMapping
	{
		var expectedDBResourceMappingCopy db.KubernetesToDBResourceMapping = expectedDBResourceMapping
		dbResourceMapping = &expectedDBResourceMappingCopy
	}

	if err := dbq.GetDBResourceMappingForKubernetesResource(ctx, dbResourceMapping); err != nil {

		if !db.IsResultNotFoundError(err) {
			return nil, fmt.Errorf("unable to get DBResourceMapping: %v", err)
		}

		dbResourceMapping = nil

	} else {
		// If there exists a db resource mapping for this cluster, see if we can get the GitOpsEngineCluster
		gitopsEngineCluster = &db.GitopsEngineCluster{
			Gitopsenginecluster_id: dbResourceMapping.DBRelationKey,
		}

		if err := dbq.UncheckedGetGitopsEngineClusterById(ctx, gitopsEngineCluster); err != nil {
			if !db.IsResultNotFoundError(err) {
				return nil, err
			}

			log.V(LogLevel_Warn).Error(nil, "UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID found a resource mapping, but no engine instance.")

			// We have found a mapping without the corresponding mapped entity, so delete the mapping.
			// (We will recreate the mapping below)
			if _, err := dbq.UncheckedDeleteKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
				return nil, err
			}

			gitopsEngineCluster = nil
			dbResourceMapping = nil
		} else {
			// Success: both existed.
			return gitopsEngineCluster, nil
		}
	}

	if dbResourceMapping == nil && gitopsEngineCluster == nil {
		// Scenario A) neither exists: create both

		// Create cluster credentials for the managed env
		// TODO: GITOPS-1564 - Cluster credentials placeholder values - we will need to create a service account on the target cluster, which we can store in the database.
		// See https://issues.redhat.com/browse/GITOPS-1564
		clusterCreds := db.ClusterCredentials{
			Host:                        "host",
			Kube_config:                 "kube_config",
			Kube_config_context:         "kube_config_context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "serviceaccount_ns",
		}
		if err := dbq.CreateClusterCredentials(ctx, &clusterCreds); err != nil {
			return nil, fmt.Errorf("unable to create cluster creds for managed env: %v", err)
		}

		gitopsEngineCluster = &db.GitopsEngineCluster{
			Clustercredentials_id: clusterCreds.Clustercredentials_cred_id,
		}

		if err := dbq.CreateGitopsEngineCluster(ctx, gitopsEngineCluster); err != nil {
			return nil, fmt.Errorf("unable to create engine cluster, when neither existed: %v", err)
		}

		expectedDBResourceMapping.DBRelationKey = gitopsEngineCluster.Gitopsenginecluster_id
		if err := dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &expectedDBResourceMapping); err != nil {
			return nil, fmt.Errorf("unable to create mapping when neither existed: %v", err)
		}

		return gitopsEngineCluster, nil

	} else if dbResourceMapping != nil && gitopsEngineCluster == nil {
		// Scenario B) This shouldn't happen: the above logic should ensure that dbResourceMapping is always nil, if gitopsEngineCluster is nil

		err := fmt.Errorf("SEVERE: the dbResourceMapping existed, but the gitops engine cluster did not")

		log.Error(err, err.Error())

		return nil, err

	} else if dbResourceMapping == nil && gitopsEngineCluster != nil {
		// Scenario C) this will happen if the engine cluster db entry exists, but there is no mapping for it

		expectedDBResourceMapping.DBRelationKey = gitopsEngineCluster.Gitopsenginecluster_id
		if err := dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &expectedDBResourceMapping); err != nil {
			return nil, fmt.Errorf("unable to create mapping when dbResourceMapping didn't exist: %v", err)
		}

		return gitopsEngineCluster, nil

	} else if dbResourceMapping != nil && gitopsEngineCluster != nil {
		// Scenario D) both exist, so just return the cluster
		return gitopsEngineCluster, nil
	}

	return nil, fmt.Errorf("unexpected return")

}

func UncheckedGetOrCreateDeploymentToApplicationMapping(ctx context.Context, createDeplToAppMapping *db.DeploymentToApplicationMapping, dbq db.ApplicationScopedQueries, actionLog logr.Logger) error {

	if err := dbq.UncheckedGetDeploymentToApplicationMappingByDeplId(ctx, createDeplToAppMapping); err != nil {
		if !db.IsResultNotFoundError(err) {
			logErr := fmt.Errorf("unable to get obj in UncheckedGetOrDeploymentToApplicationMapping: %v", err)
			actionLog.Error(logErr, "unable to get deplToApp mapping", "createDeplToAppMapping", createDeplToAppMapping)
			return logErr
		}
		// Object not found, so continue.
	} else {
		// Object was found
		return nil
	}

	// Ensure that there isn't an old depltoappmapping hanging around with matching name/namespace, before we create a new one.
	if _, err := dbq.UncheckedDeleteDeploymentToApplicationMappingByNamespaceAndName(ctx,
		createDeplToAppMapping.DeploymentName,
		createDeplToAppMapping.DeploymentNamespace,
		createDeplToAppMapping.WorkspaceUID); err != nil {
		actionLog.Error(err, "unable to delete old deployment to application mapping for name '"+
			createDeplToAppMapping.DeploymentName+"', namespace '"+createDeplToAppMapping.DeploymentNamespace+"'")
		return err
	}

	if err := dbq.CreateDeploymentToApplicationMapping(ctx, createDeplToAppMapping); err != nil {
		actionLog.Error(err, "unable to create deplToApp mapping", "createDeplToAppMapping", createDeplToAppMapping)
		return err
	}

	return nil
}

// CatchPanic calls f(), and recovers from panic if one occurs.
func CatchPanic(f func() error) (isPanic bool, err error) {

	panicLog := log.FromContext(context.Background())

	isPanic = false

	doRecover := func() {
		recoverRes := recover()

		if recoverRes != nil {
			err = fmt.Errorf("panic: %v", recoverRes)
			panicLog.Error(err, "SEVERE: Panic occurred")
			isPanic = true
		}

	}

	defer doRecover()

	err = f()

	return isPanic, err
}

// DisposeResources delets of a list of database entries in reverse order.
func DisposeResources(ctx context.Context, resources []db.DisposableResource, dbq db.DatabaseQueries, log logr.Logger) {

	if len(resources) == 0 {
		return
	}

	var err error

	for idx := len(resources) - 1; idx >= 0; idx-- {

		resource := resources[idx]
		if resource == nil {
			continue
		}

		log.V(LogLevel_Debug).Info(fmt.Sprintf("disposing of resource: %v", resource))

		disposeErr := resource.Dispose(ctx, dbq)
		if disposeErr != nil {
			if err == nil {
				err = disposeErr
			} else {
				// append the error to the existing error
				err = fmt.Errorf("error: %v. error: %v", disposeErr, err)
			}
		}
	}

	if err != nil {
		log.Error(err, "unable to delete old resources after operation")
	}
}

// DisposeApplicationScopedResources delets of a list of database entries in reverse order.
func DisposeApplicationScopedResources(ctx context.Context, resources []db.AppScopedDisposableResource, dbq db.ApplicationScopedQueries, log logr.Logger) {

	if len(resources) == 0 {
		return
	}

	var err error

	for idx := len(resources) - 1; idx >= 0; idx-- {

		resource := resources[idx]
		if resource == nil {
			continue
		}

		log.V(LogLevel_Debug).Info(fmt.Sprintf("disposing of resource: %v", resource))

		disposeErr := resource.DisposeAppScoped(ctx, dbq)
		if disposeErr != nil {
			if err == nil {
				err = disposeErr
			} else {
				// append the error to the existing error
				err = fmt.Errorf("error: %v. error: %v", disposeErr, err)
			}
		}
	}

	if err != nil {
		log.Error(err, "unable to delete old resources after operation")
	}
}
