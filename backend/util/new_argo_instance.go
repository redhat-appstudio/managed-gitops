package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateNewArgoCDInstance(namespace *corev1.Namespace, user db.ClusterUser, k8sclient client.Client, log logr.Logger, dbQueries db.AllDatabaseQueries) error {
	ctx := context.Background()

	if err := k8sclient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return fmt.Errorf("unable to retrieve gitopsengine namespace: %v", err)
	}
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -1")

	kubeSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}

	gitopsEngineInstance, _, _, err := dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, kubeSystemNamespace.Name, dbQueries, log)
	if err != nil {
		return err
	}
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -2")

	operation := db.Operation{
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Operation_owner_user_id: user.Clusteruser_id,
		Resource_type:           db.OperationResourceType_GitOpsEngineInstance,
		Resource_id:             gitopsEngineInstance.Gitopsengineinstance_id,
	}

	log.Info("Creating operation for the gitopsEngineInstance")

	_, _, err = operations.CreateOperation(ctx, false, operation, user.Clusteruser_id,
		dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, k8sclient, log)
	if err != nil {
		return fmt.Errorf("unable to create operation for gitopsEngineInstance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
	}
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -3")

	return nil
}
