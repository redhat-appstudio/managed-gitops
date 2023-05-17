package argoprojio

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

const (
	appRowBatchSize             = 50               // Number of rows needs to be fetched in each batch.
	namespaceReconcilerInterval = 30 * time.Minute // Interval in Minutes to reconcile workspace/namespace.
	sleepIntervalsOfBatches     = 1 * time.Second  // Interval in Millisecond between each batch.
	SecretDbIdentifierKey       = "databaseID"     //Secret label key to point DB entry.
)

// This function iterates through each Workspace/Namespace present in DB and ensures that the state of resources in Cluster is in Sync with DB.
func (r *ApplicationReconciler) StartNamespaceReconciler() {
	r.startTimerForNextCycle()
}

func (r *ApplicationReconciler) startTimerForNextCycle() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(namespaceReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).
			WithName(logutil.LogLogger_managed_gitops)

		_, _ = sharedutil.CatchPanic(func() error {
			runNamespaceReconcile(ctx, r.DB, r.Client, log)
			runSecretCleanup(ctx, r.DB, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'namespaceReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle()
	}()

}

func runNamespaceReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {

	// Fetch list of ArgoCD applications to be used later
	// map: applications IDs seen (string) -> (map value not used)
	processedApplicationIds := make(map[string]any)

	argoApplicationList := appv1.ApplicationList{}
	if err := client.List(ctx, &argoApplicationList); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching list of ArgoCD applications.")
	}
	argoApplications := argoApplicationList.Items

	offSet := 0

	// Delete operation resources created during previous run.
	cleanK8sOperations(ctx, dbQueries, client, log)

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching clusterUser.")
		return
	}

	log.Info("Triggered Namespace Reconciler to keep Argo application in sync with DB.")

	// Continuously iterate and fetch batches until all entries of Application table are processed.
	for {

		if offSet != 0 {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApplicationsFromDB []db.Application

		// Fetch Application table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &listOfApplicationsFromDB, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in Namespace Reconciler while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfApplicationsFromDB) == 0 {
			log.Info("All Application entries are processed by Namespace Reconciler.")
			break
		}

		// Iterate over batch received above.
		for _, applicationRowFromDB := range listOfApplicationsFromDB {
			var applicationFromDB fauxargocd.FauxApplication

			processedApplicationIds[applicationRowFromDB.Application_id] = false

			// Fetch the Application object from DB
			if err := yaml.Unmarshal([]byte(applicationRowFromDB.Spec_field), &applicationFromDB); err != nil {
				log.Error(err, "Error occurred in Namespace Reconciler while unmarshalling application: "+applicationRowFromDB.Application_id)
				continue // Skip to next iteration instead of stopping the entire loop.
			}

			// Fetch the Application object from k8s
			applicationFromArgoCD := appv1.Application{}
			namespacedName := types.NamespacedName{
				Name:      applicationFromDB.Name,
				Namespace: applicationFromDB.Namespace}

			if err := client.Get(ctx, namespacedName, &applicationFromArgoCD); err != nil {
				if apierr.IsNotFound(err) {

					if applicationRowFromDB.Managed_environment_id == "" {
						// We shouldn't recreate any Argo CD Application resources that have a nil managed environment row
						// - A nil managed environment row means the Application is currently invalid (until the user fixes it, by updating the GitOpsDeployment)

						// So we just continue to the next Application
						continue
					}

					log.Info("Application " + applicationRowFromDB.Application_id + " not found in ArgoCD, probably user deleted it, " +
						"but it still exists in DB, hence recreating application in ArgoCD.")

					// We need to recreate ArgoCD Application, to do that create Operation to inform ArgoCD about it.
					dbOperationInput := db.Operation{
						Instance_id:   applicationRowFromDB.Engine_instance_inst_id,
						Resource_id:   applicationRowFromDB.Application_id,
						Resource_type: db.OperationResourceType_Application,
					}
					engineInstanceDB := db.GitopsEngineInstance{
						Gitopsengineinstance_id: dbOperationInput.Instance_id,
					}
					if err = dbQueries.GetGitopsEngineInstanceById(ctx, &engineInstanceDB); err != nil {
						log.Error(err, "Unable to fetch GitopsEngineInstance")
						continue
					}
					_, _, err = operations.CreateOperation(ctx, false, dbOperationInput,
						specialClusterUser.Clusteruser_id, engineInstanceDB.Namespace_name, dbQueries, client, log)
					if err != nil {
						log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
					}
					log.Info("Operation is created to recreateArgoCD  Application " + applicationRowFromDB.Application_id)
				} else {
					log.Error(err, "Error occurred in Namespace Reconciler while fetching application from cluster: "+applicationRowFromDB.Application_id)
				}
				continue
			}

			// If the managed_environment_id row is not empty, then it depends on whether the DB spec field matches the
			if applicationRowFromDB.Managed_environment_id != "" {

				// At this point we have the applications from ArgoCD and DB, now compare them to check if they are not in Sync.

				if compare, err := controllers.CompareApplication(applicationFromArgoCD, applicationRowFromDB, log); err != nil {
					log.Error(err, "unable to compare application contents")
					continue
				} else if compare != "" {
					log.Info("Argo application is not in Sync with DB, updating Argo CD App. Application:" + applicationRowFromDB.Application_id)
				} else {
					log.V(logutil.LogLevel_Debug).Info("Argo application is in Sync with DB, Application:" + applicationRowFromDB.Application_id)
					continue
				}
			}

			// else { if the managed_enviroment_id row is empty, then continue executing below as we should always create an Operation in this case }

			// At this point application from ArgoCD and DB are not in Sync (or the managed env is empty),
			// so need to update Argo CD Application resource according to DB entry.

			// ArgoCD application and DB entry are not in Sync,
			// ArgoCD should use the state of resources present in the database should
			// Create Operation to inform Argo CD to get in Sync with database entry.
			dbOperationInput := db.Operation{
				Instance_id:   applicationRowFromDB.Engine_instance_inst_id,
				Resource_id:   applicationRowFromDB.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}
			engineInstanceDB := db.GitopsEngineInstance{
				Gitopsengineinstance_id: dbOperationInput.Instance_id,
			}
			if err := dbQueries.GetGitopsEngineInstanceById(ctx, &engineInstanceDB); err != nil {
				log.Error(err, "Unable to fetch GitopsEngineInstance")
				continue
			}
			if _, _, err := operations.CreateOperation(ctx, false, dbOperationInput,
				specialClusterUser.Clusteruser_id, engineInstanceDB.Namespace_name, dbQueries, client, log); err != nil {
				log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
				continue
			}

			log.Info("Namespace Reconcile processed application: " + applicationRowFromDB.Application_id)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}

	// Start a goroutine, because DeleteArgoCDApplication() function from cluster-agent/controllers may take some time to delete application.
	go deleteOrphanedApplications(argoApplications, processedApplicationIds, ctx, client, log)

	log.Info(fmt.Sprintf("Namespace Reconciler finished an iteration at %s. "+
		"Next iteration will be triggered after %v Minutes", time.Now().String(), namespaceReconcilerInterval))
}

