package shared_resource_loop

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	git "github.com/go-git/go-git/v5"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errGenericCR                   = "unable to retrieve CR from the cluster"
	errUpdateDBRepoCred            = "unable to update repository credential in the database"
	errCreateDBRepoCred            = "unable to create repository credential in the database"
	errCreateDBAppProjecRepository = "unable to create appProject repository in the database"
)

func internalProcessMessage_ReconcileRepositoryCredential(ctx context.Context,
	repositoryCredentialCRName string,
	repositoryCredentialCRNamespace corev1.Namespace,
	apiNamespaceClient client.Client,
	k8sClientFactory SRLK8sClientFactory,
	dbQueries db.DatabaseQueries, shouldWait bool, l logr.Logger) (*db.RepositoryCredentials, error) {

	resourceNS := repositoryCredentialCRNamespace.Name

	clusterUser, _, err := internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx, repositoryCredentialCRNamespace, dbQueries, l)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve cluster user while processing GitOpsRepositoryCredentials: '%s' in namespace: '%s': %v",
			repositoryCredentialCRName, string(repositoryCredentialCRNamespace.UID), err)
	}

	gitopsEngineInstance, _, _, uerr := internalDetermineGitOpsEngineInstance(ctx, *clusterUser, apiNamespaceClient, dbQueries, l)
	if uerr != nil {
		return nil, fmt.Errorf("unable to retrieve cluster user while processing GitOpsRepositoryCredentials: '%s' in namespace: '%s': Error: %w",
			repositoryCredentialCRName, string(repositoryCredentialCRNamespace.UID), uerr.DevError())
	}

	// Note: this may be nil in some if-else branches
	gitopsDeploymentRepositoryCredentialCR := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{}

	// 1) Attempt to get the gitopsDeploymentRepositoryCredentialCR from the namespace
	if err := apiNamespaceClient.Get(ctx, types.NamespacedName{Namespace: resourceNS, Name: repositoryCredentialCRName},
		gitopsDeploymentRepositoryCredentialCR); err != nil {

		if apierr.IsNotFound(err) {
			gitopsDeploymentRepositoryCredentialCR = nil

		} else {
			// Something went wrong, retry
			vErr := fmt.Errorf("unexpected error in retrieving repository credentials: %v", err)
			l.Error(err, vErr.Error(), "DebugErr", errGenericCR, "CR Name", repositoryCredentialCRName, "Namespace", resourceNS)

			return nil, vErr

		}
	}

	// 2) Look for any APICRToDBMappings that point(ed) to a K8s resource with the same name and namespace
	// as this GitOpsDeploymentRespositoryCredential

	var apiCRToDBMappingList []db.APICRToDatabaseMapping
	if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(
		ctx, db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
		repositoryCredentialCRName,
		repositoryCredentialCRNamespace.Name,
		string(repositoryCredentialCRNamespace.GetUID()),
		db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential,
		&apiCRToDBMappingList); err != nil {
		l.Error(err, "Error listing APICRToDatabaseMapping for GitOpsDeploymentRepositoryCredential", "CR Name", repositoryCredentialCRName, "Namespace", resourceNS)

		return nil, fmt.Errorf("unable to list APICRs for repository credentials: %v", err)
	}

	// APICRToDBMapping that matches the current resource UID (or nil if the current resource UID doesn't exist)
	var currentAPICRToDBMapping *db.APICRToDatabaseMapping

	// old APICRToDBMappings that don't match the current resource UID (pointing to previously deleted GitOpsDeploymentRepositoryCredentials)
	var oldAPICRToDBMappings []db.APICRToDatabaseMapping

	// 3) Identify APICRToDBMappings that refer to K8s resources that no longer exist, and find the DTAM that
	// refers to the resource we got the request for.
	for idx := range apiCRToDBMappingList {

		apiCRToDBMapping := apiCRToDBMappingList[idx]

		if gitopsDeploymentRepositoryCredentialCR != nil &&
			apiCRToDBMapping.APIResourceUID == string(gitopsDeploymentRepositoryCredentialCR.UID) {
			currentAPICRToDBMapping = &apiCRToDBMapping
		} else {
			oldAPICRToDBMappings = append(oldAPICRToDBMappings, apiCRToDBMapping)
		}
	}

	// 4) Clean up any old GitOpsDeploymentRepositoryCredential DB rows that have the same name/namespace as this resource,
	// but that no longer exist
	if len(oldAPICRToDBMappings) > 0 {

		for _, oldAPICRToDBMapping := range oldAPICRToDBMappings {
			oldAPICRToDBMapping := oldAPICRToDBMapping // Fixes G601 (CWE-118): Implicit memory aliasing in for loop. (Confidence: MEDIUM, Severity: MEDIUM)
			var operationDBID string
			repositoryCredentialPrimaryKey := oldAPICRToDBMapping.DBRelationKey

			// 4a) Delete the  RepositoryCredential DB row if it exists (and create an operation for it if needed)
			if dbRepoCred, err := dbQueries.GetRepositoryCredentialsByID(ctx, repositoryCredentialPrimaryKey); err != nil {

				if db.IsResultNotFoundError(err) {
					// The CR is not found in the database, so it is already deleted
				} else {
					// It's a glitch, so return the error
					l.Error(err, "unable to retrieve repository credential from the database")
					return nil, fmt.Errorf("unable to retrieve repository credential from the database: %v", err)
				}

			} else {
				if _, err := deleteAppProjectRepositoryFromDB(ctx, dbQueries, clusterUser.Clusteruser_id, l); err != nil {
					l.Error(err, "unable to delete app repo cred from DB")
					return nil, err
				}

				if _, err := deleteRepoCredFromDB(ctx, dbQueries, repositoryCredentialPrimaryKey, l); err != nil {
					l.Error(err, "unable to delete repo cred from DB")
					return nil, err
				}
				l.Info("RepositoryCredential row deleted from DB", "RepositoryCredential ID", repositoryCredentialPrimaryKey)

				// We need to fire-up an Operation as well
				l.Info("Creating an Operation for the deleted RepositoryCredential DB row", "RepositoryCredential ID", repositoryCredentialPrimaryKey)
				if operationDBID, err = createRepoCredOperation(ctx, dbRepoCred, clusterUser, resourceNS, dbQueries,
					apiNamespaceClient, shouldWait, l); err != nil {

					l.Error(err, "Error creating an Operation for the deleted RepositoryCredential DB row", "RepositoryCredential ID", repositoryCredentialPrimaryKey)
					return nil, err
				}
				if err := CleanRepoCredOperation(ctx, dbRepoCred, clusterUser, resourceNS, dbQueries, apiNamespaceClient, operationDBID, l); err != nil {
					l.Error(err, "Error cleaning up the Operation for the deleted RepositoryCredential DB row", "RepositoryCredential ID", repositoryCredentialPrimaryKey)
					return nil, err
				}
			}

			// 4b) Next, we delete the APICRToDBMapping that pointed to the deleted RepositoryCredential DB row
			// Delete the APICRToDatabaseMapping referenced by 'item'
			if rowsDeleted, err := dbQueries.DeleteAPICRToDatabaseMapping(ctx, &oldAPICRToDBMapping); err != nil {
				l.Error(err, "unable to delete apiCRToDBmapping", "mapping", oldAPICRToDBMapping.APIResourceUID)
				return nil, err
			} else if rowsDeleted == 0 {
				l.Info("unexpected number of rows deleted of apiCRToDBmapping", "mapping", oldAPICRToDBMapping.APIResourceUID)
			} else {
				l.Info("deleted APICRToDatabaseMapping", "mapping", oldAPICRToDBMapping.APIResourceUID)
			}
		}

		// We've completed cleanup of all the old repo cred CRs
	}

	if gitopsDeploymentRepositoryCredentialCR == nil {
		// If the GitOpsDeploymentRepositoryCredential doesn't exist, then our work is done.
		return nil, nil
	}

	// 5) If gitopsDeploymentRepositoryCredentialCR exists in the cluster, check the DB to see if the related RepositoryCredential row exists as well

	// Sanity test for gitopsDeploymentRepositoryCredentialCR.Spec.Secret to be non-empty value
	if gitopsDeploymentRepositoryCredentialCR.Spec.Secret == "" {
		if err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, apiNamespaceClient, nil, l); err != nil {
			l.Error(err, fmt.Sprintf("error updating status of GitopsDeploymentRepositoryCredential %v", gitopsDeploymentRepositoryCredentialCR))
		}
		return nil, fmt.Errorf("secret cannot be empty")
	}

	var privateURL, authUsername, authPassword, authSSHKey, secretObj string
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitopsDeploymentRepositoryCredentialCR.Spec.Secret,
			Namespace: resourceNS, // we assume the secret is in the same namespace as the CR
		},
	}

	privateURL = gitopsDeploymentRepositoryCredentialCR.Spec.Repository

	// Fetch the secret from the cluster
	if err := apiNamespaceClient.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: secret.Namespace}, secret); err != nil {
		var errMessage error
		if apierr.IsNotFound(err) {
			errMessage = fmt.Errorf("secret not found: %v", err)
		} else {
			// Something went wrong, retry
			errMessage = fmt.Errorf("error retrieving secret: %v", err)
		}
		if err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, apiNamespaceClient, secret, l); err != nil {
			l.Error(err, fmt.Sprintf("error updating status of GitopsDeploymentRepositoryCredential %v", gitopsDeploymentRepositoryCredentialCR))
		}

		return nil, errMessage
	} else {
		// Secret exists, so get its data
		authUsername = string(secret.Data["username"])
		authPassword = string(secret.Data["password"])
		authSSHKey = string(secret.Data["sshPrivateKey"])
		secretObj = secret.Name
	}

	// Before updating the records in DB, we need to set the Conditions of the CR
	if err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, apiNamespaceClient, secret, l); err != nil {
		l.Error(err, fmt.Sprintf("error updating status of GitopsDeploymentRepositoryCredential %v", gitopsDeploymentRepositoryCredentialCR))
	}

	// 6) If there is no existing APICRToDBMapping for this CR, then let's create one
	if currentAPICRToDBMapping == nil {
		dbRepoCred := db.RepositoryCredentials{
			UserID:          clusterUser.Clusteruser_id, // comply with the constraint 'fk_clusteruser_id'
			PrivateURL:      privateURL,
			AuthUsername:    authUsername,
			AuthPassword:    authPassword,
			AuthSSHKey:      authSSHKey,
			SecretObj:       secretObj,
			EngineClusterID: gitopsEngineInstance.Gitopsengineinstance_id, // comply with the constraint 'fk_gitopsengineinstance_id',
		}

		err = dbQueries.CreateRepositoryCredentials(ctx, &dbRepoCred)
		if err != nil {
			l.Error(err, "Error creating RepositoryCredential row in DB", "DebugErr", errCreateDBRepoCred, "CR Name", repositoryCredentialCRName, "Namespace", resourceNS)
			return nil, fmt.Errorf("unable to create repository credential in the database: %v", err)
		}
		l.Info("Created RepositoryCredential in the DB", "repositoryCredential", dbRepoCred.RepositoryCredentialsID)

		normalizedRepoURL := NormalizeGitURL(dbRepoCred.PrivateURL)

		appProjectRepoCredDB := db.AppProjectRepository{
			Clusteruser_id:          clusterUser.Clusteruser_id,
			RepositoryCredentialsID: dbRepoCred.RepositoryCredentialsID,
			RepoURL:                 normalizedRepoURL,
		}

		if err := dbQueries.CreateAppProjectRepository(ctx, &appProjectRepoCredDB); err != nil {
			l.Error(err, "Error creating AppProjectRepository row in DB", "DebugErr", errCreateDBAppProjecRepository, "CR Name", repositoryCredentialCRName, "Namespace", resourceNS)
			return nil, fmt.Errorf("unable to create appProject repository in the database: %v", err)
		}

		l.Info("Created new AppProjectRepository in the DB", "appProjectRepository", appProjectRepoCredDB.AppProjectRepositoryID)

		// Create the mapping
		newApiCRToDBMapping := db.APICRToDatabaseMapping{
			APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
			APIResourceUID:       string(gitopsDeploymentRepositoryCredentialCR.UID),
			APIResourceName:      repositoryCredentialCRName,
			APIResourceNamespace: resourceNS,
			NamespaceUID:         string(repositoryCredentialCRNamespace.GetUID()),

			DBRelationType: db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential,
			DBRelationKey:  dbRepoCred.RepositoryCredentialsID,
		}

		if err := dbQueries.CreateAPICRToDatabaseMapping(ctx, &newApiCRToDBMapping); err != nil {
			l.Error(err, "unable to create api to db mapping in database", "mapping", newApiCRToDBMapping.APIResourceUID)

			// If we were unable to create api to db mapping in database, delete the fresh newly created repository credential from the database
			if _, err := dbQueries.DeleteRepositoryCredentialsByID(ctx, dbRepoCred.RepositoryCredentialsID); err != nil {
				l.Error(err, "unable to delete repository credential from database")
				return nil, fmt.Errorf("unable to delete repository credential from database: %v", err)
			}

			l.Info("Deleted RepositoryCredential from the DB", "RepositoryCredential ID", dbRepoCred.RepositoryCredentialsID)

			return nil, err
		}

		l.Info(fmt.Sprintf("Created a ApiCRToDBMapping: (APIResourceType: %s, APIResourceUID: %s, DBRelationType: %s)", newApiCRToDBMapping.APIResourceType, newApiCRToDBMapping.APIResourceUID, newApiCRToDBMapping.DBRelationType))

		operationDBID, err := createRepoCredOperation(ctx, dbRepoCred, clusterUser, resourceNS, dbQueries, apiNamespaceClient, shouldWait, l)
		if err != nil {
			return nil, err
		}
		if err := CleanRepoCredOperation(ctx, dbRepoCred, clusterUser, resourceNS, dbQueries, apiNamespaceClient, operationDBID, l); err != nil {
			l.Error(err, "unable to clean up operation", "Operation ID", operationDBID)
			return nil, err
		}

		return &dbRepoCred, nil
	}

	// 7) If the APICRToDBMapping already exists in the database, and already points to the CR, then it is instead an update

	// Match found in database

	repositoryCredentialPrimaryKey := currentAPICRToDBMapping.DBRelationKey

	if dbRepoCred, err := dbQueries.GetRepositoryCredentialsByID(ctx, repositoryCredentialPrimaryKey); err != nil {

		if db.IsResultNotFoundError(err) {
			// If the APICRToDBMapping points to a RepositoryCredential that doesn't exist, delete the APICRToDBMapping
			// and return an error.

			l.Error(err, "Deleting APICRToDBMapping that points to an invalid repository credential", "mapping", currentAPICRToDBMapping.APIResourceUID)

			// Delete the APICRToDatabaseMapping referenced by 'item'
			if rowsDeleted, err := dbQueries.DeleteAPICRToDatabaseMapping(ctx, currentAPICRToDBMapping); err != nil {
				l.Error(err, "unable to delete apiCRToDBmapping", "mapping", currentAPICRToDBMapping.APIResourceUID)
				return nil, err
			} else if rowsDeleted == 0 {
				l.Info("unexpected number of rows deleted of apiCRToDBmapping", "mapping", currentAPICRToDBMapping.APIResourceUID)
			}
			l.Info("Deleted APICRToDBMapping that points to an invalid repository credential", "mapping", currentAPICRToDBMapping.APIResourceUID)
			return nil, fmt.Errorf("APICRToDBMapping pointed to a RepositoryCredential that didn't exist")
		}

		l.Error(err, "unable to get repository credential from database", "RepositoryCredential ID", repositoryCredentialPrimaryKey)
		err = fmt.Errorf("error retrieving repository credentials from DB: %v", err)
		return nil, err

	} else {

		// If the CR exists in the cluster and in the DB, then check if the data is the same and create an Operation
		isUpdateNeeded := compareAndModifyClusterResourceWithDatabaseRow(*gitopsDeploymentRepositoryCredentialCR, &dbRepoCred, secret, l)
		if isUpdateNeeded {
			var operationDBID string
			l.Info("Syncing data between the RepositoryCredential CR and its related DB row",
				"CR", gitopsDeploymentRepositoryCredentialCR.Name, "Namespace", gitopsDeploymentRepositoryCredentialCR.Namespace,
				"DB Row", dbRepoCred.RepositoryCredentialsID)
			if err := dbQueries.UpdateRepositoryCredentials(ctx, &dbRepoCred); err != nil {
				l.Error(err, errUpdateDBRepoCred)
				return nil, err
			}

			normalizedRepoURL := NormalizeGitURL(dbRepoCred.PrivateURL)

			// Check whether the AppProjectRepository exists. If the RepositoryCredentials have been updated and the AppProjectRepository is not present, create it.
			appProjectRepository := db.AppProjectRepository{
				Clusteruser_id: clusterUser.Clusteruser_id,
				RepoURL:        normalizedRepoURL,
			}

			if err := dbQueries.GetAppProjectRepositoryByClusterUserId(ctx, &appProjectRepository); err != nil {
				l.Error(err, "Unable to retrive appProjectRepository", appProjectRepository.GetAsLogKeyValues()...)

				// If AppProjectRepository is not present in DB, create it.
				if db.IsResultNotFoundError(err) {
					appProjectRepoCredDB := db.AppProjectRepository{
						Clusteruser_id:          clusterUser.Clusteruser_id,
						RepositoryCredentialsID: dbRepoCred.RepositoryCredentialsID,
						RepoURL:                 normalizedRepoURL,
					}

					if err := dbQueries.CreateAppProjectRepository(ctx, &appProjectRepoCredDB); err != nil {
						l.Error(err, "Unable to create appProjectRepository..", appProjectRepoCredDB.GetAsLogKeyValues()...)

						return nil, err
					}

					l.Info("Created new AppProjectRepository in DB : "+appProjectRepoCredDB.AppProjectRepositoryID, appProjectRepoCredDB.GetAsLogKeyValues()...)
				}

				return nil, err
			}

			if operationDBID, err = createRepoCredOperation(ctx, dbRepoCred, clusterUser, resourceNS, dbQueries, apiNamespaceClient, shouldWait, l); err != nil {
				return nil, err
			}

			if err := CleanRepoCredOperation(ctx, dbRepoCred, clusterUser, resourceNS, dbQueries, apiNamespaceClient, operationDBID, l); err != nil {
				return nil, err
			}

			return &dbRepoCred, nil

		} else {
			return &dbRepoCred, nil
		}
	}
}

