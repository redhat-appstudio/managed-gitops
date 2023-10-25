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
			if err := syncCRsWithDB_Applications(ctx, r.DB, r.Client, log); err != nil {
				log.Error(err, "error on syncing Applications with DB")
			}

			// Clean orphaned Secret CRs from Cluster.
			if err := cleanOrphanedCRsfromCluster_Secret(ctx, r.DB, r.Client, log); err != nil {
				log.Error(err, "error on cleaning orphaned Secrets")
			}

			// Clean orphaned and purposeless Operation CRs from Cluster.
			if err := cleanOrphanedCRsfromCluster_Operation(ctx, r.DB, r.Client, log); err != nil {
				log.Error(err, "error on cleaning orphaned Operations")
			}

			// Recreate Secrets that are required by Applications and RepositoryCredentials, but missing from cluster.
			if err := recreateClusterSecrets(ctx, r.DB, r.Client, log); err != nil {
				log.Error(err, "error on recreating cluster secrets")
			}

			log.Info(fmt.Sprintf("Namespace Reconciler finished an iteration at %s. "+
				"Next iteration will be triggered after %v Minutes", time.Now().String(), namespaceReconcilerInterval))

			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'namespaceReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle(ctx, namespaceReconcilerInterval, log)
	}()

}

func syncCRsWithDB_Applications(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, logger logr.Logger) error {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Applications")

	// Fetch list of ArgoCD applications to be used later
	// map: applications IDs seen (string) -> (map value not used)
	processedApplicationIds := make(map[string]any)

	argoApplicationList := appv1.ApplicationList{}
	if err := client.List(ctx, &argoApplicationList); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching list of ArgoCD Applications")
		return fmt.Errorf("error occurred in Namespace Reconciler while fetching list of ArgoCD Applications: %w", err)
	}
	argoApplications := argoApplicationList.Items

	offset := 0

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching ClusterUser")
		return fmt.Errorf("error occurred in Namespace Reconciler while fetching ClusterUser: %w", err)
	}

	log.Info("Triggered Namespace Reconciler to keep Argo Application in sync with DB")

	var res error

	// Delete operation resources created during previous run.
	if err := syncCRsWithDB_Applications_Delete_Operations(ctx, dbQueries, client, log); err != nil {
		if res == nil {
			res = err
		}
	}

	// Continuously iterate and fetch batches until all entries of Application table are processed.
	for {

		if offset != 0 {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApplicationsFromDB []db.Application

		// Fetch Application table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &listOfApplicationsFromDB, appRowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in Namespace Reconciler while fetching batch from offset: %d to %d: ",
				offset, offset+appRowBatchSize))
			if res == nil {
				res = fmt.Errorf("error occurred in Namespace Reconciler while fetching batch from offset: %w", err)
			}
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfApplicationsFromDB) == 0 {
			log.Info("All Application entries are processed by Namespace Reconciler.")
			break
		}

		// Iterate over batch received above.
		for _, applicationRowFromDB := range listOfApplicationsFromDB {

			if err := processApplicationRowToSyncWithCluster(ctx, applicationRowFromDB, &processedApplicationIds, specialClusterUser, dbQueries, client, log); err != nil {
				if res == nil {
					res = fmt.Errorf("unable to processApplicationRowToSyncWithCluster: %w", err)
				}
			}
		}

		// Skip processed entries in next iteration
		offset += appRowBatchSize
	}

	// Start a goroutine, because DeleteArgoCDApplication() function from cluster-agent/controllers may take some time to delete application.
	go func() {
		if _, err := cleanOrphanedCRsfromCluster_Applications(ctx, argoApplications, processedApplicationIds, client, log); err != nil {
			log.Error(err, "unable to cleanOrphaned Argo CD Applications")
		}
	}()

	return res
}

