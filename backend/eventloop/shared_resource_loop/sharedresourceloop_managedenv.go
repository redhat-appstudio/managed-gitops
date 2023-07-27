package shared_resource_loop

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/gitopserrors"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerLog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	KubeconfigKey = "kubeconfig"
)

func internalProcessMessage_ReconcileSharedManagedEnv(ctx context.Context, workspaceClient client.Client,
	managedEnvironmentCRName string,
	managedEnvironmentCRNamespace string,
	isWorkspaceTarget bool,
	workspaceNamespace corev1.Namespace,
	k8sClientFactory SRLK8sClientFactory,
	dbQueries db.DatabaseQueries,
	log logr.Logger) (SharedResourceManagedEnvContainer, error) {

	container, condition, err := internalProcessMessage_internalReconcileSharedManagedEnv(ctx, workspaceClient, managedEnvironmentCRName,
		managedEnvironmentCRNamespace, isWorkspaceTarget, workspaceNamespace, k8sClientFactory, dbQueries, log)

	if condition.reason != "" && condition.managedEnvCR.Name != "" {

		// If a metav1.Condition{} needs to be set, set it here.
		updateManagedEnvironmentConnectionStatus(ctx, condition.managedEnvCR, workspaceClient, condition, log)

	}

	return container, err

}

func internalProcessMessage_internalReconcileSharedManagedEnv(ctx context.Context, workspaceClient client.Client,
	managedEnvironmentCRName string,
	managedEnvironmentCRNamespace string,
	isWorkspaceTarget bool,
	workspaceNamespace corev1.Namespace,
	k8sClientFactory SRLK8sClientFactory,
	dbQueries db.DatabaseQueries,
	log logr.Logger) (SharedResourceManagedEnvContainer, connectionInitializedCondition, error) {

	gitopsEngineClient, err := k8sClientFactory.GetK8sClientForGitOpsEngineInstance(ctx, nil)
	if err != nil {
		return newSharedResourceManagedEnvContainer(), createUnknownErrorEnvInitCondition(), err
	}

	// If the GitOpsDeployment's 'target' field has an empty environment field, indicating it is targeting the same
	// namespace as the GitOpsDeployment itself, then we use a separate function to process the message.
	if isWorkspaceTarget {
		return internalProcessMessage_GetOrCreateSharedResources(ctx, gitopsEngineClient, workspaceNamespace, dbQueries, log)
	}

	clusterUser, isNewUser, err := internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx, workspaceNamespace, dbQueries, log)
	if err != nil || clusterUser == nil {
		return newSharedResourceManagedEnvContainer(), createUnknownErrorEnvInitCondition(),
			fmt.Errorf("unable to retrieve cluster user in processMessage, '%s': %v", string(workspaceNamespace.UID), err)
	}

	// Attempt to retrieve the CRs; if they don't exist, then delete the corresponding Managed Environment DB entry
	managedEnvironmentCR, secretCR, doesNotExist, err := getManagedEnvironmentCRs(ctx, managedEnvironmentCRName,
		managedEnvironmentCRNamespace, workspaceClient, workspaceNamespace, k8sClientFactory, dbQueries, *clusterUser, log)

	if err != nil {
		return newSharedResourceManagedEnvContainer(), createUnknownErrorEnvInitCondition(), err
	}
	if doesNotExist {
		return newSharedResourceManagedEnvContainer(), createUnknownErrorEnvInitCondition(), nil
	}

	if strings.Contains(managedEnvironmentCR.Spec.APIURL, "?") || strings.Contains(managedEnvironmentCR.Spec.APIURL, "&") {

		return newSharedResourceManagedEnvContainer(), connectionInitializedCondition{
			managedEnvCR: managedEnvironmentCR,
			status:       metav1.ConditionUnknown,
			reason:       managedgitopsv1alpha1.ConditionReasonUnsupportedAPIURL,
			message:      "the API URL must not have ? or & values",
		}, nil
	}

	// After this point in the code, the API CR necessarily exists.

	// Retrieve all existing APICRToDatabaseMappings for this resource name/namespace, and clean up the ones that don't match the UID
	// of managedEnvironmentNew
	if err := deleteManagedEnvironmentDBByAPINameAndNamespace(ctx, workspaceClient, managedEnvironmentCRName, managedEnvironmentCRNamespace,
		string(managedEnvironmentCR.UID), workspaceNamespace, k8sClientFactory, dbQueries, *clusterUser, log); err != nil {

		return SharedResourceManagedEnvContainer{},
			createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
			fmt.Errorf("unable to delete old managed environments by API name and namespace '%s' in '%s': %w",
				managedEnvironmentCRName, managedEnvironmentCRNamespace, err)
	}

	apiCRToDBMapping := db.APICRToDatabaseMapping{
		APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
		APIResourceUID:  string(managedEnvironmentCR.UID),
		DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
	}

	if err := dbQueries.GetDatabaseMappingForAPICR(ctx, &apiCRToDBMapping); err != nil {

		if !db.IsResultNotFoundError(err) {

			return newSharedResourceManagedEnvContainer(),
				createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
				fmt.Errorf("unable to retrieve managed environment APICRToDatabaseMapping for %s: %w", apiCRToDBMapping.APIResourceUID, err)
		}

		// A) If there exists no APICRToDatabaseMapping for this Managed Environment resource, then just create a new managed environment
		//    for it, and return that.
		return constructNewManagedEnv(ctx, gitopsEngineClient, workspaceClient, *clusterUser, isNewUser, managedEnvironmentCR, secretCR, workspaceNamespace, k8sClientFactory, dbQueries, log)
	}

	managedEnv := &db.ManagedEnvironment{
		Managedenvironment_id: apiCRToDBMapping.DBRelationKey,
	}
	if err := dbQueries.GetManagedEnvironmentById(ctx, managedEnv); err != nil {

		if !db.IsResultNotFoundError(err) {
			return newSharedResourceManagedEnvContainer(),
				createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
				fmt.Errorf("unable to retrieve managed environment '%s", managedEnv.Managedenvironment_id)
		}

		// B) The APICRToDBMapping exists, but the managed env doesn't, so delete the mapping, then create the
		//    managed environment/mapping from scratch.
		rowsDeleted, err := dbQueries.DeleteAPICRToDatabaseMapping(ctx, &apiCRToDBMapping)
		if err != nil {
			return newSharedResourceManagedEnvContainer(),
				createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
				fmt.Errorf("unable to delete APICRToDatabaseMapping for '%s'", apiCRToDBMapping.APIResourceUID)
		}
		if rowsDeleted != 1 {
			// Warn, but continue.
			log.V(logutil.LogLevel_Warn).Info("unexpected number of rows deleted for APICRToDatabaseMapping", "mapping", apiCRToDBMapping.APIResourceUID)
		}

		return constructNewManagedEnv(ctx, gitopsEngineClient, workspaceClient, *clusterUser, isNewUser, managedEnvironmentCR, secretCR, workspaceNamespace, k8sClientFactory, dbQueries, log)
	}

	clusterCreds := &db.ClusterCredentials{
		Clustercredentials_cred_id: managedEnv.Clustercredentials_id,
	}
	if err := dbQueries.GetClusterCredentialsById(ctx, clusterCreds); err != nil {

		if !db.IsResultNotFoundError(err) {
			return newSharedResourceManagedEnvContainer(),
				createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
				fmt.Errorf("unable to retrieve cluster credentials for '%s': %w", clusterCreds.Clustercredentials_cred_id, err)
		}

		// Sanity test:
		// Cluster credentials referenced by managed environment doesn't exist.
		// However, this really shouldn't be possible, since there is a foreign key from managed environment to cluster credentials.
		return newSharedResourceManagedEnvContainer(),
			createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
			fmt.Errorf("SEVERE: managed environment referenced cluster credentials value which doesn't exist: %w", err)

	}

	managedEnvNamespaceSliceList, err := convertManagedEnvNamespacesFieldToCommaSeparatedList(managedEnvironmentCR.Spec.Namespaces)
	if err != nil {
		msg := fmt.Sprintf("user specified an invalid namespace: %v", err)
		return newSharedResourceManagedEnvContainer(),
			connectionInitializedCondition{
				managedEnvCR: managedEnvironmentCR,
				status:       metav1.ConditionUnknown,
				reason:       managedgitopsv1alpha1.ConditionReasonInvalidNamespaceList,
				message:      msg,
			}, errors.New(msg)
	}

	// We found the managed env, now verify that the ManagedEnv's .spec values match the corresponding fields in the ClusterCredentials row
	if clusterCreds.Host != managedEnvironmentCR.Spec.APIURL ||
		clusterCreds.AllowInsecureSkipTLSVerify != managedEnvironmentCR.Spec.AllowInsecureSkipTLSVerify ||
		clusterCreds.ClusterResources != managedEnvironmentCR.Spec.ClusterResources ||
		clusterCreds.Namespaces != managedEnvNamespaceSliceList {
		// C) If at least one of the fields in the managed env CR has changed, then replace the cluster credentials of the managed environment
		return replaceExistingManagedEnv(ctx, gitopsEngineClient, workspaceClient, *clusterUser, isNewUser, managedEnvironmentCR, secretCR, *managedEnv,
			workspaceNamespace, k8sClientFactory, dbQueries, log)
	}

	// Verify that we are able to connect to the cluster using the service account token we stored
	validClusterCreds, err := verifyClusterCredentialsWithNamespaceList(ctx, *clusterCreds, managedEnvironmentCR, k8sClientFactory)
	if !validClusterCreds || err != nil {
		log.Info("was unable to connect using provided cluster credentials, so acquiring new ones.", "clusterCreds", clusterCreds.Clustercredentials_cred_id)
		// D) If the cluster credentials appear to no longer be valid (we're no longer able to connect), then reacquire using the
		// Secret.
		return replaceExistingManagedEnv(ctx, gitopsEngineClient, workspaceClient, *clusterUser, isNewUser, managedEnvironmentCR, secretCR, *managedEnv,
			workspaceNamespace, k8sClientFactory, dbQueries, log)
	}

	// The API url hasn't changed, the existing service account still works, so no more work needed.

	// E) We already have an existing managed env from the database, so get or create the remaining items for it

	engineInstance, isNewEngineInstance, clusterAccess, isNewClusterAccess, engineCluster, uerr := wrapManagedEnv(ctx,
		*managedEnv, workspaceNamespace, *clusterUser, gitopsEngineClient, dbQueries, log)

	if uerr != nil {
		return newSharedResourceManagedEnvContainer(),
			convertConditionErrorToConnInitCondition(uerr, managedEnvironmentCR),
			fmt.Errorf("unable to wrap managed environment, on existing managed env, for %s: %w", apiCRToDBMapping.APIResourceUID, uerr.DevError())
	}

	res := SharedResourceManagedEnvContainer{
		ClusterUser:          clusterUser,
		IsNewUser:            isNewUser,
		ManagedEnv:           managedEnv,
		IsNewManagedEnv:      false,
		GitopsEngineInstance: engineInstance,
		IsNewInstance:        isNewEngineInstance,
		ClusterAccess:        clusterAccess,
		IsNewClusterAccess:   isNewClusterAccess,
		GitopsEngineCluster:  engineCluster,
	}

	// Ensure the managed environment CR has a connection status of "Succeeded"
	return res, createSuccessEnvInitCondition(managedEnvironmentCR), nil
}

