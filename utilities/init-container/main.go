package main

import (
	"context"
	"fmt"
	"os"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/utilities/init-container/hotfix"
)

func main() {

	fmt.Println("* Running the init-container")

	// March 6th, 2023 - Fix an issue with incorrect GitOpsEngineInstance on Stonesoup prod member clusters
	// - Jonathan West

	patchMultitenantCluster()
	patchRHTenantCluster()

	os.Exit(0)
}

func patchMultitenantCluster() {

	// KubernetesToDBResourceMapping entry:
	// Pre:

	//  kubernetes_resource_type |       kubernetes_resource_uid        |   db_relation_type   |           db_relation_key            | seq_id
	// --------------------------+--------------------------------------+----------------------+--------------------------------------+--------
	//  Namespace                | 7b907c12-81ad-4661-bff7-c3030b26111b | GitopsEngineInstance | 4c1470c7-fefd-4911-8bc1-0cf5e2298411 |     28

	// Post:

	//  kubernetes_resource_type |       kubernetes_resource_uid        |   db_relation_type   |           db_relation_key            | seq_id
	// --------------------------+--------------------------------------+----------------------+--------------------------------------+--------
	//  Namespace                | 7dcba8c2-88d3-4960-865a-9b0eb691cba5 | GitopsEngineInstance | 4c1470c7-fefd-4911-8bc1-0cf5e2298411 |     28

	// The target DB entry to update
	targetKDB := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		DBRelationType:         "GitopsEngineInstance",
		DBRelationKey:          "4c1470c7-fefd-4911-8bc1-0cf5e2298411",
	}

	// Old and new values for the K8sToDBResourceMapping
	const (
		oldK8sResourceUID = "7b907c12-81ad-4661-bff7-c3030b26111b"
		newK8ResourceUID  = "7dcba8c2-88d3-4960-865a-9b0eb691cba5"
	)

	// We shouldn't panic: this will return a non-zero error code, and prevent the controller from starting.
	_, outerErr := util.CatchPanic(func() error {
		err := hotfix.HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping(context.Background(), targetKDB, oldK8sResourceUID,
			newK8ResourceUID)
		return err
	})

	if outerErr != nil {
		fmt.Println("Error return by hotfix function:", outerErr)
	}

	if outerErr != nil {
		os.Exit(1)
	}

}

func patchRHTenantCluster() {

	// KubernetesToDBResourceMapping entry:

	// Pre:
	//
	// kubernetes_resource_type |       kubernetes_resource_uid        |   db_relation_type   |           db_relation_key            | seq_id
	// --------------------------+--------------------------------------+----------------------+--------------------------------------+--------
	// Namespace                | f5820fe1-7347-41c6-9a6a-1b62b6c5567b | GitopsEngineInstance | 8fb948f8-5602-46f5-9656-804661c7229c |      3

	//
	// Post:
	// Namespace                | cc240bc3-3a91-400c-8f42-b46af32697f6 | GitopsEngineInstance | 8fb948f8-5602-46f5-9656-804661c7229c |      3

	// The target DB entry to update
	targetKDB := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		DBRelationType:         "GitopsEngineInstance",
		DBRelationKey:          "8fb948f8-5602-46f5-9656-804661c7229c",
	}

	// Old and new values for the K8sToDBResourceMapping
	const (
		oldK8sResourceUID = "f5820fe1-7347-41c6-9a6a-1b62b6c5567b"
		newK8ResourceUID  = "cc240bc3-3a91-400c-8f42-b46af32697f6"
	)

	// We shouldn't panic: this will return a non-zero error code, and prevent the controller from starting.
	_, outerErr := util.CatchPanic(func() error {
		err := hotfix.HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping(context.Background(), targetKDB, oldK8sResourceUID,
			newK8ResourceUID)
		return err
	})

	if outerErr != nil {
		fmt.Println("Error return by hotfix function:", outerErr)
	}

	if outerErr != nil {
		os.Exit(1)
	}
}