func processApplicationRowToSyncWithCluster(ctx context.Context, applicationRowFromDB db.Application, processedApplicationIds *map[string]any, specialClusterUser db.ClusterUser, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) error {

	var applicationFromDB fauxargocd.FauxApplication

	log = log.WithValues(logutil.Log_ApplicationID, applicationRowFromDB.Application_id)

	(*processedApplicationIds)[applicationRowFromDB.Application_id] = false

	// Fetch the Application object from DB
	if err := yaml.Unmarshal([]byte(applicationRowFromDB.Spec_field), &applicationFromDB); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while unmarshalling application")

		return fmt.Errorf("error occurred in Namespace Reconciler while unmarshalling application: %w", err)
	}

	// Fetch the Application object from k8s
	applicationFromArgoCD := appv1.Application{}
	namespacedName := types.NamespacedName{
		Name:      applicationFromDB.Name,
		Namespace: applicationFromDB.Namespace}

	if err := client.Get(ctx, namespacedName, &applicationFromArgoCD); err != nil {

		if !apierr.IsNotFound(err) {
			log.Error(err, "Error occurred in Namespace Reconciler while fetching application from cluster")
			return fmt.Errorf("error occurred in Namespace Reconciler while fetching application from cluster: %w", err)
		}

		if applicationRowFromDB.Managed_environment_id == "" {
			// We shouldn't recreate any Argo CD Application resources that have a nil managed environment row
			// - A nil managed environment row means the Application is currently invalid (until the user fixes it, by updating the GitOpsDeployment)

			// So we just continue to the next Application
			return nil
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
		if err := dbQueries.GetGitopsEngineInstanceById(ctx, &engineInstanceDB); err != nil {
			log.Error(err, "Unable to fetch GitopsEngineInstance")
			return fmt.Errorf("unable to fetch GitopsEngineInstance: %w", err)
		}

		if _, _, err := operations.CreateOperation(ctx, false, dbOperationInput,
			specialClusterUser.Clusteruser_id, engineInstanceDB.Namespace_name, dbQueries, client, log); err != nil {

			log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
			return fmt.Errorf("namespace Reconciler is unable to create operation: %w", err)
		}
		log.Info("Operation is created to recreateArgoCD")

		return nil
	}

	// If the managed_environment_id row is not empty, then it depends on whether the DB spec field matches the
	if applicationRowFromDB.Managed_environment_id != "" {

		// At this point we have the applications from ArgoCD and DB, now compare them to check if they are not in Sync.

		if compare, err := controllers.CompareApplication(applicationFromArgoCD, applicationRowFromDB, log); err != nil {
			log.Error(err, "unable to compare application contents")
			return fmt.Errorf("unable to compare application contents: %w", err)
		} else if compare != "" {
			log.Info("Argo application is not in Sync with DB, updating Argo CD App")
		} else {
			log.V(logutil.LogLevel_Debug).Info("Argo application is in Sync with DB")
			return nil
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
		return fmt.Errorf("unable to fetch GitopsEngineInstance: %w", err)
	}

	if _, dbOp, err := operations.CreateOperation(ctx, false, dbOperationInput,
		specialClusterUser.Clusteruser_id, engineInstanceDB.Namespace_name, dbQueries, client, log); err != nil {

		log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
		return fmt.Errorf("namespace Reconciler is unable to create operation: %w", err)

	} else if dbOp != nil {
		log.Info("Operation is created to sync Application row with Argo CD Application", "operationDBID", dbOp.Operation_id)

	}

	return nil
}

func syncCRsWithDB_Applications_Delete_Operations(ctx context.Context, dbq db.DatabaseQueries, client client.Client, log logr.Logger) error {
	// Get list of Operations from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	if err := client.List(ctx, &listOfK8sOperation); err != nil {
		log.Error(err, "Unable to fetch list of k8s Operation from cluster.")
		return fmt.Errorf("unable to fetch list of k8s Operation from cluster: %w", err)
	}

	var res error

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

			if res == nil {
				res = fmt.Errorf("unable to get operation: %w", err)
			}
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
			if res == nil {
				res = fmt.Errorf("unable to delete k8s Operation: %w", err)
			}
		} else {
			log.V(logutil.LogLevel_Debug).Info("Clean up job for CR deleting Operation created by Namespace Reconciler.")
		}
	}

	log.Info("Cleaned all Operations created by Namespace Reconciler.")

	return res
}