// getManagedEnvironmentCRs retrieves the Managed Environment and Secret CRs.
// returns:
// - managed environment and secret CRs, if they exist
// - bool: whether or not the managed env CR does not exist: true if the CR doesn't exist, false otherwise.
// - error
func getManagedEnvironmentCRs(ctx context.Context,
	managedEnvironmentCRName string,
	managedEnvironmentCRNamespace string,
	workspaceClient client.Client, workspaceNamespace corev1.Namespace, k8sClientFactory SRLK8sClientFactory, dbQueries db.DatabaseQueries,
	clusterUser db.ClusterUser, log logr.Logger) (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret, bool, error) {

	const (
		resourceDoesNotExist = true
		resourceExists       = false
	)

	// Attempt to retrieve the CRs; if they don't exist, then delete the corresponding Managed Environment DB entry
	// Retrieve the managed environment
	managedEnvironmentCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedEnvironmentCRName,
			Namespace: managedEnvironmentCRNamespace,
		},
	}
	if err := workspaceClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvironmentCR), &managedEnvironmentCR); err != nil {

		// If the managed environment CR doesn't exist, then ensure it is deleted from the database, then exit.
		if apierr.IsNotFound(err) {
			log.Info("Managed environment could not be found, so function was called to clean database entry.")

			err = deleteManagedEnvironmentDBByAPINameAndNamespace(ctx, workspaceClient, managedEnvironmentCRName,
				managedEnvironmentCRNamespace, "", workspaceNamespace, k8sClientFactory, dbQueries, clusterUser, log)

			return managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}, corev1.Secret{}, resourceDoesNotExist, err
		}

		return managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}, corev1.Secret{}, resourceExists,
			fmt.Errorf("managed environment '%s' in '%s', could not be retrieved: %v", managedEnvironmentCR.Name, managedEnvironmentCR.Namespace, err)
	}

	if managedEnvironmentCR.Spec.ClusterCredentialsSecret == "" {
		return managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}, corev1.Secret{}, resourceExists,
			fmt.Errorf("secret '%s' referenced by managed environment '%s' in '%s', is invalid",
				managedEnvironmentCR.Spec.ClusterCredentialsSecret, managedEnvironmentCR.Name, managedEnvironmentCR.Namespace)
	}

	// Retrieve the Secret CR from the workspace
	secretCR := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedEnvironmentCR.Spec.ClusterCredentialsSecret,
			Namespace: managedEnvironmentCR.Namespace,
		},
	}
	if err := workspaceClient.Get(ctx, client.ObjectKeyFromObject(&secretCR), &secretCR); err != nil {
		return managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}, corev1.Secret{}, resourceExists,
			fmt.Errorf("secret '%s' referenced by managed environment '%s' in '%s', could not be retrieved: %v",
				managedEnvironmentCR.Spec.ClusterCredentialsSecret, managedEnvironmentCR.Name, managedEnvironmentCR.Namespace, err)
	}

	return managedEnvironmentCR, secretCR, resourceExists, nil
}

