package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	v1 "k8s.io/api/core/v1"
)

// TODO: GITOPS-1564 - Add tests for all scenarios, for each of these.
// See https://issues.redhat.com/browse/GITOPS-1564

func UncheckedGetOrCreateManagedEnvironmentByNamespaceUID(ctx context.Context, namespace v1.Namespace, dbq db.DatabaseQueries, actionLog logr.Logger) (*db.ManagedEnvironment, error) {

	namespaceUID := string(namespace.UID)

	var err error

	dbResourceMapping := &db.KubernetesToDBResourceMapping{
		KubernetesResourceType: db.K8sToDBMapping_Namespace,
		KubernetesResourceUID:  string(namespaceUID),
		DBRelationType:         db.K8sToDBMapping_ManagedEnvironment}
	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, dbResourceMapping)

	if err == nil {
		// If there already exists a managed environment for this workspace, return it

		managedEnvironment := db.ManagedEnvironment{Managedenvironment_id: string(namespaceUID)}
		err = dbq.UncheckedGetManagedEnvironmentById(ctx, &managedEnvironment)

		if err == nil {
			// mapping exists, and so does the environment it points to
			return &managedEnvironment, nil
		}

		if !db.IsResultNotFoundError(err) {
			return nil, fmt.Errorf("unable to retrieve resource mapping: %v", err)
		}

		// Since the managed environment doesn't exist, delete the mapping; it will be recreated below.
		if _, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
			return nil, fmt.Errorf("unable to delete K8s resource to DB mapping: %v", dbResourceMapping)
		}

	} else if !db.IsResultNotFoundError(err) {
		return nil, fmt.Errorf("unable to retrieve resource mapping: %v", err)
	}

	// At this point in the function, both the managed environment and mapping doesn't exist

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
		Name:                  "managed env for " + namespace.Name, // TODO: Placeholder name
		Clustercredentials_id: clusterCreds.Clustercredentials_cred_id,
	}
	if err := dbq.CreateManagedEnvironment(ctx, &managedEnvironment); err != nil {
		return nil, fmt.Errorf("unable to create managed env: %v", err)
	}

	dbResourceMapping = &db.KubernetesToDBResourceMapping{
		KubernetesResourceType: db.K8sToDBMapping_Namespace,
		KubernetesResourceUID:  string(namespaceUID),
		DBRelationType:         db.K8sToDBMapping_GitopsEngineCluster,
		DBRelationKey:          managedEnvironment.Managedenvironment_id,
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
	clusterCredentials db.ClusterCredentials, dbq db.DatabaseQueries, log logr.Logger) (*db.GitopsEngineInstance, *db.GitopsEngineCluster, error) {

	// First create the GitOpsEngine cluster if needed; this will be used to create the instance.
	gitopsEngineCluster, err := UncheckedGetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, kubesystemNamespaceUID, clusterCredentials, dbq, log)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create GitOpsEnglineCluster for %s in %v", kubesystemNamespaceUID, err)
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
			if _, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
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
func UncheckedGetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx context.Context, kubesystemNamespaceUID string, clusterCredentials db.ClusterCredentials, dbq db.DatabaseQueries, log logr.Logger) (*db.GitopsEngineCluster, error) {

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
			if _, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, dbResourceMapping); err != nil {
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

		gitopsEngineCluster = &db.GitopsEngineCluster{
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
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
		// Scenario C) this will happen if the cluster exists, but there is no mapping for it

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