func cleanOrphanedCRsfromCluster_Applications(ctx context.Context, argoApplications []appv1.Application, processedApplicationIds map[string]any,
	client client.Client, log logr.Logger) ([]appv1.Application, error) {

	if len(argoApplications) == 0 {
		return []appv1.Application{}, nil
	}

	shuffledList := argoApplications

	// Shuffle the list of Argo Applications, so that we are not always deleting in the same order.
	// - This is beneficial when we have a long list of Applications to delete, that take longer than namespaceReconcilerInterval.
	rand.Shuffle(len(shuffledList), func(i, j int) {
		shuffledList[i], shuffledList[j] = shuffledList[j], shuffledList[i]
	})

	var res error

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
				if res == nil {
					res = fmt.Errorf("unable to delete an orphaned Argo CD Application: %w", err)
				}

			} else {
				deletedOrphanedApplications = append(deletedOrphanedApplications, application)
				log.Info("Deleted orphaned Argo CD Application")
			}
		}
	}
	return deletedOrphanedApplications, res
}

// cleanOrphanedCRsfromCluster_Secret goes through the Argo CD Cluster/Repository Secrets, and deletes secrets that no longer point to valid database entries.
func cleanOrphanedCRsfromCluster_Secret(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, logger logr.Logger) error {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Secret")

	kubesystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubesystemNamespace), kubesystemNamespace); err != nil {
		log.Error(err, "Error occurred in retrieving kube-system namespace, while running secret cleanup")
		return fmt.Errorf("error occurred in retrieving kube-system namespace, while running secret cleanup: %w", err)
	}

	var err error
	var gitopsEngineCluster *db.GitopsEngineCluster
	if gitopsEngineCluster, err = dbutil.GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, log); err != nil {
		log.Error(err, "Unable to retrieve GitopsEngineClusterFromDatabase, while running secret cleanup")
		return fmt.Errorf("unable to retrieve GitopsEngineClusterFromDatabase, while running secret cleanup: %w", err)
	} else if gitopsEngineCluster == nil {
		log.Info("Skipping secret cleanup, as the GitOpsEngineCluster does not yet exist for this cluster.")
		return nil
	}

	// map: whether we have processed this namespace already.
	// key is namespace UID, value is not used.
	// - we should only ever process a namespace once per iteration.
	namespacesProcessed := map[string]interface{}{}

	// Get list of instances from GitopsEngineInstance table.
	var gitopsEngineInstance []db.GitopsEngineInstance

	if err := dbQueries.ListGitopsEngineInstancesForCluster(ctx, *gitopsEngineCluster, &gitopsEngineInstance); err != nil {
		log.Error(err, "Error occurred in Secret Clean-up while fetching list of GitopsEngineInstances")
		return fmt.Errorf("error occurred in Secret Clean-up while fetching list of GitopsEngineInstances: %w", err)
	}

	var res error

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
			log.Error(err, "Error occurred in Secret Clean-up while fetching list of Secrets from Namespace: "+instance.Namespace_name)
			return fmt.Errorf("error occurred in Secret Clean-up while fetching list of Secrets from Namespace: %w", err)
		}

		for secretIndex := range secretList.Items {
			secret := secretList.Items[secretIndex] // To avoid "Implicit memory aliasing in for loop." error.

			// cleanOrphanedCRsfromCluster_Secret_Delete looks for orphaned Argo CD Cluster/Repo secrets, and if orphaned, deletes the cluster secret
			if err := cleanOrphanedCRsfromCluster_Secret_Delete(ctx, secret, dbQueries, k8sClient, log); err != nil {
				if res == nil {
					res = fmt.Errorf("unable to clean orned crs from cluster secret: %w", err)
				}
			}
		}
	}

	return res
}