// deleteManagedEnvironmentDBByAPINameAndNamespace will delete all the Managed Environments DB resources (plus related DB resources) that
// have the given name/namespace (that don't match 'skipResourcesWithK8sUID')
// Parameters:
// - managedEnvironmentCRName: GitOpsDeploymentManagedEnvironment resource name
// - managedEnvironmentCRNamespace: GitOpsDeploymentManagedEnvironment resource namespace
// - skipResourceWithK8sUID: If 'skipResourceWithK8sUID' is non-empty, resources with this UID will NOT be deleted.
//
// Note: skipResourceWithK8sUID can be used to skip deleting a managed environment that is still valid.
func deleteManagedEnvironmentDBByAPINameAndNamespace(ctx context.Context, workspaceClient client.Client,
	managedEnvironmentCRName string,
	managedEnvironmentCRNamespace string,
	skipResourceWithK8sUID string,
	workspaceNamespace corev1.Namespace,
	k8sClientFactory SRLK8sClientFactory,
	dbQueries db.DatabaseQueries,
	user db.ClusterUser,
	log logr.Logger) error {

	apiCRToDBMapping := []db.APICRToDatabaseMapping{}

	// 1) Locate all managed environments resources that have used this name in this namespace
	if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx,
		db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
		managedEnvironmentCRName, managedEnvironmentCRNamespace, string(workspaceNamespace.UID),
		db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment, &apiCRToDBMapping); err != nil {

		return fmt.Errorf("unable to list API CR to database mappings for name '%s' and namespace '%s': %v",
			managedEnvironmentCRName, managedEnvironmentCRNamespace, err)
	}

	// 2) For each of the old managed environments, clean up their database entries
	for idx := range apiCRToDBMapping {
		mapping := apiCRToDBMapping[idx]

		// If skipResourceWithK8sUID is defined, then don't delete any managed environment CRs with that UID
		if skipResourceWithK8sUID != "" && mapping.APIResourceUID == skipResourceWithK8sUID {
			continue
		}

		// 2a) Locate the managed environment DB row, if it exists
		managedEnv := &db.ManagedEnvironment{Managedenvironment_id: mapping.DBRelationKey}
		if err := dbQueries.GetManagedEnvironmentById(ctx, managedEnv); err != nil {
			if !db.IsResultNotFoundError(err) {
				return fmt.Errorf("unable to retrieve managed environment: %v", err)
			}
			// If the managed environment can't be found, there is no other work to do, so just continue.
		} else {
			// Delete AppProjectManagedEnvironment before deleting ManagedEnvironment, as it contains a foreign key managedenv id.
			// 2b) Whether or not the managed environment DB row exists, clean up all related database entries
			if err := DeleteManagedEnvironmentResources(ctx, managedEnv.Managedenvironment_id, managedEnv, user, k8sClientFactory, dbQueries, log); err != nil {
				return fmt.Errorf("unable to delete managed environment row '%s': %v", managedEnv.Managedenvironment_id, err)
			}
		}

		// 2c) On successful cleanup of managed env, clean up the APICRToDatabaseMapping for the managed env
		if _, err := dbQueries.DeleteAPICRToDatabaseMapping(ctx, &mapping); err != nil {
			log.Error(err, "Unable to delete APICRToDatabaseMapping", mapping.GetAsLogKeyValues()...)
			return fmt.Errorf("unable to delete api cr to database mapping: %v", err)
		}
		log.Info("Deleted APICRToDatabaseMapping", mapping.GetAsLogKeyValues()...)
	}

	return nil
}

// replaceExistingManagedEnv updates an existing managed environment by creating new credentials, updating the
// managed environment to point to them, then deleting the old credentials.
func replaceExistingManagedEnv(ctx context.Context,
	gitopsEngineClient client.Client,
	workspaceClient client.Client,
	clusterUser db.ClusterUser, isNewUser bool,
	managedEnvironmentCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment,
	secret corev1.Secret,
	managedEnvironmentDB db.ManagedEnvironment,
	workspaceNamespace corev1.Namespace,
	k8sClientFactory SRLK8sClientFactory,
	dbQueries db.DatabaseQueries,
	log logr.Logger) (SharedResourceManagedEnvContainer, connectionInitializedCondition, error) {

	oldClusterCredentialsPrimaryKey := managedEnvironmentDB.Clustercredentials_id

	// 1) Create new cluster creds, based on secret
	clusterCredentials, connInitCondition, err := createNewClusterCredentials(ctx, managedEnvironmentCR, secret, k8sClientFactory, dbQueries, log, workspaceClient)
	if err != nil {
		return SharedResourceManagedEnvContainer{}, connInitCondition,
			fmt.Errorf("unable to create new cluster credentials for managed env, while replacing existing managed env: %v", err)
	}

	// 2) Update the existing managed environment to point to the new credentials
	managedEnvironmentDB.Clustercredentials_id = clusterCredentials.Clustercredentials_cred_id

	if err := dbQueries.UpdateManagedEnvironment(ctx, &managedEnvironmentDB); err != nil {
		log.Error(err, "Unable to update ManagedEnvironment with new cluster credentials ID", managedEnvironmentDB.GetAsLogKeyValues()...)

		return SharedResourceManagedEnvContainer{},
			createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
			fmt.Errorf("unable to update managed environment with new credentials: %w", err)
	}
	log.Info("Updated ManagedEnvironment with new cluster credentials ID", managedEnvironmentDB.GetAsLogKeyValues()...)

	appProjectManagedEnv := db.AppProjectManagedEnvironment{
		Managed_environment_id: managedEnvironmentDB.Managedenvironment_id,
	}

	// Whenever a ManagedEnvironment is updated, check whether the corresponding AppProjectManagedEnvironment has been created. If it has not been created, create it.
	if err := dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnv); err != nil {
		log.Error(err, "Unable to retrieve appProjectManagedEnv", appProjectManagedEnv.GetAsLogKeyValues()...)

		// If AppProjectManagedEnvironment is not present in DB, create it.
		if db.IsResultNotFoundError(err) {
			appProjectManagedEnv := db.AppProjectManagedEnvironment{
				Clusteruser_id:         clusterUser.Clusteruser_id,
				Managed_environment_id: managedEnvironmentDB.Managedenvironment_id,
			}

			if err := dbQueries.CreateAppProjectManagedEnvironment(ctx, &appProjectManagedEnv); err != nil {
				log.Error(err, "Unable to create appProjectManagedEnv", appProjectManagedEnv.GetAsLogKeyValues()...)

				return SharedResourceManagedEnvContainer{},
					createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
					fmt.Errorf("unable to call CreateAppProjectManagedEnvironment: %w", err)
			}

			log.Info("Created new AppProjectManagedEnvironment in DB : "+appProjectManagedEnv.Managed_environment_id, appProjectManagedEnv.GetAsLogKeyValues()...)
		} else {

			return SharedResourceManagedEnvContainer{}, connInitCondition,
				fmt.Errorf("unable to call GetAppProjectManagedEnvironmentById: %v", err)
		}
	}

	// 4) Delete the old credentials
	rowsDeleted, err := dbQueries.DeleteClusterCredentialsById(ctx, oldClusterCredentialsPrimaryKey)
	if err != nil {
		log.Error(err, "Unable to delete old ClusterCredentials row which is no longer used by ManagedEnv", "clusterCredentials", oldClusterCredentialsPrimaryKey)

		return SharedResourceManagedEnvContainer{},
			createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR),
			fmt.Errorf("unable to delete old cluster credentials '%s': %w", oldClusterCredentialsPrimaryKey, err)
	}
	if rowsDeleted != 1 {
		log.V(logutil.LogLevel_Warn).Info("unexpected number of rows deleted when deleting cluster credentials",
			"clusterCredentialsID", oldClusterCredentialsPrimaryKey)
		return SharedResourceManagedEnvContainer{}, createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR), nil
	}
	log.Info("Deleted old ClusterCredentials row which is no longer used by ManagedEnv", "clusterCredentials", oldClusterCredentialsPrimaryKey)

	// 5) Retrieve/create the other env vars for the managed env, and return
	engineInstance, isNewEngineInstance, clusterAccess,
		isNewClusterAccess, engineCluster, uerr := wrapManagedEnv(ctx,
		managedEnvironmentDB, workspaceNamespace, clusterUser, gitopsEngineClient, dbQueries, log)

	if uerr != nil {
		return newSharedResourceManagedEnvContainer(),
			convertConditionErrorToConnInitCondition(uerr, managedEnvironmentCR),
			fmt.Errorf("unable to wrap managed environment for %s: %w", managedEnvironmentCR.UID, uerr.DevError())
	}

	res := SharedResourceManagedEnvContainer{
		ClusterUser:          &clusterUser,
		IsNewUser:            isNewUser,
		ManagedEnv:           &managedEnvironmentDB,
		IsNewManagedEnv:      true,
		GitopsEngineInstance: engineInstance,
		IsNewInstance:        isNewEngineInstance,
		ClusterAccess:        clusterAccess,
		IsNewClusterAccess:   isNewClusterAccess,
		GitopsEngineCluster:  engineCluster,
	}

	return res, createSuccessEnvInitCondition(managedEnvironmentCR), nil
}

