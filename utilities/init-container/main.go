package main

import (
	"context"
	"fmt"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/utilities/init-container/hotfix"
)

func main() {

	fmt.Println("* Running the init-container")

	// February 8th, 2023 - Fix an issue with incorrect GitOpsEngineInstance on Stonesoup staging cluster
	// - Jonathan West

	// The target DB entry to update
	targetKDB := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		DBRelationType:         "GitopsEngineInstance",
		DBRelationKey:          "701632d3-dd8b-4173-88c3-84306579a156",
	}

	// Old and new values for the K8sToDBResourceMapping
	const (
		oldK8sResourceUID = "9dc321a8-aab6-4482-ac71-3c16be1e7c47"
		newK8ResourceUID  = "6c91e02f-b6d6-40cf-aa57-7478694f660b"
	)

	// We shouldn't panic: this will return a non-zero error code, and prevent the controller from starting.
	_, _ = util.CatchPanic(func() error {
		err := hotfix.HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping(context.Background(), targetKDB, oldK8sResourceUID,
			newK8ResourceUID)
		if err != nil {
			fmt.Println("error return by hotfix function:", err)
		}
		return nil
	})

}