func CleanRepoCredOperation(ctx context.Context, dbRepoCred db.RepositoryCredentials, clusterUser *db.ClusterUser, ns string,
	dbQueries db.DatabaseQueries, client client.Client, operationDBID string, l logr.Logger) error {

	// Get list of Operations from cluster.
	listOfK8sOperation := managedgitopsv1alpha1.OperationList{}
	err := client.List(ctx, &listOfK8sOperation)
	if err != nil {
		l.Error(err, "unable to fetch list of Operation from cluster.", "clusterUser", clusterUser.User_name)
		return err
	}

	for _, k8sOperation := range listOfK8sOperation.Items {
		// Skip if Operation CR is not related to the Database Operation row.
		if k8sOperation.Spec.OperationID != operationDBID {
			errV := fmt.Errorf("skipping Operation (ID: %v) because it is not for this RepositoryCredential Operation Row (ID: %v)", k8sOperation.Spec.OperationID, operationDBID)
			l.Error(errV, errV.Error())
			continue
		}

		// Fetch corresponding DB entry.
		dbOperation := db.Operation{
			Operation_id: k8sOperation.Spec.OperationID,
		}

		// If we cannot fetch the DB entry, then skip.
		if err := dbQueries.GetOperationById(ctx, &dbOperation); err != nil {
			l.Error(err, "unable to fetch Operation row from DB.", "Operation DB ID", dbOperation.Operation_id)
			continue
		}

		// If Operation is not in a terminal state, then skip.
		if dbOperation.State != db.OperationState_Completed && dbOperation.State != db.OperationState_Failed {
			l.Info("Operation CR is not ready for cleanup: " + k8sOperation.Spec.OperationID)
			continue
		}

		// If Operation is not for the Operation_owner_user_id of the RepositoryCredential, then skip.
		if dbOperation.Operation_owner_user_id != clusterUser.Clusteruser_id {
			l.Error(err, "skipping Operation that is not for this RepositoryCredential's Operation_owner_user_id.",
				"Operation DB ID", dbOperation.Operation_id, "Operation Owner User ID", dbOperation.Operation_owner_user_id,
				"ClusterUser ID", clusterUser.Clusteruser_id)
			continue
		}

		l.Info("Deleting Operation CR: " + string(k8sOperation.Name) + " and the related Operation DB entry: " + dbOperation.Operation_id + " ID")

		// Delete the Operation CR and the related DB entry.
		if err := operations.CleanupOperation(ctx, dbOperation, k8sOperation, dbQueries, client, true, l); err != nil {
			l.Error(err, "unable to delete Operations for RepositoryCredential.", "Operation CR Name", k8sOperation.Name,
				"Operation DB ID", dbOperation.Operation_id, "RepositoryCredential ID", dbRepoCred.RepositoryCredentialsID)
			return err
		} else {
			l.Info("Deleted Operation CR and the related Operation DB entry.", "Operation CR Name", k8sOperation.Name,
				"Operation DB ID", dbOperation.Operation_id, "RepositoryCredential ID", dbRepoCred.RepositoryCredentialsID)
		}
	}

	return nil
}