// constructNewManagedEnv creates a new ManagedEnvironment using the provided parameters, then creates ClusterAccess/GitOpsEngineInstance,
// and returns those all created resources in a SharedResourceContainer
func constructNewManagedEnv(ctx context.Context,
	gitopsEngineClient client.Client,
	workspaceClient client.Client,
	clusterUser db.ClusterUser,
	isNewUser bool,
	managedEnvironment managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment,
	secret corev1.Secret,
	workspaceNamespace corev1.Namespace,
	k8sClientFactory SRLK8sClientFactory,
	dbQueries db.DatabaseQueries,
	log logr.Logger) (SharedResourceManagedEnvContainer, connectionInitializedCondition, error) {

	managedEnvDB, connInitErr, err := createNewManagedEnv(ctx, managedEnvironment, secret, clusterUser, workspaceNamespace, k8sClientFactory, dbQueries, log, workspaceClient)
	if err != nil {
		return newSharedResourceManagedEnvContainer(), connInitErr,
			fmt.Errorf("unable to create managed environment for %s: %w", managedEnvironment.UID, err)
	}

	// Whenever a ManagedEnvironment is created, the corresponding AppProjectManagedEnvironment should also be created
	appProjectManagedEnvDB := db.AppProjectManagedEnvironment{
		Clusteruser_id:         clusterUser.Clusteruser_id,
		Managed_environment_id: managedEnvDB.Managedenvironment_id,
	}

	if err := dbQueries.CreateAppProjectManagedEnvironment(ctx, &appProjectManagedEnvDB); err != nil {
		log.Error(err, "Unable to create new AppProjectManagedEnvironment")
		return newSharedResourceManagedEnvContainer(), connInitErr,
			fmt.Errorf("unable to create AppProjectManagedEnvironment for %s: %w", appProjectManagedEnvDB.Clusteruser_id, err)
	}

	log.Info("Created new AppProjectManagedEnvironment")

	engineInstance, isNewEngineInstance, clusterAccess,
		isNewClusterAccess, engineCluster, uerr := wrapManagedEnv(ctx,
		*managedEnvDB, workspaceNamespace, clusterUser, gitopsEngineClient, dbQueries, log)

	if uerr != nil {
		return newSharedResourceManagedEnvContainer(),
			convertConditionErrorToConnInitCondition(uerr, managedEnvironment),
			fmt.Errorf("unable to wrap managed environment for %s: %w", managedEnvironment.UID, uerr.DevError())
	}

	res := SharedResourceManagedEnvContainer{
		ClusterUser:          &clusterUser,
		IsNewUser:            isNewUser,
		ManagedEnv:           managedEnvDB,
		IsNewManagedEnv:      true,
		GitopsEngineInstance: engineInstance,
		IsNewInstance:        isNewEngineInstance,
		ClusterAccess:        clusterAccess,
		IsNewClusterAccess:   isNewClusterAccess,
		GitopsEngineCluster:  engineCluster,
	}

	return res, createSuccessEnvInitCondition(managedEnvironment), nil
}

// wrapManagedEnv creates (or gets) a GitOpsEngineInstance, GitOpsEngineCluster, and ClusterAccess, for the provided 'managedEnv' param
func wrapManagedEnv(ctx context.Context, managedEnv db.ManagedEnvironment, workspaceNamespace corev1.Namespace,
	clusterUser db.ClusterUser, gitopsEngineClient client.Client, dbQueries db.DatabaseQueries, log logr.Logger) (*db.GitopsEngineInstance,
	bool, *db.ClusterAccess, bool, *db.GitopsEngineCluster, gitopserrors.ConditionError) {

	engineInstance, isNewInstance, gitopsEngineCluster, err :=
		internalDetermineGitOpsEngineInstance(ctx, clusterUser, gitopsEngineClient, dbQueries, log)

	if err != nil {
		log.Error(err.DevError(), "unable to determine gitops engine instance")
		return nil, false, nil, false, nil, err
	}

	// Create the cluster access object, to allow us to interact with the GitOpsEngine and ManagedEnvironment on the user's behalf
	ca := db.ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnv.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: engineInstance.Gitopsengineinstance_id,
	}

	isNewClusterAccess, err1 := internalGetOrCreateClusterAccess(ctx, &ca, dbQueries, log)
	if err1 != nil {
		log.Error(err1, "unable to create cluster access")
		msg := gitopserrors.UnknownError
		return nil, false, nil, false, nil, gitopserrors.NewUserConditionError(msg, err1, string(managedgitopsv1alpha1.ConditionReasonDatabaseError))
	}

	return engineInstance,
		isNewInstance,
		&ca,
		isNewClusterAccess,
		gitopsEngineCluster,
		nil

}