// runSecretCleanup goes through the Argo CD Cluster/Repository Secrets, and deletes secrets that no longer point to valid database entries.
func runSecretCleanup(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) {

	kubesystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubesystemNamespace), kubesystemNamespace); err != nil {
		log.Error(err, "Error occurred in retrieving kube-system namespace, while running secret cleanup")
		return
	}

	var err error
	var gitopsEngineCluster *db.GitopsEngineCluster
	if gitopsEngineCluster, err = dbutil.GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, log); err != nil {
		log.Error(err, "Unable to retrieve GitopsEngineClusterFromDatabase, while running secret cleanup")
		return
	} else if gitopsEngineCluster == nil {
		log.Info("Skipping secret cleanup, as the GitOpsEngineCluster does not yet exist for this cluster.")
		return
	}

	// map: whether we have processed this namespace already.
	// key is namespace UID, value is not used.
	// - we should only ever process a namespace once per iteration.
	namespacesProcessed := map[string]interface{}{}

	// Get list of instances from GitopsEngineInstance table.
	var gitopsEngineInstance []db.GitopsEngineInstance

	if err := dbQueries.ListGitopsEngineInstancesForCluster(ctx, *gitopsEngineCluster, &gitopsEngineInstance); err != nil {
		log.Error(err, "Error occurred in Secret Clean-up while fetching list of GitopsEngineInstances.")
		return
	}

	for instanceIndex := range gitopsEngineInstance {
		// For each GitOpsEngine instance, read the namespace and check all the Argo CD secrets in that namespace

		instance := gitopsEngineInstance[instanceIndex] // To avoid "Implicit memory aliasing in for loop." error.

		// Sanity check: have we processed this Namespace already
		if _, exists := namespacesProcessed[instance.Namespace_uid]; exists {
			// Log it an skip. There really should never be any GitOpsEngineInstances with matching namespace UID
			log.V(logutil.LogLevel_Warn).Error(nil, "there appears to exist multiple GitOpsEngineInstances targeting the same namespace", "namespaceUID", instance.Namespace_uid)
			continue
		}
		namespacesProcessed[instance.Namespace_uid] = nil

		// Get list of Secrets in the given namespace
		secretList := corev1.SecretList{}
		if err := k8sClient.List(context.Background(), &secretList, &client.ListOptions{Namespace: instance.Namespace_name}); err != nil {
			log.Error(err, "Error occurred in Secret Clean-up while fetching list of Secrets from Namespace : "+instance.Namespace_name)
			return
		}

		for secretIndex := range secretList.Items {
			secret := secretList.Items[secretIndex] // To avoid "Implicit memory aliasing in for loop." error.

			// processSecret looks for orphaned Argo CD Cluster/Repo secrets, and if orphaned, deletes the cluster secret
			processSecret(ctx, secret, dbQueries, k8sClient, log)
		}
	}
}