// cleanOrphanedCRsfromCluster_Secret_Delete looks for orphaned Argo CD Cluster/Repo secrets, and if orphaned, deletes the cluster secret
func cleanOrphanedCRsfromCluster_Secret_Delete(ctx context.Context, secret corev1.Secret, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) error {

	// Look for secrets which have required labels i.e databaseID and argocd.argoproj.io/secret-type
	// Ignore secrets which don't have these labels
	var secretType string
	databaseID, ok := secret.Labels[SecretDbIdentifierKey]
	if !ok {
		// Secret does not have 'databaseID' label, so no work to do.
		return nil
	}

	secretType, ok = secret.Labels[sharedutil.ArgoCDSecretTypeIdentifierKey]
	if !ok {
		// Secret does not have the label that Argo CD uses to identify what type of Secret it is, so just return
		return nil
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
		return nil
	}

	log = log.WithValues("secretName", secret.Name, "secretNamespace", secret.Namespace, "secretType", secretType)

	if db.IsResultNotFoundError(err) {
		// If entry is not present in DB then it is an orphaned CR, hence we delete the Secret from cluster.
		if err := k8sClient.Delete(ctx, &secret); err != nil {
			log.Error(err, "error occurred in Secret Clean-up while deleting an orphan Secret")

			return fmt.Errorf("error occurred in Secret Clean-up while deleting an orphan Secret: %w", err)

		}
		log.Info("Clean up job deleted orphaned Secret CR")

		return nil

	} else {
		// Some other unexpected error occurred, so we just skip it until next time
		log.Error(err, "unexpected error occurred in Secret Clean-up while fetching DB entry pointed by Secret")

		return fmt.Errorf("unexpected error occurred in Secret Clean-up while fetching DB entry pointed by Secret: %w", err)
	}

}

// cleanOrphanedCRsfromCluster_Operation goes through the Operation CRs of cluster, and deletes CRs that are no longer point to valid database entries or already completed.
func cleanOrphanedCRsfromCluster_Operation(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, logger logr.Logger) error {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Applications")

	// Get list of Operations from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	if err := k8sClient.List(ctx, &listOfK8sOperation); err != nil {
		// If there is an error then return and clean CRs in next iteration.
		log.Error(err, "unable to fetch list of Operation from cluster.")
		return fmt.Errorf("unable to fetch list of Operation from cluster: %w", err)
	}

	var res error

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
				if res == nil {
					res = fmt.Errorf("error occurred in cleanOrphanedCRsfromCluster_Operation while fetching Operation: %w", err)
				}
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
				if res == nil {
					res = fmt.Errorf("unable to delete orphaned Operation from cluster: %w", err)
				}
			}
			log.Info("Managed-gitops clean up job for CR successfully deleted Operation: " + k8sOperation.Name + " CR from Cluster.")
		}
	}

	return res
}

// recreateClusterSecrets goes through list of ManagedEnvironments & RepositoryCredentials created in cluster and recreates Secrets that are missing from cluster.
func recreateClusterSecrets(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, logger logr.Logger) error {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Secret_recreate")

		// First get list of ClusterAccess, Application and RepositoryCredentials entries from DB.

	listOfClusterAccessFromDB, err := getListOfClusterAccessFromTable(ctx, dbQueries, false, log)
	if err != nil {
		return fmt.Errorf("unable to retrieve ClusterAccess from table: %w", err)
	}

	listOfApplicationFromDB, err := getListOfApplicationsFromTable(ctx, dbQueries, false, log)
	if err != nil {
		return fmt.Errorf("unable to retrieve Applications from table: %w", err)
	}

	listOfRepoCredFromDB, err := getListOfRepositoryCredentialsFromTable(ctx, dbQueries, false, log)
	if err != nil {
		return fmt.Errorf("unable to retrieve RepositoryCredentials from table: %w", err)
	}

	// Now get list of GitopsEngineInstances from DB for the cluster service is running on.
	listOfGitopsEngineInstanceFromCluster, err := getListOfGitopsEngineInstancesForCurrentCluster(ctx, dbQueries, k8sClient, log)
	if err != nil {
		log.Error(err, "Error occurred in recreateClusterSecrets while fetching GitopsEngineInstances for cluster.")
		return fmt.Errorf("error occurred in recreateClusterSecrets while fetching GitopsEngineInstances for cluster: %w", err)
	}

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching ClusterUser")
		return fmt.Errorf("error occurred in Namespace Reconciler while fetching ClusterUser: %w", err)
	}

	// map: whether we have processed this namespace already.
	// key is namespace UID, value is not used.
	// - we should only ever process a namespace once per iteration.
	namespacesProcessed := map[string]interface{}{}

	var res error

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

		if err := recreateClusterSecrets_ManagedEnvironments(ctx, dbQueries, k8sClient, listOfClusterAccessFromDB, listOfApplicationFromDB, instance, log); err != nil {

			if res == nil {
				res = fmt.Errorf("unable to recreate cluster secret for managed envs: %w", err)
			}
		}
		if err := recreateClusterSecrets_RepositoryCredentials(ctx, dbQueries, k8sClient, listOfRepoCredFromDB, instance, log); err != nil {
			if res == nil {
				res = fmt.Errorf("unable to create cluster secret for repository credentials: %w", err)
			}
		}
	}
	return res
}