func createNewManagedEnv(ctx context.Context, managedEnvironment managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment,
	secret corev1.Secret, clusterUser db.ClusterUser, workspaceNamespace corev1.Namespace,
	k8sClientFactory SRLK8sClientFactory, dbQueries db.DatabaseQueries, log logr.Logger,
	workspaceClient client.Client) (*db.ManagedEnvironment, connectionInitializedCondition, error) {

	clusterCredentials, connInitCondition, err := createNewClusterCredentials(ctx, managedEnvironment, secret, k8sClientFactory, dbQueries, log, workspaceClient)
	if err != nil {
		return nil, connInitCondition,
			fmt.Errorf("unable to create new cluster credentials for managed env, while creating new managed env: %w", err)
	}

	managedEnv := &db.ManagedEnvironment{
		Name:                  managedEnvironment.Name,
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
	}

	if err := dbQueries.CreateManagedEnvironment(ctx, managedEnv); err != nil {
		log.Error(err, "Unable to create new ManagedEnvironment", managedEnv.GetAsLogKeyValues()...)
		return nil, createGenericDatabaseErrorEnvInitCondition(managedEnvironment),
			fmt.Errorf("unable to create managed environment for env obj '%s': %w", managedEnvironment.UID, err)
	}
	log.Info("Created new ManagedEnvironment", managedEnv.GetAsLogKeyValues()...)

	apiCRToDBMapping := &db.APICRToDatabaseMapping{
		APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
		APIResourceUID:       string(managedEnvironment.UID),
		APIResourceName:      managedEnvironment.Name,
		APIResourceNamespace: managedEnvironment.Namespace,
		NamespaceUID:         string(workspaceNamespace.UID),
		DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
		DBRelationKey:        managedEnv.Managedenvironment_id,
	}
	if err := dbQueries.CreateAPICRToDatabaseMapping(ctx, apiCRToDBMapping); err != nil {
		log.Error(err, "Unable to create new APICRToDatabaseMapping", apiCRToDBMapping.GetAsLogKeyValues()...)
		err := fmt.Errorf("unable to create APICRToDatabaseMapping for managed environment: %w", err)
		return nil, createGenericDatabaseErrorEnvInitCondition(managedEnvironment), err
	}
	log.Info("Created new APICRToDatabaseMapping", apiCRToDBMapping.GetAsLogKeyValues()...)

	return managedEnv, createSuccessEnvInitCondition(managedEnvironment), err
}

func DeleteManagedEnvironmentResources(ctx context.Context, managedEnvID string, managedEnvCR *db.ManagedEnvironment, user db.ClusterUser,
	k8sClientFactory SRLK8sClientFactory, dbQueries db.DatabaseQueries, log logr.Logger) error {

	log = log.WithValues("managedEnvID", managedEnvID)

	// 1) Retrieve all the Applications that reference this ManagedEnvironment
	applications := []db.Application{}

	log.Info("niling the values of Application rows that reference deleted ManagedEnvironment")
	if _, err := dbQueries.RemoveManagedEnvironmentFromAllApplications(ctx, managedEnvID, &applications); err != nil {
		return fmt.Errorf("unable to list applications for managed environment '%s': %v", managedEnvID, err)
	}

	// gitopsEngineInstances is a hash set of all the gitops engine instances that referenced the managed environment
	// key: gitops engine instance uid -> engine instance k8s object
	gitopsEngineInstances := map[string]db.GitopsEngineInstance{}

	// 2) For each application, nil the managed environment field, then create an operation to instruct the cluster-agent to update the Application
	for idx := range applications {
		app := applications[idx]

		log := log.WithValues("applicationID", app.Application_id)

		gitopsEngineInstance := &db.GitopsEngineInstance{
			Gitopsengineinstance_id: app.Engine_instance_inst_id,
		}
		if err := dbQueries.GetGitopsEngineInstanceById(ctx, gitopsEngineInstance); err != nil {
			return fmt.Errorf("unable to retrieve gitopsengineinstance '%s' while deleting managed environment '%s': %v",
				gitopsEngineInstance.Gitopsengineinstance_id, managedEnvID, err)
		}

		// Add the gitops engine instance key to the map
		gitopsEngineInstances[app.Engine_instance_inst_id] = *gitopsEngineInstance

		client, err := k8sClientFactory.GetK8sClientForGitOpsEngineInstance(ctx, gitopsEngineInstance)
		if err != nil {
			return fmt.Errorf("unable to retrieve k8s client for engine instance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
		}

		operation := db.Operation{
			Instance_id:             app.Engine_instance_inst_id,
			Operation_owner_user_id: user.Clusteruser_id,
			Resource_type:           db.OperationResourceType_Application,
			Resource_id:             app.Application_id,
		}

		log.Info("Creating operation for updated application, of deleted managed environment")

		// Don't wait for the Operation to complete, just create it and continue with the next.
		_, _, err = operations.CreateOperation(ctx, false, operation, user.Clusteruser_id,
			gitopsEngineInstance.Namespace_name, dbQueries, client, log)
		// TODO: GITOPSRVCE-174 - Add garbage collection of this operation once 174 is finished.
		if err != nil {
			return fmt.Errorf("unable to create operation for applicaton '%s': %v", app.Application_id, err)
		}
	}

	// 3) Delete all cluster accesses that reference this managed env
	clusterAccesses := []db.ClusterAccess{}
	if err := dbQueries.ListClusterAccessesByManagedEnvironmentID(ctx, managedEnvID, &clusterAccesses); err != nil {
		// We exit here, because if this doesn't succeed, we won't be able to do any of the other next steps, due to database foreign keys
		return fmt.Errorf("unable to list cluster accesses by managed id '%s': %v", managedEnvID, err)
	}
	for idx := range clusterAccesses {
		clusterAccess := clusterAccesses[idx]

		rowsDeleted, err := dbQueries.DeleteClusterAccessById(ctx, clusterAccess.Clusteraccess_user_id, managedEnvID, clusterAccess.Clusteraccess_gitops_engine_instance_id)
		if err != nil {
			log.Error(err, "Unable to delete ClusterAccess row that referenced to ManagedEnvironment", "userID", clusterAccess.Clusteraccess_user_id, "gitopsEngineInstanceID", clusterAccess.Clusteraccess_gitops_engine_instance_id)
			return fmt.Errorf("unable to delete cluster access while deleting managed environment '%s': %v", clusterAccess.Clusteraccess_managed_environment_id, err)
		}
		if rowsDeleted != 1 {
			// It's POSSIBLE (but unlikely) the cluster access was deleted while this for loop was in progress, so it's a WARN.
			log.V(logutil.LogLevel_Warn).Info("Unexpected number of cluster accesses rows deleted, when deleting managed env.",
				"rowsDeleted", rowsDeleted, "cluster-access", clusterAccess)
		}
		log.Info("Deleted ClusterAccess row that referenced to ManagedEnvironment", "userID", clusterAccess.Clusteraccess_user_id, "gitopsEngineInstanceID", clusterAccess.Clusteraccess_gitops_engine_instance_id)

	}

	// 4) Delete all appProjectManagedEnvs that reference this managed env
	appProjectManagedEnvs := []db.AppProjectManagedEnvironment{}
	if err := dbQueries.ListAppProjectManagedEnvironmentByClusterUserId(ctx, user.Clusteruser_id, &appProjectManagedEnvs); err != nil {
		// We exit here, because if this doesn't succeed, we won't be able to do any of the other next steps, due to database foreign keys
		return fmt.Errorf("unable to list appProjectManagedEnvs by cluster user id '%s': %v", managedEnvID, err)
	}

	for idx, a := range appProjectManagedEnvs {
		if a.Managed_environment_id == managedEnvID {
			appProjectManagedEnv := appProjectManagedEnvs[idx]

			rowsDeleted, err := dbQueries.DeleteAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnv)
			if err != nil {
				log.Error(err, "Unable to delete appProjectManagedEnv row that referenced to ManagedEnvironment", "managedEnvID", appProjectManagedEnv.Managed_environment_id)
				return fmt.Errorf("unable to delete appProjectManagedEnv row that referenced to ManagedEnvironment '%s': %v", appProjectManagedEnv.Managed_environment_id, err)
			}
			if rowsDeleted != 1 {
				log.V(logutil.LogLevel_Warn).Info("Unexpected number of appProjectManagedEnvs rows deleted, when deleting managed env.",
					"rowsDeleted", rowsDeleted, "appProject-ManagedEnv", appProjectManagedEnv)
			} else {
				log.Info("Deleted appProjectManagedEnv row that referenced to ManagedEnvironment", "userID", appProjectManagedEnv.Managed_environment_id)
			}
			break
		}
	}

	// 5) Delete the ManagedEnvironment entry
	rowsDeleted, err := dbQueries.DeleteManagedEnvironmentById(ctx, managedEnvID)
	if err != nil || rowsDeleted != 1 {
		log.Error(err, "Unable to delete ManagedEnvironment row")
		return fmt.Errorf("unable to deleted managed environment '%s': %v (%v)", managedEnvID, err, rowsDeleted)
	}
	log.Info("Deleted ManagedEnvironment row")

	// 6) Delete the cluster credentials row of the managed environment
	if managedEnvCR != nil {
		log := log.WithValues("clusterCredentialsId", managedEnvCR.Clustercredentials_id)

		rowsDeleted, err = dbQueries.DeleteClusterCredentialsById(ctx, managedEnvCR.Clustercredentials_id)
		if err != nil || rowsDeleted != 1 {
			log.Error(err, "Unable to delete ClusterCredentials of the managed environment")
			return fmt.Errorf("unable to delete cluster credentials '%s' for managed environment: %v (%v)", managedEnvCR.Clustercredentials_id, err, rowsDeleted)
		}
		log.Info("Deleted ClusterCredentials of the managed environment")
	}

	// 7) For each Argo CD instances that was involved, create a new Operation to delete the managed environment
	//    (the Argo CD Cluster Secret) of that Argo CD instance.
	for idx := range gitopsEngineInstances {

		gitopsEngineInstance := gitopsEngineInstances[idx]

		client, err := k8sClientFactory.GetK8sClientForGitOpsEngineInstance(ctx, &gitopsEngineInstance)
		if err != nil {
			return fmt.Errorf("unable to retrieve k8s client for engine instance '%s': %v", gitopsEngineInstance.Gitopsengineinstance_id, err)
		}

		// Create an operation, pointing to the managed environment: but note, the managed environment database entry won't exist
		// when it processed by cluster agent. This is intentional.
		operation := db.Operation{
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Operation_owner_user_id: user.Clusteruser_id,
			Resource_type:           db.OperationResourceType_ManagedEnvironment,
			Resource_id:             managedEnvID,
		}

		log.Info("Creating Operation to delete Argo CD cluster secret, referencing managed environment")

		// TODO: GITOPSRVCE-174 - Add garbage collection of this operation once 174 is finished.
		_, _, err = operations.CreateOperation(ctx, false, operation, user.Clusteruser_id,
			gitopsEngineInstance.Namespace_name, dbQueries, client, log)
		if err != nil {
			return fmt.Errorf("unable to create operation for deleted managed environment: %v", err)
		}
	}

	// Add a field to the database operation: GC after datetime, after which the operation is deleted if completed.

	return nil
}