// processSecret looks for orphaned Argo CD Cluster/Repo secrets, and if orphaned, deletes the cluster secret
func processSecret(ctx context.Context, secret corev1.Secret, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) {

	// Look for secrets which have required labels i.e databaseID and argocd.argoproj.io/secret-type
	// Ignore secrets which don't have these labels
	var secretType string
	databaseID, ok := secret.Labels[SecretDbIdentifierKey]
	if !ok {
		// Secret does not have 'databaseID' label, so no work to do.
		return
	}

	secretType, ok = secret.Labels[sharedutil.ArgoCDSecretTypeIdentifierKey]
	if !ok {
		// Secret does not have the label that Argo CD uses to identify what type of Secret it is, so just return
		return
	}

	// If secret has both labels then process it.
	var err error

	if secretType == sharedutil.ArgoCDSecretClusterTypeValue {
		// If secret type is Cluster, then check if DB entry pointed by databaseID label is present in ManagedEnvironment table.
		err = dbQueries.GetManagedEnvironmentById(ctx, &db.ManagedEnvironment{Managedenvironment_id: databaseID})

	} else if secretType == sharedutil.ArgoCDSecretRepoTypeValue {
		// If secret type is Repository, then check if DB entry pointed by databaseID label is present in RepositoryCredentials table.
		_, err = dbQueries.GetRepositoryCredentialsByID(ctx, databaseID)
	}

	if err == nil {
		// No error was returned by retrieving the database entry, therefore it exists.
		// Thus, no work to do.
		return
	}

	if db.IsResultNotFoundError(err) {
		// If entry is not present in DB then it is an orphan CR, hence delete the Secret from Cluster.
		k8sErr := k8sClient.Delete(ctx, &secret) // k8sErr is declared above To avoid "Implicit memory aliasing in for loop." error.
		if k8sErr != nil {
			log.Error(k8sErr, "error occurred in Secret Clean-up while deleting an orphan Secret: "+secret.Name, "namespace", secret.Namespace)
		} else {
			log.Info("Secret Clean-up successfully deleted orphan Secret: "+secret.Name, "namespace", secret.Namespace)
		}

	} else {
		// Some other unexpected error occurred, so we just skip it until next time
		log.Error(err, "unexpected error occurred in Secret Clean-up while fetching DB entry pointed by Secret : "+secret.Name, "namespace", secret.Namespace)
	}
}

