package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateNewArgoCDInstance(ctx context.Context, namespace *corev1.Namespace, user db.ClusterUser, operationid string, k8sclient client.Client, log logr.Logger, dbQueries db.AllDatabaseQueries) error {

	if err := k8sclient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		log.Error(err, "Unable to get namespace")
		return fmt.Errorf("unable to retrieve gitopsengine namespace: %v", err)
	}
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -1")

	kubeSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	if err := k8sclient.Get(ctx, client.ObjectKeyFromObject(kubeSystemNamespace), kubeSystemNamespace); err != nil {
		log.Error(err, "Unable to get kubesystem namespace")
		return fmt.Errorf("unable to retrieve kubesystem namespace %v", err)
	}

	gitopsEngineInstance, _, _, err := dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, string(kubeSystemNamespace.UID), dbQueries, log)
	if err != nil {
		return err
	}
	fmt.Println(gitopsEngineInstance.Gitopsengineinstance_id, gitopsEngineInstance.Namespace_name, gitopsEngineInstance.Namespace_uid, gitopsEngineInstance.EngineCluster_id)
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -2")

	dboperation := db.Operation{
		Operation_id:            operationid,
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Operation_owner_user_id: user.Clusteruser_id,
		Resource_type:           db.OperationResourceType_GitOpsEngineInstance,
		Resource_id:             gitopsEngineInstance.Gitopsengineinstance_id,
	}

	log.Info("Creating operation row for the gitopsEngineInstance")

	err = dbQueries.CreateOperation(ctx, &dboperation, dboperation.Operation_owner_user_id)
	if err != nil {
		log.Error(err, "Unable to create db row for Operation")
		return fmt.Errorf("unable to create operation db row for gitopsEngineInstance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
	}
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -3")

	log.Info("Creating operation CR for the gitopsEngineInstance")

	// k8sOperation, dbOperation, err := operations.CreateOperation(ctx, false, operation, user.Clusteruser_id, gitopsEngineInstance.Namespace_name, dbQueries, k8sclient, log)
	// if err != nil {
	// 	return fmt.Errorf("unable to create operation for gitopsEngineInstance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
	// } else if k8sOperation == nil {
	// 	return fmt.Errorf("k8soperation returned nil for gitopsEngineInstance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
	// } else if dbOperation == nil {
	// 	return fmt.Errorf("dboperation returned nil for gitopsEngineInstance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
	// }
	k8sOperation := managedgitopsv1alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operations.GenerateOperationCRName(dboperation),
			Namespace: gitopsEngineInstance.Namespace_name,
		},
		Spec: managedgitopsv1alpha1.OperationSpec{
			OperationID: dboperation.Operation_id,
		},
	}

	if err := k8sclient.Create(ctx, &k8sOperation, &client.CreateOptions{}); err != nil {
		log.Error(err, "Unable to create K8s Operation")
		return fmt.Errorf("unable to create operation for gitopsEngineInstance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
	}
	fmt.Println("BBBBBAAACCCKKKEENNNDDD -4")

	return nil
}