// SRLK8sClientFactory abstracts out the creation of client.Client, which allows mocking by unit tests.
type SRLK8sClientFactory interface {

	// Create a client.Client using the given restconfig
	BuildK8sClient(restConfig *rest.Config) (client.Client, error)

	// Create a client.Client which can access the cluster that Argo CD is on
	GetK8sClientForGitOpsEngineInstance(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error)

	// Create a client.Client which can access the cluster where GitOps Service is running
	GetK8sClientForServiceWorkspace() (client.Client, error)
}

var _ SRLK8sClientFactory = DefaultK8sClientFactory{}

// DefaultK8sClientFactory should always be used, except when mocking for unit tests.
type DefaultK8sClientFactory struct {
}

func (DefaultK8sClientFactory) GetK8sClientForGitOpsEngineInstance(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
	return eventlooptypes.GetK8sClientForGitOpsEngineInstance(ctx, gitopsEngineInstance)
}

func (DefaultK8sClientFactory) GetK8sClientForServiceWorkspace() (client.Client, error) {
	return eventlooptypes.GetK8sClientForServiceWorkspace()
}

func (DefaultK8sClientFactory) BuildK8sClient(restConfig *rest.Config) (client.Client, error) {
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	k8sClient = sharedutil.IfEnabledSimulateUnreliableClient(k8sClient)
	if err != nil {
		return nil, err
	}

	return k8sClient, err

}