func createRepoCredOperation(ctx context.Context, dbRepoCred db.RepositoryCredentials, clusterUser *db.ClusterUser, ns string,
	dbQueries db.DatabaseQueries, apiNamespaceClient client.Client, shouldWait bool, l logr.Logger) (string, error) {

	dbOperationInput := db.Operation{
		Instance_id:             dbRepoCred.EngineClusterID,
		Resource_id:             dbRepoCred.RepositoryCredentialsID,
		Resource_type:           db.OperationResourceType_RepositoryCredentials,
		State:                   db.OperationState_Waiting,
		Operation_owner_user_id: clusterUser.Clusteruser_id,
	}

	operationCR, operationDB, err := operations.CreateOperation(ctx, shouldWait, dbOperationInput, clusterUser.Clusteruser_id, ns, dbQueries,
		apiNamespaceClient, l)
	if err != nil {
		errV := fmt.Errorf("unable to create operation: %v", err)
		return "", errV
	}

	l.Info("operation has been created", "CR", operationCR, "DB", operationDB)

	return operationDB.Operation_id, nil
}

// Updates the given repository credential CR's status condition to match the given condition and additional checks.
// If there is an existing status condition with the exact same status, reason and message, no update is made in order
// to preserve the LastTransitionTime (see https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition.LastTransitionTime )
func UpdateGitopsDeploymentRepositoryCredentialStatus(ctx context.Context, repositoryCredential *managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential, client client.Client, secret *corev1.Secret, log logr.Logger) error {

	// if the condition was sent along with the function call, we don't need to perform additional checks
	newConditions := generateValidRepositoryCredentialsConditions(repositoryCredential, ctx, secret)

	needToUpdateConditions := false
	for _, condition := range newConditions {
		// do nothing if GitOpsDeploymentRepositoryCredential already has same condition
		for _, c := range repositoryCredential.Status.Conditions {
			if c.Type == condition.Type && (c.Reason != condition.Reason || c.Status != condition.Status || c.Message != condition.Message) {
				needToUpdateConditions = true
				break
			}
		}
	}

	if needToUpdateConditions || len(repositoryCredential.Status.Conditions) != 3 {
		// 1) Attempt to get the latest gitopsDeploymentRepositoryCredentialCR from the namespace
		if err := client.Get(ctx, types.NamespacedName{Namespace: repositoryCredential.Namespace, Name: repositoryCredential.Name},
			repositoryCredential); err != nil {

			if apierr.IsNotFound(err) {
				return nil
			}
			// Something went wrong, retry
			vErr := fmt.Errorf("unexpected error in retrieving repository credentials: %v", err)
			log.Error(err, vErr.Error(), "DebugErr", errGenericCR, "CR Name", repositoryCredential, "Namespace", repositoryCredential.Namespace)
			return vErr
		}
		repositoryCredential.Status.SetConditions(newConditions)
		// Update the GitOpsDeploymentRepositoryCredential CR
		if err := client.Status().Update(ctx, repositoryCredential); err != nil {
			log.Error(err, "updating repository credential CR's status condition")
		}
	}

	return nil
}