func cleanK8sOperations(ctx context.Context, dbq db.DatabaseQueries, client client.Client, log logr.Logger) {
	// Get list of Operations from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	err := client.List(ctx, &listOfK8sOperation)
	if err != nil {
		log.Error(err, "Unable to fetch list of k8s Operation from cluster.")
		return
	}

	for _, k8sOperation := range listOfK8sOperation.Items {

		// Skip if Operation was not created by Namespace Reconciler.
		if k8sOperation.Annotations[operations.IdentifierKey] != operations.IdentifierValue {
			continue
		}

		// Fetch corresponding DB entry
		dbOperation := db.Operation{
			Operation_id: k8sOperation.Spec.OperationID,
		}
		if err := dbq.GetOperationById(ctx, &dbOperation); err != nil {
			continue
		}

		if dbOperation.State != db.OperationState_Completed && dbOperation.State != db.OperationState_Failed {
			log.V(logutil.LogLevel_Debug).Info("K8s Operation is not ready for cleanup : " + string(k8sOperation.UID) + " DbOperation: " + string(k8sOperation.Spec.OperationID))
			continue
		}

		log.Info("Deleting Operation created by Namespace Reconciler." + string(k8sOperation.UID))
		engineInstanceDB := db.GitopsEngineInstance{
			Gitopsengineinstance_id: dbOperation.Instance_id,
		}
		if err = dbq.GetGitopsEngineInstanceById(ctx, &engineInstanceDB); err != nil {
			log.Error(err, "Unable to fetch GitopsEngineInstance")
			continue
		}
		// Delete the k8s operation now.
		if err := operations.CleanupOperation(ctx, dbOperation, k8sOperation, engineInstanceDB.Namespace_name,
			dbq, client, false, log); err != nil {

			log.Error(err, "Unable to Delete k8s Operation"+string(k8sOperation.UID)+" for DbOperation: "+string(k8sOperation.Spec.OperationID))
		} else {
			log.Info("Deleted k8s Operation: " + string(k8sOperation.UID) + " for DbOperation: " + string(k8sOperation.Spec.OperationID))
		}
	}
	log.V(logutil.LogLevel_Debug).Info("Cleaned all Operations created by Namespace Reconciler.")
}

func deleteOrphanedApplications(argoApplications []appv1.Application, processedApplicationIds map[string]any,
	ctx context.Context, client client.Client, log logr.Logger) []appv1.Application {

	if len(argoApplications) == 0 {
		return []appv1.Application{}
	}

	shuffledList := argoApplications

	// Shuffle the list of Argo Applications, so that we are not always deleting in the same order.
	// - This is beneficial when we have a long list of Applications to delete, that take longer than namespaceReconcilerInterval.
	rand.Shuffle(len(shuffledList), func(i, j int) {
		shuffledList[i], shuffledList[j] = shuffledList[j], shuffledList[i]
	})

	// Iterate through all Argo CD applications and delete applications which are not having entry in DB.
	var deletedOrphanedApplications []appv1.Application
	for _, application := range shuffledList {

		// Skip Applications not created by the GitOps Service
		if value, exists := application.Labels[controllers.ArgoCDApplicationDatabaseIDLabel]; !exists || value == "" {
			continue
		}

		if _, ok := processedApplicationIds[application.Labels["databaseID"]]; !ok {
			if err := controllers.DeleteArgoCDApplication(ctx, application, client, log); err != nil {
				log.Error(err, "unable to delete an orphaned Argo CD Application "+application.Name)
			} else {
				deletedOrphanedApplications = append(deletedOrphanedApplications, application)
				log.Info("Deleting orphaned Argo CD Application " + application.Name)
			}
		}
	}
	return deletedOrphanedApplications
}