func createNewClusterCredentials(ctx context.Context, managedEnvironment managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment,
	secret corev1.Secret, k8sClientFactory SRLK8sClientFactory, dbQueries db.DatabaseQueries, log logr.Logger,
	workspaceClient client.Client) (db.ClusterCredentials, connectionInitializedCondition, error) {

	if secret.Type != sharedutil.ManagedEnvironmentSecretType {
		err := fmt.Errorf("invalid secret type: %s", secret.Type)
		return db.ClusterCredentials{},
			convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonInvalidSecretType, err, managedEnvironment),
			err
	}

	kubeconfig, exists := secret.Data[KubeconfigKey]
	if !exists {
		err := fmt.Errorf("missing %s field in Secret", KubeconfigKey)

		return db.ClusterCredentials{},
			convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonMissingKubeConfigField, err, managedEnvironment),
			err
	}

	// Load the kubeconfig from the field
	config, err := clientcmd.Load(kubeconfig)
	if err != nil {
		err := fmt.Errorf("unable to parse kubeconfig data: %w", err)

		return db.ClusterCredentials{},
			convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonUnableToParseKubeconfigData, err, managedEnvironment),
			err

	}

	matchingContextName, matchingContext, err := locateContextThatMatchesAPIURL(config, managedEnvironment.Spec.APIURL)
	if err != nil {
		return db.ClusterCredentials{},
			convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonUnableToLocateContext, err, managedEnvironment),
			err
	}

	clientConfig := clientcmd.NewNonInteractiveClientConfig(*config, matchingContextName, &clientcmd.ConfigOverrides{}, nil)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		err := fmt.Errorf("unable to retrieve restConfig from managed environment secret: %w", err)

		return db.ClusterCredentials{},
			convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonUnableToRetrieveRestConfig, err, managedEnvironment),
			err
	}

	// Ignore the self-signed certificate
	if managedEnvironment.Spec.AllowInsecureSkipTLSVerify {
		restConfig.Insecure = true
	}

	k8sClient, err := k8sClientFactory.BuildK8sClient(restConfig)
	if err != nil {
		err := fmt.Errorf("unable to create k8s client from restConfig from managed environment secret: %w", err)

		return db.ClusterCredentials{},
			convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonUnableToCreateClient, err, managedEnvironment),
			err
	}

	var saBearerToken string
	log.Info("createNewServiceAccount is ", "CreateNewServiceAccount", managedEnvironment.Spec.CreateNewServiceAccount)
	if managedEnvironment.Spec.CreateNewServiceAccount {
		// This is the original behaviour, where we create a new service account
		saBearerToken, _, err = sharedutil.InstallServiceAccount(ctx, k8sClient, string(managedEnvironment.UID), serviceAccountNamespaceKubeSystem, log)
		if err != nil {
			err2 := fmt.Errorf("unable to install service account from secret '%s': %w", secret.Name, err)

			return db.ClusterCredentials{},
				convertErrToEnvInitCondition(managedgitopsv1alpha1.ConditionReasonUnableToInstallServiceAccount, err, managedEnvironment),
				err2

		}
	} else {
		// If an existing service account is used instead, we just simply take the provided secret as the token for the cluster credentials

		val, exists := config.AuthInfos[matchingContext.AuthInfo]

		if !exists {
			msg := "unable to extract remote cluster configuration from kubeconfig, missing auth info for " + matchingContextName
			return db.ClusterCredentials{}, connectionInitializedCondition{
				managedEnvCR: managedEnvironment,
				status:       metav1.ConditionFalse,
				reason:       managedgitopsv1alpha1.ConditionReasonUnableToParseKubeconfigData,
				message:      msg,
			}, fmt.Errorf("%s", msg)
		}

		if val.Token == "" {
			msg := fmt.Sprintf("kubeconfig must have a service account token for the user in context \"%s\". client-certificate is not supported at this time.", matchingContextName)
			return db.ClusterCredentials{}, connectionInitializedCondition{
				managedEnvCR: managedEnvironment,
				status:       metav1.ConditionFalse,
				reason:       managedgitopsv1alpha1.ConditionReasonUnableToParseKubeconfigData,
				message:      msg,
			}, fmt.Errorf("%s", msg)
		}

		saBearerToken = val.Token
	}

	// Convert the .spec.namespaces field to a comma-separated list of namespaces
	var namespacesField string
	if len(managedEnvironment.Spec.Namespaces) > 0 {

		namespacesField, err = convertManagedEnvNamespacesFieldToCommaSeparatedList(managedEnvironment.Spec.Namespaces)
		if err != nil {
			log.Error(err, "ManagedEnvironment contains an invalid namespace slice", "namespaceSlice", managedEnvironment.Spec.Namespaces)

			return db.ClusterCredentials{},
				connectionInitializedCondition{
					managedEnvCR: managedEnvironment,
					status:       metav1.ConditionUnknown,
					reason:       managedgitopsv1alpha1.ConditionReasonInvalidNamespaceList,
					message:      err.Error(),
				}, fmt.Errorf("user specified an invalid namespace: %v", err)
		}

	}

	insecureVerifyTLS := managedEnvironment.Spec.AllowInsecureSkipTLSVerify
	clusterCredentials := db.ClusterCredentials{
		Host:                        managedEnvironment.Spec.APIURL,
		Kube_config:                 "",
		Kube_config_context:         "",
		Serviceaccount_bearer_token: saBearerToken,
		Serviceaccount_ns:           serviceAccountNamespaceKubeSystem,
		AllowInsecureSkipTLSVerify:  insecureVerifyTLS,
		Namespaces:                  namespacesField,
		ClusterResources:            managedEnvironment.Spec.ClusterResources,
	}
	// If an existing service account is used instead, we should verify the cluster credentials based on the provided token
	if !managedEnvironment.Spec.CreateNewServiceAccount {
		validClusterCreds, err := verifyClusterCredentialsWithNamespaceList(ctx, clusterCredentials, managedEnvironment, k8sClientFactory)

		if !validClusterCreds || err != nil {

			log.Error(err, "Unable to verify ClusterCredentials using provided token", clusterCredentials.GetAsLogKeyValues()...)

			message := "Unable to validate the credentials provided in the ManagedEnvironment Secret. Verify the API URL, and service account token are correct."
			if apierr.IsForbidden(err) {
				message = "Provided service account does not have permission to access resources in the cluster. Verify that the service account has the correct Role and RoleBinding."
			} else if isCertificateSignedByUnknownAuthority(err) {
				message = "Certificate signed by unknown authority. Note that the '.spec.allowInsecureSkipTLSVerify' field can be used to ignore this error."
			}
			return db.ClusterCredentials{}, connectionInitializedCondition{
				managedEnvCR: managedEnvironment,
				status:       metav1.ConditionUnknown,
				reason:       managedgitopsv1alpha1.ConditionReasonUnableToValidateClusterCredentials,
				message:      message,
			}, fmt.Errorf("unable to create cluster credentials for host '%s': %w", clusterCredentials.Host, err)

		}
	}

	if err := dbQueries.CreateClusterCredentials(ctx, &clusterCredentials); err != nil {
		log.Error(err, "Unable to create ClusterCredentials for ManagedEnvironment", clusterCredentials.GetAsLogKeyValues()...)

		return db.ClusterCredentials{}, connectionInitializedCondition{
			managedEnvCR: managedEnvironment,
			status:       metav1.ConditionUnknown,
			reason:       managedgitopsv1alpha1.ConditionReasonUnableToCreateClusterCredentials,
			message:      gitopserrors.UnknownError,
		}, fmt.Errorf("unable to create cluster credentials for host '%s': %w", clusterCredentials.Host, err)

	}
	log.Info("Created ClusterCredentials for ManagedEnvironment", clusterCredentials.GetAsLogKeyValues()...)

	return clusterCredentials, createSuccessEnvInitCondition(managedEnvironment), nil

}

// locateContextThatMatchesAPIURL examines a kubeconfig (Config struct), and looks for the context that
// matches the cluster with the given API URL.
// See 'sharedresourceloop_managedend_test.go' for an example of a kubeconfig.
func locateContextThatMatchesAPIURL(config *clientcmdapi.Config, apiURL string) (string, clientcmdapi.Context, error) {
	var matchingClusterName string

	// Look for the cluster with the given API URL
	for clusterName := range config.Clusters {
		cluster := config.Clusters[clusterName]
		if strings.EqualFold(cluster.Server, apiURL) {
			matchingClusterName = clusterName
			// matchingCluster = cluster
			break
		}
	}
	if matchingClusterName == "" {
		return "", clientcmdapi.Context{}, fmt.Errorf("the kubeconfig did not have a cluster entry that matched the API URL '%s", apiURL)
	}

	// Look for the context that matches the cluster above
	var matchingContextName string
	var matchingContext *clientcmdapi.Context
	for contextName := range config.Contexts {
		context := config.Contexts[contextName]
		if context.Cluster == matchingClusterName {
			matchingContextName = contextName
			matchingContext = context
		}
	}
	if matchingContextName == "" {
		return "", clientcmdapi.Context{}, fmt.Errorf("the kubeconfig did not have a context that matched "+
			"the cluster specified in the API URL of the GitOpsDeploymentManagedEnvironment. Context "+
			"was expected to reference cluster '%s'", matchingClusterName)
	}

	return matchingContextName, *matchingContext, nil
}

func isCertificateSignedByUnknownAuthority(err error) bool {
	if err == nil {
		return false
	}
	var cause error
	for err != nil {
		cause = err
		err = errors.Unwrap(err)
	}
	return strings.HasSuffix(cause.Error(), "x509: certificate signed by unknown authority")
}

// sanityTestCredentials returns true if we were able to successfully connect with the credentials, false otherwise.
func sanityTestCredentials(clusterCreds db.ClusterCredentials) (*rest.Config, bool, error) {

	// Sanity test the fields we are using in rest.Config
	if clusterCreds.Host == "" {
		return nil, false, fmt.Errorf("cluster credentials is missing host")
	}
	if clusterCreds.Serviceaccount_bearer_token == "" {
		return nil, false, fmt.Errorf("cluster credentials is missing service account bearer token")
	}

	configParam := &rest.Config{
		Host:        clusterCreds.Host,
		BearerToken: clusterCreds.Serviceaccount_bearer_token,
	}

	configParam.Insecure = clusterCreds.AllowInsecureSkipTLSVerify

	configParam.ServerName = ""

	return configParam, true, nil
}