func generateValidRepositoryCredentialsConditions(repositoryCredential *managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential, ctx context.Context, secret *corev1.Secret) []metav1.Condition {

	var validRepoUrlCondition, validRepoCredCondition metav1.Condition

	errorOccuredCondition := metav1.Condition{}

	// Check if Secret mentioned in repositoryCredential exists
	if repositoryCredential.Spec.Secret == "" {
		errorOccuredCondition = metav1.Condition{
			Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
			Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotSpecified,
			Status:  metav1.ConditionTrue,
			Message: "Secret field is missing value",
		}
	} else if secret == nil {
		// Secret is not sent because secret identified in RepositoryCredential was not found
		errorOccuredCondition = metav1.Condition{
			Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
			Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotFound,
			Status:  metav1.ConditionTrue,
			Message: "Secret specified not found",
		}
	}

	if errorOccuredCondition != (metav1.Condition{}) {
		validRepoUrlCondition = metav1.Condition{
			Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
			Reason:  errorOccuredCondition.Reason,
			Status:  metav1.ConditionFalse,
			Message: errorOccuredCondition.Message,
		}
		validRepoCredCondition = metav1.Condition{
			Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
			Reason:  errorOccuredCondition.Reason,
			Status:  metav1.ConditionFalse,
			Message: errorOccuredCondition.Message,
		}
	} else {
		err := validateRepositoryCredentials(repositoryCredential.Spec.Repository, secret)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				// Repository does not exist
				validRepoUrlCondition = metav1.Condition{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
					Status:  metav1.ConditionFalse,
					Message: fmt.Sprintf("Repository does not exist: %s", err.Error()),
				}
				validRepoCredCondition = metav1.Condition{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
					Status:  metav1.ConditionFalse,
					Message: fmt.Sprintf("Repository does not exist: %s", err.Error()),
				}
				errorOccuredCondition = metav1.Condition{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
					Status:  metav1.ConditionTrue,
					Message: fmt.Sprintf("Repository does not exist: %s", err.Error()),
				}
			} else {
				// Credentials were invalid
				validRepoUrlCondition = metav1.Condition{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status:  metav1.ConditionTrue,
					Message: fmt.Sprintf("Repository %s exists", repositoryCredential.Spec.Repository),
				}
				validRepoCredCondition = metav1.Condition{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status:  metav1.ConditionFalse,
					Message: fmt.Sprintf("Repository Credentials provided %s for Repository %s are invalid", secret.Name, repositoryCredential.Spec.Repository),
				}
				errorOccuredCondition = metav1.Condition{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status:  metav1.ConditionTrue,
					Message: fmt.Sprintf("Repository Credentials provided %s for Repository %s are invalid", secret.Name, repositoryCredential.Spec.Repository),
				}
			}
		} else {
			// No errors occured
			errorOccuredCondition = metav1.Condition{
				Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
				Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
				Status:  metav1.ConditionFalse,
				Message: "RepositoryCredentials are Valid",
			}
			validRepoUrlCondition = metav1.Condition{
				Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
				Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
				Status:  metav1.ConditionTrue,
				Message: fmt.Sprintf("Repository %s exists", repositoryCredential.Spec.Repository),
			}
			validRepoCredCondition = metav1.Condition{
				Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
				Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
				Status:  metav1.ConditionTrue,
				Message: fmt.Sprintf("Repository Credentials provided %s for Repository %s are valid", secret.Name, repositoryCredential.Spec.Repository),
			}
		}
	}

	return []metav1.Condition{errorOccuredCondition, validRepoUrlCondition, validRepoCredCondition}
}