// recreateClusterSecrets_ManagedEnvironments goes through list of ManagedEnvironments created in cluster and recreates Secrets that are missing from cluster.
func recreateClusterSecrets_ManagedEnvironments(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, listOfClusterAccessFromDB []db.ClusterAccess, listOfApplicationFromDB []db.Application, instance db.GitopsEngineInstance, logger logr.Logger) error {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Secret_recreate_managedEnv")

	var res error

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in recreateClusterSecrets_ManagedEnvironments while fetching clusterUser.")
		return fmt.Errorf("error occurred in recreateClusterSecrets_ManagedEnvironments while fetching clusterUser: %w", err)
	}

	// Iterate through list of ClusterAccess entries from DB and find entry using current GitOpsEngineInstance.
	for clusterAccessIndex := range listOfClusterAccessFromDB {

		clusterAccess := listOfClusterAccessFromDB[clusterAccessIndex] // To avoid "Implicit memory aliasing in for loop." error.

		if clusterAccess.Clusteraccess_gitops_engine_instance_id != instance.Gitopsengineinstance_id {
			// skip ClusterAccess rows that don't match the current Argo CD (GitOpsEngineInstance)
			continue
		}

		if err := recreateManagedEnvClusterSecretFromClusterAccess(ctx, clusterAccess, instance, listOfApplicationFromDB, specialClusterUser, dbQueries, k8sClient, log); err != nil {

			if res == nil {
				res = fmt.Errorf("unable to recreate managed env cluster secret: %w", err)
			}
		}

	}

	return res
}

func recreateManagedEnvClusterSecretFromClusterAccess(ctx context.Context, clusterAccess db.ClusterAccess, instance db.GitopsEngineInstance, listOfApplicationFromDB []db.Application, specialClusterUser db.ClusterUser, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) error {

	// This ClusterAccess is using current GitOpsEngineInstance,
	// now find the ManagedEnvironment using this ClusterAccess.
	managedEnvironment := db.ManagedEnvironment{
		Managedenvironment_id: clusterAccess.Clusteraccess_managed_environment_id,
	}

	if err := dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment); err != nil {
		log.Error(err, "Error occurred in recreateClusterSecrets_ManagedEnvironments while fetching ManagedEnvironment from DB.:"+managedEnvironment.Managedenvironment_id)
		return fmt.Errorf("error occurred in recreateClusterSecrets_ManagedEnvironments while fetching ManagedEnvironment from DB: %w", err)
	}

	// Skip if ManagedEnvironment is created recently
	if time.Since(managedEnvironment.Created_on) < 30*time.Minute {
		return nil
	}

	clusterCreds := db.ClusterCredentials{
		Clustercredentials_cred_id: managedEnvironment.Clustercredentials_id,
	}
	if err := dbQueries.GetClusterCredentialsById(ctx, &clusterCreds); err != nil {
		log.Error(err, "Error occurred in recreateClusterSecrets while retrieving ClusterCredentials:"+managedEnvironment.Clustercredentials_id)
		return fmt.Errorf("error occurred in recreateClusterSecrets while retrieving ClusterCredentials: %s, %w", managedEnvironment.Clustercredentials_id, err)
	}

	// Skip if the cluster credentials do not have a token
	// - this usually indicates that the ManagedEnvironment is on the local cluster, and thus does not require an Argo CD Cluster Secret in order to deploy
	if clusterCreds.Serviceaccount_bearer_token == db.DefaultServiceaccount_bearer_token {
		return nil
	}

	// Check if Secret used for this ManagedEnvironment is present in GitOpsEngineInstance namespace.
	secretName := argosharedutil.GenerateArgoCDClusterSecretName(managedEnvironment)

	argoSecret := corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: instance.Namespace_name}, &argoSecret); err != nil {

		// If Secret is not present, then create Operation to recreate the Secret.
		if apierr.IsNotFound(err) {

			log.Info("Secret: " + secretName + " not found in Namespace:" + instance.Namespace_name + ", recreating it.")

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
					log.Error(err, "Error occurred in recreateClusterSecrets_ManagedEnvironments while creating Operation.")
					return fmt.Errorf("error occurred in recreateClusterSecrets_ManagedEnvironments while creating Operation: %w", err)
				}

				log.Info("Operation " + dbOperationInput.Operation_id + " is created to create Secret: managed-env-" + managedEnvironment.Managedenvironment_id)
			}
		} else {
			log.Error(err, "Error occurred in recreateClusterSecrets_ManagedEnvironments while fetching Secret:"+secretName+" from Namespace: "+instance.Namespace_name)

			return fmt.Errorf("error occurred in recreateClusterSecrets_ManagedEnvironments while fetching Secret:"+secretName+" from Namespace: %s, %w", instance.Namespace_name, err)
		}
	}

	return nil
}

