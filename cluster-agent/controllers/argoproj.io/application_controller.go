/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package argoprojio

import (
	"context"
	"fmt"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLog := log.FromContext(ctx)

	app := appv1.Application{}

	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		contextLog.Error(err, "Unable to find Application: "+app.Name)
		return ctrl.Result{}, err
	}

	contextLog.Info("Application event seen in reconciler: " + app.Name)

	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.Application{}).
		Complete(r)
}

// getUniqueClusterID returns a unique UID that can be used to identify one individual cluster instance from another.
//
// Since I'm not aware of an exist K8s API that gives us a value like this, I am using the uid of kube-system, which is
// unlikely to change under normal circumstances.
func getUniqueClusterID(ctx context.Context, clientClient client.Client) (string, error) {

	namespace := corev1.Namespace{}

	if err := clientClient.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "kube-system"}, &namespace); err != nil {
		return "", err
	}

	return string(namespace.UID), nil
}

func getGitOpsEngineInstanceForThisCluster(ctx context.Context, gitopsEngineInstanceParam *db.GitopsEngineInstance, gitopsEngineNamespace corev1.Namespace, clientClient client.Client, ownerId string) error {

	// create cluster user (does it seems strange that an argo cd instances needs a cluster user?)
	// create cluster credentials
	// create gitops engine cluster
	// create gitops engine instance (this is fake data)

	if gitopsEngineInstanceParam == nil {
		return fmt.Errorf("nil pointer param")
	}

	dbQueries, err := db.NewProductionPostgresDBQueries(true)
	if err != nil {
		return err
	}

	// Get the ID that uniquely identifies the cluster we are running on
	uniqueClusterID, err := getUniqueClusterID(ctx, clientClient)
	if err != nil {
		return err
	}

	// Get the GitOpsEngineCluster that corresponds to this namespace
	kdb := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		KubernetesResourceUID:  uniqueClusterID,
		DBRelationType:         "GitOpsEngineCluster",
	}
	// TODO: Whose job is it to create this?
	if err := dbQueries.GetDBResourceMappingForKubernetesResource(ctx, &kdb); err != nil {
		return err
	}

	gitopsEngineCluster := db.GitopsEngineCluster{
		Gitopsenginecluster_id: kdb.DBRelationKey,
	}
	if err := dbQueries.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster, ownerId); err != nil {
		return err
	}

	var gitopsEngineInstances []db.GitopsEngineInstance
	if err := dbQueries.ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, gitopsEngineCluster.Gitopsenginecluster_id, ownerId, &gitopsEngineInstances); err != nil {
		return err
	}

	if len(gitopsEngineInstances) == 0 {
		return fmt.Errorf("no GitOpsEngineInstances were found for %v, owner of %v", gitopsEngineCluster.Gitopsenginecluster_id, ownerId)
	}

	for idx, gitopsEngineInstance := range gitopsEngineInstances {
		if gitopsEngineInstance.Namespace_name == gitopsEngineNamespace.Name &&
			gitopsEngineInstance.Namespace_uid == string(gitopsEngineNamespace.UID) {
			// Match found: return it
			*gitopsEngineInstanceParam = gitopsEngineInstances[idx]
			return nil
		}
	}

	return fmt.Errorf("GitOpsEngineInstances were found, but no matches for namespace %v, for %v, owner of %v",
		gitopsEngineNamespace.UID, gitopsEngineCluster.Gitopsenginecluster_id, ownerId)
}