func validateRepositoryCredentials(rawRepoURL string, secret *corev1.Secret) error {

	normalizedRepoUrl := NormalizeGitURL(rawRepoURL)
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{normalizedRepoUrl},
	})

	// Secret exists, so get its data
	authUsername := string(secret.Data["username"])
	authPassword := string(secret.Data["password"])
	authSSHKey := string(secret.Data["sshPrivateKey"])

	listOptions := &git.ListOptions{}

	if authSSHKey != "" {
		privateKey, err := ssh.NewPublicKeys("git", []byte(authSSHKey), "")
		if err != nil {
			return err
		}
		listOptions.Auth = privateKey
	} else {
		listOptions.Auth = &http.BasicAuth{
			Username: authUsername,
			Password: authPassword,
		}
	}

	_, err := rem.List(listOptions)
	return err
}

// EnsurePrefix idempotently ensures that a base string has a given prefix.
func ensurePrefix(s, prefix string) string {
	if !strings.HasPrefix(s, prefix) {
		s = prefix + s
	}
	return s
}

// removeSuffix idempotently removes a given suffix
func removeSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s[0 : len(s)-len(suffix)]
	}
	return s
}

var (
	sshURLRegex = regexp.MustCompile("^(ssh://)?([^/:]*?)@[^@]+$")
)

// normalizeGitURL normalizes a git URL for purposes of comparison, as well as preventing redundant
// local clones (by normalizing various forms of a URL to a consistent location).
func NormalizeGitURL(repo string) string {
	repo = strings.ToLower(strings.TrimSpace(repo))
	if yes, _ := isSSHURL(repo); yes {
		if !strings.HasPrefix(repo, "ssh://") {
			// We need to replace the first colon in git@server... style SSH URLs with a slash, otherwise
			// net/url.Parse will interpret it incorrectly as the port.
			repo = strings.Replace(repo, ":", "/", 1)
			repo = ensurePrefix(repo, "ssh://")
		}
	}
	repo = removeSuffix(repo, ".git")
	repoURL, err := url.Parse(repo)
	if err != nil {
		return ""
	}
	normalized := repoURL.String()
	return strings.TrimPrefix(normalized, "ssh://")
}

// isSSHURL returns true if supplied URL is SSH URL
func isSSHURL(url string) (bool, string) {
	matches := sshURLRegex.FindStringSubmatch(url)
	if len(matches) > 2 {
		return true, matches[2]
	}
	return false, ""
}