// recreateClusterSecrets_RepositoryCredentials goes through list of RepositoryCredentials created in cluster and recreates Secrets that are missing from cluster.
func recreateClusterSecrets_RepositoryCredentials(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, listOfRepoCredFromDB []db.RepositoryCredentials, instance db.GitopsEngineInstance, logger logr.Logger) error {

	log := logger.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "CR_Secret_recreate_repoCred")

	var res error

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in recreateClusterSecrets_RepositoryCredentials while fetching clusterUser.")
		return fmt.Errorf("error occurred in recreateClusterSecrets_RepositoryCredentials while fetching clusterUser: %w", err)
	}

	// Iterate through list of RepositoryCredentials entries from DB and find entry using current GitOpsEngineInstance.
	for repositoryCredentialsIndex := range listOfRepoCredFromDB {

		repositoryCredentials := listOfRepoCredFromDB[repositoryCredentialsIndex] // To avoid "Implicit memory aliasing in for loop." error.

		if repositoryCredentials.EngineClusterID == instance.Gitopsengineinstance_id {
			// Skip if RepositoryCredentials is created recently
			if time.Since(repositoryCredentials.Created_on) < 30*time.Minute {
				continue
			}

			// Check if Secret used for this RepositoryCredentials is present in GitOpsEngineInstance namespace.
			argoSecret := corev1.Secret{}

			if err := k8sClient.Get(ctx, types.NamespacedName{Name: argosharedutil.GenerateArgoCDRepoCredSecretName(repositoryCredentials), Namespace: instance.Namespace_name}, &argoSecret); err != nil {

				// If Secret is not present, then create Operation to recreate the Secret.
				if apierr.IsNotFound(err) {

					log.Info("Secret: " + repositoryCredentials.SecretObj + " not found in Namespace:" + instance.Namespace_name + ", recreating it.")

					// We need to recreate Secret, to do that create Operation to inform Argo CD about it.
					dbOperationInput := db.Operation{
						Instance_id:   instance.Gitopsengineinstance_id,
						Resource_id:   repositoryCredentials.RepositoryCredentialsID,
						Resource_type: db.OperationResourceType_RepositoryCredentials,
					}

					if _, _, err := operations.CreateOperation(ctx, false, dbOperationInput, specialClusterUser.Clusteruser_id, instance.Namespace_name, dbQueries, k8sClient, log); err != nil {
						log.Error(err, "Error occurred in recreateClusterSecrets_RepositoryCredentials while creating Operation.")
						if res == nil {
							res = fmt.Errorf("error occurred in recreateClusterSecrets_RepositoryCredentials while creating Operation: %w", err)
						}
						continue
					}

					log.Info("Operation " + dbOperationInput.Operation_id + " is created to create Secret: " + repositoryCredentials.SecretObj)
				} else {
					log.Error(err, "Error occurred in recreateClusterSecrets_RepositoryCredentials while fetching Secret:"+repositoryCredentials.SecretObj+" from Namespace: "+instance.Namespace_name)
					if res == nil {
						res = fmt.Errorf("error occurred in recreateClusterSecrets_RepositoryCredentials while fetching Secret: %w", err)
					}
				}
			}
		}
	}

	return res
}

