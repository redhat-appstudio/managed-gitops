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
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
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
	appRowBatchSize                    = 50               // Number of rows needs to be fetched in each batch.
	defaultNamespaceReconcilerInterval = 30 * time.Minute // Interval in Minutes to reconcile workspace/namespace.
	sleepIntervalsOfBatches            = 1 * time.Second  // Interval in Millisecond between each batch.
	SecretDbIdentifierKey              = "databaseID"     //Secret label key to point DB entry.
	waitTimeforRowDelete               = 1 * time.Hour    // Number of hours to wait before deleting DB row
	waitTimeForK8sResourceDelete       = 1 * time.Hour    // Number of hours to wait before deleting k8s resource

)

// This function iterates through each Workspace/Namespace present in DB and ensures that the state of resources in Cluster is in Sync with DB.
func (r *ApplicationReconciler) StartNamespaceReconciler() {
	ctx := context.Background()

	log := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops).
		WithValues(logutil.Log_Component, logutil.Log_Component_ClusterAgent)

	namespaceReconcilerInterval := sharedutil.SelfHealInterval(defaultNamespaceReconcilerInterval, log)
	if namespaceReconcilerInterval > 0 {
		r.startTimerForNextCycle(ctx, namespaceReconcilerInterval, log)
		log.Info(fmt.Sprintf("Namespace reconciliation has been scheduled every %s", namespaceReconcilerInterval.String()))
	} else {
		log.Info("Namespace reconciliation has been disabled")
	}
}

func (r *ApplicationReconciler) startTimerForNextCycle(ctx context.Context, namespaceReconcilerInterval time.Duration, log logr.Logger) {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(namespaceReconcilerInterval)
		<-timer.C

		_, _ = sharedutil.CatchPanic(func() error {

			// Sync Argo CD Application with DB entry
			syncCRsWithDB_Applications(ctx, r.DB, r.Client, log)

			// Clean orphaned Secret CRs from Cluster.
			cleanOrphanedCRsfromCluster_Secret(ctx, r.DB, r.Client, log)

			// Clean orphaned and purposeless Operation CRs from Cluster.
			cleanOrphanedCRsfromCluster_Operation(ctx, r.DB, r.Client, log)

			// Recreate Secrets that are required by Applications, but missing from cluster.
			recreateClusterSecrets(ctx, r.DB, r.Client, log)

			log.Info(fmt.Sprintf("Namespace Reconciler finished an iteration at %s. "+
				"Next iteration will be triggered after %v Minutes", time.Now().String(), namespaceReconcilerInterval))

			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'namespaceReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle(ctx, namespaceReconcilerInterval, log)
	}()

}

func syncCRsWithDB_Applications(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, logger logr.Logger) {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Applications")

	// Fetch list of ArgoCD applications to be used later
	// map: applications IDs seen (string) -> (map value not used)
	processedApplicationIds := make(map[string]any)

	argoApplicationList := appv1.ApplicationList{}
	if err := client.List(ctx, &argoApplicationList); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching list of ArgoCD Aapplications")
		return
	}
	argoApplications := argoApplicationList.Items

	offSet := 0

	// Delete operation resources created during previous run.
	syncCRsWithDB_Applications_Delete_Operations(ctx, dbQueries, client, log)

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching ClusterUser")
		return
	}

	log.Info("Triggered Namespace Reconciler to keep Argo Application in sync with DB")

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

			log := log.WithValues(logutil.Log_ApplicationID, applicationRowFromDB.Application_id)

			processedApplicationIds[applicationRowFromDB.Application_id] = false

			// Fetch the Application object from DB
			if err := yaml.Unmarshal([]byte(applicationRowFromDB.Spec_field), &applicationFromDB); err != nil {
				log.Error(err, "Error occurred in Namespace Reconciler while unmarshalling application")
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

					log.Info("Application not found in ArgoCD, probably user deleted it, but it still exists in DB, hence recreating application in ArgoCD.")

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
					log.Info("Operation is created to recreateArgoCD")
				} else {
					log.Error(err, "Error occurred in Namespace Reconciler while fetching application from cluster")
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
					log.Info("Argo application is not in Sync with DB, updating Argo CD App")
				} else {
					log.V(logutil.LogLevel_Debug).Info("Argo application is in Sync with DB")
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

			if _, dbOp, err := operations.CreateOperation(ctx, false, dbOperationInput,
				specialClusterUser.Clusteruser_id, engineInstanceDB.Namespace_name, dbQueries, client, log); err != nil {
				log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
				continue
			} else if dbOp != nil {
				log.Info("Operation is created to sync Application row with Argo CD Application", "operationDBID", dbOp.Operation_id)

			}

		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}

	// Start a goroutine, because DeleteArgoCDApplication() function from cluster-agent/controllers may take some time to delete application.
	go cleanOrphanedCRsfromCluster_Applications(argoApplications, processedApplicationIds, ctx, client, log)
}