// connectionInitializedCondition is returned by functions in this file, to indicate that a Condition should be set in the
// .status.conditions field of the ManagedEnvironment CR, of type ManagedEnvironmentStatusConnectionInitializationSucceeded.
type connectionInitializedCondition struct {
	// managedEnvCR is the API Resource we need to update the condition on
	managedEnvCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment

	// type is ManagedEnvironmentStatusConnectionInitializationSucceeded
	status  metav1.ConditionStatus
	reason  managedgitopsv1alpha1.ManagedEnvironmentConditionReason
	message string
}

func convertErrToEnvInitCondition(reason managedgitopsv1alpha1.ManagedEnvironmentConditionReason, err error, managedEnvironment managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) connectionInitializedCondition {

	errMsg := gitopserrors.UnknownError

	if err != nil {
		errMsg = err.Error()
	}

	return connectionInitializedCondition{
		managedEnvCR: managedEnvironment,
		status:       metav1.ConditionFalse,
		reason:       reason,
		message:      errMsg,
	}

}

func convertConditionErrorToConnInitCondition(conditionError gitopserrors.ConditionError, managedEnvironmentCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) connectionInitializedCondition {

	msg := gitopserrors.UnknownError
	reason := managedgitopsv1alpha1.ConditionReasonUnknownError
	if conditionError != nil {
		msg = conditionError.UserError()
		reason = managedgitopsv1alpha1.ManagedEnvironmentConditionReason(conditionError.ConditionReason())
	} else {
		// This shouldn't happen under any circumstances, so sanity test and log as severe
		controllerLog.FromContext(context.Background()).Error(nil, "SEVERE: nil error passed to convertConditionErrorToConnInitCondition")
	}

	return connectionInitializedCondition{
		managedEnvCR: managedEnvironmentCR,
		status:       metav1.ConditionFalse,
		reason:       reason,
		message:      msg,
	}
}

func createSuccessEnvInitCondition(managedEnvironmentCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) connectionInitializedCondition {

	return connectionInitializedCondition{
		managedEnvCR: managedEnvironmentCR,
		status:       metav1.ConditionTrue,
		reason:       managedgitopsv1alpha1.ConditionReasonSucceeded,
		message:      "",
	}
}

func createGenericDatabaseErrorEnvInitCondition(managedEnvironmentCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) connectionInitializedCondition {
	return connectionInitializedCondition{
		managedEnvCR: managedEnvironmentCR,
		status:       metav1.ConditionUnknown,
		reason:       managedgitopsv1alpha1.ConditionReasonDatabaseError,
		message:      gitopserrors.UnknownError,
	}
}

func createUnknownErrorEnvInitCondition() connectionInitializedCondition {
	return connectionInitializedCondition{
		managedEnvCR: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{},
		status:       metav1.ConditionUnknown,
		reason:       managedgitopsv1alpha1.ConditionReasonUnknownError,
		message:      gitopserrors.UnknownError,
	}
}

// Updates the given managed environment's connection status condition to match the given status, reason and message.
// If there is an existing status condition with the exact same status, reason and message, no update is made in order
// to preserve the LastTransitionTime (see https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition.LastTransitionTime )
func updateManagedEnvironmentConnectionStatus(ctx context.Context,
	managedEnvironment managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment,
	client client.Client, connInitCondition connectionInitializedCondition, log logr.Logger) {

	const conditionType = managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded
	var condition *metav1.Condition = nil
	for i := range managedEnvironment.Status.Conditions {
		if managedEnvironment.Status.Conditions[i].Type == conditionType {
			condition = &managedEnvironment.Status.Conditions[i]
			break
		}
	}
	if condition == nil {
		managedEnvironment.Status.Conditions = append(managedEnvironment.Status.Conditions, metav1.Condition{Type: conditionType})
		condition = &managedEnvironment.Status.Conditions[len(managedEnvironment.Status.Conditions)-1]
	}
	if condition.Reason != string(connInitCondition.reason) || condition.Message != connInitCondition.message ||
		condition.Status != connInitCondition.status {

		condition.Reason = string(connInitCondition.reason)
		condition.Message = connInitCondition.message
		condition.LastTransitionTime = metav1.Now()
		condition.Status = connInitCondition.status
		if err := client.Status().Update(ctx, &managedEnvironment); err != nil {
			log.Error(err, "updating managed environment status condition")
		}
	}
}

// verifyClusterCredentialsWithNamespaceList returns true if we were able to successfully connect with the credentials, false otherwise.
func verifyClusterCredentialsWithNamespaceList(ctx context.Context, clusterCreds db.ClusterCredentials, managedEnvCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment,
	k8sClientFactory SRLK8sClientFactory) (bool, error) {

	configParam, _, err := sanityTestCredentials(clusterCreds)
	if err != nil {
		return false, err
	}

	// Ignore the self-signed certificate
	if managedEnvCR.Spec.AllowInsecureSkipTLSVerify {
		configParam.Insecure = true
	}

	clientObj, err := k8sClientFactory.BuildK8sClient(configParam)
	if err != nil {
		return false, fmt.Errorf("unable to create new K8s client to '%v': %w", configParam.Host, err)
	}

	if len(managedEnvCR.Spec.Namespaces) > 0 {
		// If the managed environment contains a namespace, use it to validate that the k8s client (based on the credentials) is valid
		firstNamespaceName := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: managedEnvCR.Spec.Namespaces[0],
			},
		}
		if err := clientObj.Get(ctx, client.ObjectKeyFromObject(&firstNamespaceName), &firstNamespaceName); err != nil {
			return false, fmt.Errorf("unable to verify cluster credentials by retrieving a namespace in the namespace list '%s': %w",
				firstNamespaceName.Name, err)
		}

	} else {
		// To verify that the client works, attempt to simply get the list of namespaces
		var namespaceList corev1.NamespaceList
		if err := clientObj.List(ctx, &namespaceList); err != nil {
			return false, fmt.Errorf("unable to verify cluster credentials by retrieving the list of namespaces '%s': %w",
				clusterCreds.Clustercredentials_cred_id, err)
		}
	}

	// Success!
	return true, nil
}

// Convert the .spec.namespaces field to a sorted, comma-separated list of namespaces
func convertManagedEnvNamespacesFieldToCommaSeparatedList(namespaces []string) (string, error) {
	if len(namespaces) == 0 {
		return "", nil
	}

	for _, namespace := range namespaces {
		if !isValidNamespaceName(namespace) {
			return "", fmt.Errorf("ManagedEnvironment contains an invalid namespace in namespaces list: %s", namespace)
		}
	}

	namespacesSlice := append([]string{}, namespaces...)
	sort.Strings(namespacesSlice)

	return strings.Join(namespacesSlice, ","), nil

}

// A namespace is valid if it conforms to RFC 1123 DNS label standard
func isValidNamespaceName(namespaceName string) bool {
	return len(validation.IsDNS1123Label(namespaceName)) == 0
}