// getListOfClusterAccessFromTable loops through ClusterAccess in database and returns list of user IDs.
func getListOfClusterAccessFromTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) ([]db.ClusterAccess, error) {

	offset := 0
	var listOfClusterAccessFromDB []db.ClusterAccess

	var res error

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.ClusterAccess

		// Fetch ClusterAccess table entries in batch size as configured above.​
		if err := dbQueries.GetClusterAccessBatch(ctx, &tempList, appRowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from offset: %d to %d: ",
				offset, offset+appRowBatchSize))

			res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from offset: %w", err)
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		listOfClusterAccessFromDB = append(listOfClusterAccessFromDB, tempList...)

		// Skip processed entries in next iteration
		offset += appRowBatchSize
	}

	return listOfClusterAccessFromDB, res
}

// getListOfApplicationsFromTable loops through ClusterAccess in database and returns list of user IDs.
func getListOfApplicationsFromTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) ([]db.Application, error) {

	offset := 0
	var listOfApplicationsFromDB []db.Application

	var res error

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.Application

		// Fetch ClusterAccess table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &tempList, appRowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from offset: %d to %d: ",
				offset, offset+appRowBatchSize))
			res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from offset: %w", err)
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		listOfApplicationsFromDB = append(listOfApplicationsFromDB, tempList...)

		// Skip processed entries in next iteration
		offset += appRowBatchSize
	}

	return listOfApplicationsFromDB, res
}

// getListOfGitopsEngineInstancesForCurrentCluster finds GitopsEngineInstance for the cluster, where service is running.
func getListOfGitopsEngineInstancesForCurrentCluster(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) ([]db.GitopsEngineInstance, error) {

	var gitopsEngineInstance []db.GitopsEngineInstance

	// get Kube System Namespace.
	kubesystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubesystemNamespace), kubesystemNamespace); err != nil {
		return nil, fmt.Errorf("error occurred in retrieving kube-system namespace, while running secret cleanup: %v", err)
	}

	// get GitopsEngineCluster.
	var err error
	var gitopsEngineCluster *db.GitopsEngineCluster

	if gitopsEngineCluster, err = dbutil.GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, log); err != nil {
		return gitopsEngineInstance, fmt.Errorf("unable to retrieve GitopsEngineClusterFromDatabase, while running secret cleanup: %v", err)
	} else if gitopsEngineCluster == nil {
		log.Info("Skipping secret cleanup, as the GitOpsEngineCluster does not yet exist for this cluster.")
		return gitopsEngineInstance, nil
	}

	// Get list of GitopsEngineInstance for given cluster from DB.
	if err := dbQueries.ListGitopsEngineInstancesForCluster(ctx, *gitopsEngineCluster, &gitopsEngineInstance); err != nil {
		return nil, fmt.Errorf("error occurred in Secret Clean-up while fetching list of GitopsEngineInstances.:%v", err)
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

// getListOfRepositoryCredentialsFromTable loops through RepositoryCredentials in database and returns list of user IDs.
func getListOfRepositoryCredentialsFromTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) ([]db.RepositoryCredentials, error) {

	var res error

	offset := 0
	var listOfRepositoryCredentialsFromDB []db.RepositoryCredentials

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.RepositoryCredentials

		// Fetch ClusterAccess table entries in batch size as configured above.​
		if err := dbQueries.GetRepositoryCredentialsBatch(ctx, &tempList, appRowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offset, offset+appRowBatchSize))
			if res != nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
			}
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		listOfRepositoryCredentialsFromDB = append(listOfRepositoryCredentialsFromDB, tempList...)

		// Skip processed entries in next iteration
		offset += appRowBatchSize
	}

	return listOfRepositoryCredentialsFromDB, res
}