func syncCRsWithDB_Applications_Delete_Operations(ctx context.Context, dbq db.DatabaseQueries, client client.Client, log logr.Logger) {
	// Get list of Operations from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	if err := client.List(ctx, &listOfK8sOperation); err != nil {
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

		log := log.WithValues("dbOperation", k8sOperation.Spec.OperationID, "k8sOperationUID", string(k8sOperation.UID))

		if dbOperation.State != db.OperationState_Completed && dbOperation.State != db.OperationState_Failed {
			log.V(logutil.LogLevel_Debug).Info("K8s Operation is not ready for cleanup")
			continue
		}

		// Delete the k8s operation now.
		if err := operations.CleanupOperation(ctx, dbOperation, k8sOperation, dbq, client, false, log); err != nil {
			log.Error(err, "Unable to delete k8s Operation")
		} else {
			log.V(logutil.LogLevel_Debug).Info("Clean up job for CR deleting Operation created by Namespace Reconciler.")
		}
	}

	log.Info("Cleaned all Operations created by Namespace Reconciler.")
}

func cleanOrphanedCRsfromCluster_Applications(argoApplications []appv1.Application, processedApplicationIds map[string]any,
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

		log := log.WithValues("argoCDApplicationName", application.Name)

		// Skip Applications not created by the GitOps Service
		if value, exists := application.Labels[controllers.ArgoCDApplicationDatabaseIDLabel]; !exists || value == "" {
			continue
		}

		if _, ok := processedApplicationIds[application.Labels["databaseID"]]; !ok {
			if err := controllers.DeleteArgoCDApplication(ctx, application, client, log); err != nil {
				log.Error(err, "unable to delete an orphaned Argo CD Application")
			} else {
				deletedOrphanedApplications = append(deletedOrphanedApplications, application)
				log.Info("Deleting orphaned Argo CD Application")
			}
		}
	}
	return deletedOrphanedApplications
}

// cleanOrphanedCRsfromCluster_Secret goes through the Argo CD Cluster/Repository Secrets, and deletes secrets that no longer point to valid database entries.
func cleanOrphanedCRsfromCluster_Secret(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, logger logr.Logger) {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Secret")

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

			// cleanOrphanedCRsfromCluster_Secret_Delete looks for orphaned Argo CD Cluster/Repo secrets, and if orphaned, deletes the cluster secret
			cleanOrphanedCRsfromCluster_Secret_Delete(ctx, secret, dbQueries, k8sClient, log)
		}
	}
}

// cleanOrphanedCRsfromCluster_Secret_Delete looks for orphaned Argo CD Cluster/Repo secrets, and if orphaned, deletes the cluster secret
func cleanOrphanedCRsfromCluster_Secret_Delete(ctx context.Context, secret corev1.Secret, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) {

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

	log = log.WithValues("secretName", secret.Name, "secretNamespace", secret.Namespace, "secretType", secretType)

	if db.IsResultNotFoundError(err) {
		// If entry is not present in DB then it is an orphaned CR, hence we delete the Secret from cluster.
		if err := k8sClient.Delete(ctx, &secret); err != nil {
			log.Error(err, "error occurred in Secret Clean-up while deleting an orphan Secret")
		} else {
			log.Info("Clean up job deleted orphaned Secret CR")
		}

	} else {
		// Some other unexpected error occurred, so we just skip it until next time
		log.Error(err, "unexpected error occurred in Secret Clean-up while fetching DB entry pointed by Secret")
	}
}

// cleanOrphanedCRsfromCluster_Operation goes through the Operation CRs of cluster, and deletes CRs that are no longer point to valid database entries or already completed.
func cleanOrphanedCRsfromCluster_Operation(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, logger logr.Logger) {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Applications")

	// Get list of Operations from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	err := k8sClient.List(ctx, &listOfK8sOperation)
	if err != nil {
		// If there is an error then return and clean CRs in next iteration.
		log.Error(err, "unable to fetch list of Operation from cluster.")
		return
	}

	for _, k8sOperation := range listOfK8sOperation.Items {
		k8sOperation := k8sOperation // To avoid "Implicit memory aliasing in for loop." error.

		// Fetch corresponding DB entry.
		dbOperation := db.Operation{
			Operation_id: k8sOperation.Spec.OperationID,
		}

		deleteCr := false
		if err := dbQueries.GetOperationById(ctx, &dbOperation); err != nil {
			if db.IsResultNotFoundError(err) {
				// Delete the CR since it doesn't point to a DB entry, hence it is an orphaned CR.
				deleteCr = true
			} else {
				log.Error(err, fmt.Sprintf("error occurred in cleanOrphanedCRsfromCluster_Operation while fetching Operation: "+dbOperation.Operation_id+" from DB."))
			}
		} else {
			if dbOperation.State == db.OperationState_Completed &&
				time.Since(dbOperation.Created_on) > waitTimeForK8sResourceDelete {
				// Delete the CR since it is marked as "Completed" in DB entry, hence it is no longer in required.
				deleteCr = true
			}
		}

		if deleteCr {
			if err := k8sClient.Delete(ctx, &k8sOperation); err != nil {
				// If not able to delete then just log the error and leave it for next run.
				log.Error(err, "unable to delete orphaned Operation from cluster.", "OperationName", k8sOperation.Name)
			}
			log.Info("Managed-gitops clean up job for CR successfully deleted Operation: " + k8sOperation.Name + " CR from Cluster.")
		}
	}
}

// recreateClusterSecrets goes through list of ManagedEnvironments created in cluster and recreates Secrets that are missing from cluster.
func recreateClusterSecrets(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, logger logr.Logger) {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Secret_recreate")

	// First get list of ClusterAccess and Application entries from DB.
	listOfClusterAccessFromDB := getListOfClusterAccessFromTable(ctx, dbQueries, false, log)
	listOfApplicationFromDB := getListOfApplicationsFromTable(ctx, dbQueries, false, log)

	// Now get list of GitopsEngineInstances from DB for the cluster service is running on.
	listOfGitopsEngineInstanceFromCluster, err := getListOfGitopsEngineInstancesForCurrentCluster(ctx, dbQueries, k8sClient, log)
	if err != nil {
		log.Error(err, "Error occurred in recreateClusterSecrets while fetching GitopsEngineInstances for cluster.")
		return
	}

	// map: whether we have processed this namespace already.
	// key is namespace UID, value is not used.
	// - we should only ever process a namespace once per iteration.
	namespacesProcessed := map[string]interface{}{}

	for instanceIndex := range listOfGitopsEngineInstanceFromCluster {

		// For each GitOpsEngineinstance, read the namespace and check all the Argo CD secrets in that namespace
		instance := listOfGitopsEngineInstanceFromCluster[instanceIndex] // To avoid "Implicit memory aliasing in for loop." error.

		// Sanity check: have we processed this Namespace already
		if _, exists := namespacesProcessed[instance.Namespace_uid]; exists {
			// Log it an skip. There really should never be any GitOpsEngineInstances with matching namespace UID
			log.V(logutil.LogLevel_Warn).Error(nil, "there appears to exist multiple GitOpsEngineInstances targeting the same namespace", "namespaceUID", instance.Namespace_uid)
			continue
		}
		namespacesProcessed[instance.Namespace_uid] = nil

		// Iterate through list of ClusterAccess entries from DB and find entry using current GitOpsEngineInstance.
		for clusterAccessIndex := range listOfClusterAccessFromDB {

			clusterAccess := listOfClusterAccessFromDB[clusterAccessIndex] // To avoid "Implicit memory aliasing in for loop." error.

			if clusterAccess.Clusteraccess_gitops_engine_instance_id == instance.Gitopsengineinstance_id {

				// This ClusterAccess is using current GitOpsEngineInstance,
				// now find the ManagedEnvironment using this ClusterAccess.
				managedEnvironment := db.ManagedEnvironment{
					Managedenvironment_id: clusterAccess.Clusteraccess_managed_environment_id,
				}

				if err := dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment); err != nil {
					log.Error(err, "Error occurred in recreateClusterSecrets while fetching ManagedEnvironment from DB.:"+managedEnvironment.Managedenvironment_id)
					continue
				}

				// Skip if ManagedEnvironment is created recently
				if time.Since(managedEnvironment.Created_on) < 30*time.Minute {
					continue
				}

				clusterCreds := db.ClusterCredentials{
					Clustercredentials_cred_id: managedEnvironment.Clustercredentials_id,
				}
				if err := dbQueries.GetClusterCredentialsById(ctx, &clusterCreds); err != nil {
					log.Error(err, "Error occurred in recreateClusterSecrets while retrieving ClusterCredentials:"+managedEnvironment.Clustercredentials_id)
					continue
				}

				// Skip if the cluster credentials do not have a token
				// - this usually indicates that the ManagedEnvironment is on the local cluster, and thus does not require an Argo CD Cluster Secret in order to deploy
				if clusterCreds.Serviceaccount_bearer_token == db.DefaultServiceaccount_bearer_token {
					continue
				}

				// Check if Secret used for this ManagedEnvironment is present in GitOpsEngineInstance namespace.
				secretName := argosharedutil.GenerateArgoCDClusterSecretName(managedEnvironment)

				argoSecret := corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: instance.Namespace_name}, &argoSecret); err != nil {

					// If Secret is not present, then create Operation to recreate the Secret.
					if apierr.IsNotFound(err) {

						log.Info("Secret: " + secretName + " not found in Namespace:" + instance.Namespace_name + ", recreating it.")

						// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
						// Hence created a dummy Cluster User for internal purpose.
						var specialClusterUser db.ClusterUser
						if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
							log.Error(err, "Error occurred in recreateClusterSecrets while fetching clusterUser.")
							return
						}

						// We need to create an Operation to recreate the Secret, which requires Application details running in current ManagedEnvironment
						// hence we iterate through list of Application entries from DB to find that Application.
						if ok, application := getApplicationRunningInManagedEnvironment(listOfApplicationFromDB, managedEnvironment.Managedenvironment_id); ok {

							// We need to recreate Secret, to do that create Operation to inform Argo CD about it.
							dbOperationInput := db.Operation{
								Instance_id:   application.Engine_instance_inst_id,
								Resource_id:   application.Application_id,
								Resource_type: db.OperationResourceType_Application,
							}

							if _, _, err := operations.CreateOperation(ctx, false, dbOperationInput, specialClusterUser.Clusteruser_id, instance.Namespace_name, dbQueries, k8sClient, log); err != nil {
								log.Error(err, "Error occurred in recreateClusterSecrets while creating Operation.")
								continue
							}

							log.Info("Operation " + dbOperationInput.Operation_id + " is created to create Secret: managed-env-" + managedEnvironment.Managedenvironment_id)
						}
					} else {
						log.Error(err, "Error occurred in recreateClusterSecrets while fetching Secret:"+secretName+" from Namespace: "+instance.Namespace_name)
					}
				}
			}
		}
	}
}

// getListOfClusterAccessFromTable loops through ClusterAccess in database and returns list of user IDs.
func getListOfClusterAccessFromTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) []db.ClusterAccess {

	offSet := 0
	var listOfClusterAccessFromDB []db.ClusterAccess

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.ClusterAccess

		// Fetch ClusterAccess table entries in batch size as configured above.​
		if err := dbQueries.GetClusterAccessBatch(ctx, &tempList, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		listOfClusterAccessFromDB = append(listOfClusterAccessFromDB, tempList...)

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}

	return listOfClusterAccessFromDB
}

// getListOfApplicationsFromTable loops through ClusterAccess in database and returns list of user IDs.
func getListOfApplicationsFromTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) []db.Application {

	offSet := 0
	var listOfApplicationsFromDB []db.Application

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.Application

		// Fetch ClusterAccess table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &tempList, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		listOfApplicationsFromDB = append(listOfApplicationsFromDB, tempList...)

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}

	return listOfApplicationsFromDB
}

// getListOfGitopsEngineInstancesForCurrentCluster finds GitopsEngineInstance for the cluster, where service is running.
func getListOfGitopsEngineInstancesForCurrentCluster(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) ([]db.GitopsEngineInstance, error) {

	var gitopsEngineInstance []db.GitopsEngineInstance

	// get Kube System Namespace.
	kubesystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubesystemNamespace), kubesystemNamespace); err != nil {
		return nil, fmt.Errorf("Error occurred in retrieving kube-system namespace, while running secret cleanup: %v", err)
	}

	// get GitopsEngineCluster.
	var err error
	var gitopsEngineCluster *db.GitopsEngineCluster

	if gitopsEngineCluster, err = dbutil.GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, log); err != nil {
		return gitopsEngineInstance, fmt.Errorf("Unable to retrieve GitopsEngineClusterFromDatabase, while running secret cleanup: %v", err)
	} else if gitopsEngineCluster == nil {
		log.Info("Skipping secret cleanup, as the GitOpsEngineCluster does not yet exist for this cluster.")
		return gitopsEngineInstance, nil
	}

	// Get list of GitopsEngineInstance for given cluster from DB.
	if err := dbQueries.ListGitopsEngineInstancesForCluster(ctx, *gitopsEngineCluster, &gitopsEngineInstance); err != nil {
		return nil, fmt.Errorf("Error occurred in Secret Clean-up while fetching list of GitopsEngineInstances.:%v", err)
	}

	return gitopsEngineInstance, nil
}

// getApplicationRunningInManagedEnvironment loops through Applications to find Application runing in given ManagedEnvironment.
func getApplicationRunningInManagedEnvironment(applicationList []db.Application, managedEnvId string) (bool, db.Application) {

	for _, application := range applicationList {
		if application.Managed_environment_id == managedEnvId {
			return true, application
		}
	}

	return false, db.Application{}
}
