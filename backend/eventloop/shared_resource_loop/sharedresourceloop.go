package shared_resource_loop

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/gitopserrors"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The goal of the shared resource event loop is to ensure that API-namespace-scoped resources are only
// created from a single thread, preventing concurrent goroutines from stepping on each others toes.
//
// This ensures that:
//   - When multiple 'application event loop' goroutines are attempting to create workspace-scoped resources,
//     that no duplicates are created (eg it shouldn't be possible to create multiple ClusterUsers for a single user, or multiple
//     ManagedEnvironments for a single namespace)
//   - There are no race conditions on creation of namespace-scoped resources.
//
// API-namespace-scoped resources are:
// - managedenv
// - clusteraccess
// - clusteruser
// - gitopsengineinstance
// - repositorycredential
//
// Ultimately the goal of this file is to avoid this issue:
// - In the same moment of time, both these actions happen simultaneously:
//   - thread 1: creates (for example) a managed environment DB row for environment A, while processing a GitOpsDeployment targeting A
//   - thread 2: creates (for example) a managed environment DB row for environment A, while processing a different GitOpsDeployment targeting A
//   - But this is bad: the database now contains _two different_ managed environment database entries for the same environment A.
//   - Thus, without mutexes/locking, there is a race condition.
//   - However, the shared resource event loop prevents this issue, by ensuring that threads are never able to
//     concurrently create API-namespace-scoped database resources at the same time.
type SharedResourceEventLoop struct {
	inputChannel chan sharedResourceLoopMessage

	// For use by unit tests only: If this function is non-nil, it will be used instead of the default repository credentials validation function.
	validateRepoURLAndCredentialsFunction ValidateRepoURLAndCredentialsFunction
}

// ReconcileAppProjectRepositories ensures that the necessary AppProjectRepository database rows exists in the database, and that they are consistent with the GitOpsDeployment/GitOpsDeploymentRepositoryCredentials defined in the given Namespace.
//
// parameters:
// - gitRepoURLUnnormalizedOfRequest is the repository URL defined in the GitOpDeployment or GitOpsDeploymentRepositoryCredential for which
// this function was invoked.
//   - this function will only process DB rows, or K8s resources that reference this specific Git repository URL (ignoring all others)
//   - If 'gitRepoURLUnnormalizedOfRequest' is empty (""), then all resources will be processed.
//
// return value:
// - bool: true if one or more 'AppProject*' rows in the DB were updated, false otherwise. (If yes, ensure an Operation on the Application caused the AppProject CR to be regenerated)
// - error
func (srEventLoop *SharedResourceEventLoop) ReconcileAppProjectRepositories(ctx context.Context, workspaceClient client.Client,
	workspaceNamespace corev1.Namespace, l logr.Logger) (bool, error) {

	responseChannel := make(chan any)

	msg := sharedResourceLoopMessage{
		log:                l,
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_reconcileAppProjectRepositories,
		responseChannel:    responseChannel,
		ctx:                ctx,
		payload:            sharedResourceLoopMessage_reconcileAppProjectRepositoriesRequest{},
	}

	srEventLoop.inputChannel <- msg

	var rawResponse any

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return false, fmt.Errorf("context cancelled in ReconcileAppProjectRepositories")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_reconcileAppProjectRepositoryResponse)
	if !ok {
		return false, fmt.Errorf("SEVERE: unexpected response type")
	}

	return response.appProjectDBRowsUpdated, response.err

}

// The bool return value is 'true' if ClusterUser is created; 'false' if it already exists in DB or in case of failure.
func (srEventLoop *SharedResourceEventLoop) GetOrCreateClusterUserByNamespaceUID(ctx context.Context, workspaceClient client.Client,
	workspaceNamespace corev1.Namespace, l logr.Logger) (*db.ClusterUser, bool, error) {

	responseChannel := make(chan any)

	msg := sharedResourceLoopMessage{
		log:                l,
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUID,
		responseChannel:    responseChannel,
		ctx:                ctx,
	}

	srEventLoop.inputChannel <- msg

	var rawResponse any

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return nil, false, fmt.Errorf("context cancelled in getOrCreateClusterUserByNamespaceUID")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDResponse)
	if !ok {
		return nil, false, fmt.Errorf("SEVERE: unexpected response type")
	}

	return response.clusterUser, response.isNewUser, response.err

}

func (srEventLoop *SharedResourceEventLoop) GetGitopsEngineInstanceById(ctx context.Context, id string, workspaceClient client.Client,
	workspaceNamespace corev1.Namespace, l logr.Logger) (*db.GitopsEngineInstance, error) {

	responseChannel := make(chan any)

	msg := sharedResourceLoopMessage{
		log:                l,
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_getGitopsEngineInstanceById,
		responseChannel:    responseChannel,
		payload: sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDRequest{
			gitopsEngineInstanceID: id,
		},
		ctx: ctx,
	}

	srEventLoop.inputChannel <- msg

	var rawResponse any

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled in getGitOpsEngineInstanceById")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_getGitopsEngineInstanceByIdResponse)
	if !ok {
		return nil, fmt.Errorf("SEVERE: unexpected response type")
	}

	return response.gitopsEngineInstance, response.err

}

// Ensure the user's workspace is configured, ensure a GitOpsEngineInstance exists that will target it, and ensure
// a cluster access exists the give the user permission to target them from the engine.
// Return values:
// - SharedResourceManagedEnvContainer: contains DB resources that were created/retrieved by the call
// - bool: whether or not the error param is a user error (see elsewhere for definition of user error)
// - error: whether an error occurred during reconciliation
func (srEventLoop *SharedResourceEventLoop) ReconcileSharedManagedEnv(ctx context.Context,
	workspaceClient client.Client, workspaceNamespace corev1.Namespace,
	managedEnvironmentCRName string, managedEnvironmentCRNamespace string, isWorkspaceTarget bool,
	k8sClientFactory SRLK8sClientFactory, l logr.Logger) (SharedResourceManagedEnvContainer, bool, error) {

	res := newSharedResourceManagedEnvContainer()

	if !isWorkspaceTarget && (managedEnvironmentCRName == "" || managedEnvironmentCRNamespace == "") {
		// Sanity test the parameters
		return res, userError_false, fmt.Errorf("managed environment name or namespace were empty")
	}

	request := sharedResourceLoopMessage_getOrCreateSharedResourceManagedEnvRequest{
		managedEnvironmentCRName:      managedEnvironmentCRName,
		managedEnvironmentCRNamespace: managedEnvironmentCRNamespace,
		isWorkspaceTarget:             isWorkspaceTarget,
		k8sClientFactory:              k8sClientFactory,
	}

	responseChannel := make(chan any)

	msg := sharedResourceLoopMessage{
		log:                l,
		ctx:                ctx,
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_getOrCreateSharedManagedEnv,
		responseChannel:    responseChannel,
		payload:            request,
	}

	srEventLoop.inputChannel <- msg

	var rawResponse any

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return res, userError_false, fmt.Errorf("context cancelled in GetOrCreateSharedManagedEnv")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_getOrCreateSharedResourcesResponse)
	if !ok {
		return res, userError_false, fmt.Errorf("SEVERE: unexpected response type")
	}
	res = response.responseContainer

	return res, response.isUserError, response.err

}

func (srEventLoop *SharedResourceEventLoop) ReconcileRepositoryCredential(ctx context.Context,
	workspaceClient client.Client, workspaceNamespace corev1.Namespace,
	repositoryCredentialCRName string, k8sClientFactory SRLK8sClientFactory, l logr.Logger) (*db.RepositoryCredentials, error) {

	request := sharedResourceLoopMessage_reconcileRepositoryCredentialRequest{
		repositoryCredentialCRName:      repositoryCredentialCRName,
		repositoryCredentialCRNamespace: workspaceNamespace.Name,
		k8sClientFactory:                k8sClientFactory,
	}

	responseChannel := make(chan any)

	msg := sharedResourceLoopMessage{
		log:                l,
		ctx:                ctx,
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_reconcileRepositoryCredential,
		responseChannel:    responseChannel,
		payload:            request,
	}

	srEventLoop.inputChannel <- msg

	var rawResponse any

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled in ReconcileRepositoryCredential")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_reconcileRepositoryCredentialResponse)
	if !ok {
		return nil, fmt.Errorf("SEVERE: unexpected response type")
	}
	return response.repositoryCredential, response.err

}

// NewSharedResourceLoop creates a new SharedResourceLoop, and starts the goroutine responsible for processing channel messages.
// See documentation at top of this file for details.
func NewSharedResourceLoop() *SharedResourceEventLoop {

	sharedResourceEventLoop := &SharedResourceEventLoop{
		inputChannel: make(chan sharedResourceLoopMessage),
	}

	go sharedResourceEventLoop.internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

	return sharedResourceEventLoop
}

// NewSharedResourceLoopWithCustomFuncs allows override of validation functions from NewSharedResourceLoop
// Note: This should only be called from unit tests
func NewSharedResourceLoopWithCustomFuncs(validateRepoURLFunction ValidateRepoURLAndCredentialsFunction) *SharedResourceEventLoop {

	sharedResourceEventLoop := &SharedResourceEventLoop{
		inputChannel:                          make(chan sharedResourceLoopMessage),
		validateRepoURLAndCredentialsFunction: validateRepoURLFunction,
	}

	go sharedResourceEventLoop.internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

	return sharedResourceEventLoop
}

type sharedResourceLoopMessageType string

const (
	sharedResourceLoopMessage_getOrCreateSharedManagedEnv          sharedResourceLoopMessageType = "getOrCreateSharedManagedEnv"
	sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUID sharedResourceLoopMessageType = "getOrCreateClusterUserByNamespaceUID"
	sharedResourceLoopMessage_getGitopsEngineInstanceById          sharedResourceLoopMessageType = "getGitopsEngineInstanceById"
	sharedResourceLoopMessage_reconcileRepositoryCredential        sharedResourceLoopMessageType = "reconcileRepositoryCredential"
	sharedResourceLoopMessage_reconcileAppProjectRepositories      sharedResourceLoopMessageType = "reconcileAppProjectRepositories"
)

type sharedResourceLoopMessage struct {
	log                logr.Logger
	ctx                context.Context
	workspaceClient    client.Client
	workspaceNamespace corev1.Namespace

	messageType     sharedResourceLoopMessageType
	responseChannel chan (any)

	// optional payload
	payload any
}

type sharedResourceLoopMessage_getGitopsEngineInstanceByIdResponse struct {
	err                  error
	gitopsEngineInstance *db.GitopsEngineInstance
}

type sharedResourceLoopMessage_getOrCreateSharedResourceManagedEnvRequest struct {
	managedEnvironmentCRName      string
	managedEnvironmentCRNamespace string
	isWorkspaceTarget             bool
	k8sClientFactory              SRLK8sClientFactory
}

type sharedResourceLoopMessage_getOrCreateSharedResourcesResponse struct {

	// err is whether an error occurred while reconciling the managed env
	err error

	// if 'err' is non-nil, the 'isUserError' field will indicate whether the error is a user error.
	// A user error is case where the user has specified an invalid value in the GitOpsDeploymentManagedEnvironment, or the Secret
	//
	// - An example of a user error: user specified a Secret (in the GitOpsDeploymentManagedEnvironment) that doesn't exist.
	// - An example of a non-user error: unable to connect to the database
	//
	// We do not need to continue to reconcile a resource that has a user error: a fix is required to the resource (for example, creating a missing Secret)
	isUserError bool

	responseContainer SharedResourceManagedEnvContainer
}

type sharedResourceLoopMessage_reconcileRepositoryCredentialRequest struct {
	repositoryCredentialCRName      string
	repositoryCredentialCRNamespace string
	k8sClientFactory                SRLK8sClientFactory
}

type sharedResourceLoopMessage_reconcileRepositoryCredentialResponse struct {
	err                  error
	repositoryCredential *db.RepositoryCredentials
}

type sharedResourceLoopMessage_reconcileAppProjectRepositoriesRequest struct {
}

func newSharedResourceManagedEnvContainer() SharedResourceManagedEnvContainer {
	return SharedResourceManagedEnvContainer{
		ClusterUser:          nil,
		IsNewUser:            false,
		ManagedEnv:           nil,
		IsNewManagedEnv:      false,
		GitopsEngineInstance: nil,
		IsNewInstance:        false,
		ClusterAccess:        nil,
		IsNewClusterAccess:   false,
		GitopsEngineCluster:  nil,
	}
}

// SharedResourceManagedEnvContainer is the return value of ReconcileSharedManagedEnv, and contains the
// resources that were created by the reconciliation.
type SharedResourceManagedEnvContainer struct {
	ClusterUser          *db.ClusterUser
	ManagedEnv           *db.ManagedEnvironment
	GitopsEngineInstance *db.GitopsEngineInstance
	ClusterAccess        *db.ClusterAccess
	GitopsEngineCluster  *db.GitopsEngineCluster
	IsNewUser            bool
	IsNewManagedEnv      bool
	IsNewInstance        bool
	IsNewClusterAccess   bool
}

type sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDRequest struct {
	gitopsEngineInstanceID string
}

type sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDResponse struct {
	err         error
	clusterUser *db.ClusterUser
	isNewUser   bool
}

type sharedResourceLoopMessage_reconcileAppProjectRepositoryResponse struct {

	// appProjectDBRowsUpdated is true if the AppProject* DB rows were updated by this call
	// - If this value is true, then the calling code should ensure an Operation is creating on the corresponding Application row.
	// - The Operation on the Application row will cause cluster-agent to update the Argo CD AppProject CR
	appProjectDBRowsUpdated bool

	err error
}

func (srel *SharedResourceEventLoop) internalSharedResourceEventLoop(inputChan chan sharedResourceLoopMessage) {

	ctx := context.Background()
	l := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops)
	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		l.Error(err, "SEVERE: internalSharedResourceEventLoop exiting before startup")
		return
	}

	for {
		msg := <-inputChan

		_, err = sharedutil.CatchPanic(func() error {
			srel.processSharedResourceMessage(msg.ctx, msg, dbQueries, msg.log)
			return nil
		})
		if err != nil {
			l.Error(err, "unexpected error from processMessage in internalSharedResourceEventLoop")
		}
	}
}

func (srel *SharedResourceEventLoop) processSharedResourceMessage(ctx context.Context, msg sharedResourceLoopMessage, dbQueries db.DatabaseQueries, log logr.Logger) {

	log.V(logutil.LogLevel_Debug).Info("sharedResourceEventLoop received message: " + string(msg.messageType))

	if msg.messageType == sharedResourceLoopMessage_getOrCreateSharedManagedEnv {

		payload, ok := (msg.payload).(sharedResourceLoopMessage_getOrCreateSharedResourceManagedEnvRequest)
		if !ok {
			err := fmt.Errorf("SEVERE: unexpected payload")
			log.Error(err, "")
			// Reply on a separate goroutine so cancelled callers don't block the event loop
			go func() {
				msg.responseChannel <- sharedResourceLoopMessage_getOrCreateSharedResourcesResponse{
					err: err,
				}
			}()

			return
		}

		res, isUserError, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, msg.workspaceClient, payload.managedEnvironmentCRName,
			payload.managedEnvironmentCRNamespace, payload.isWorkspaceTarget, msg.workspaceNamespace,
			payload.k8sClientFactory, dbQueries, log)

		response := sharedResourceLoopMessage_getOrCreateSharedResourcesResponse{
			err:               err,
			isUserError:       isUserError,
			responseContainer: res,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else if msg.messageType == sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUID {

		clusterUser, isNewUser, err := internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx, msg.workspaceNamespace, dbQueries, log)

		response := sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDResponse{
			err:         err,
			clusterUser: clusterUser,
			isNewUser:   isNewUser,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else if msg.messageType == sharedResourceLoopMessage_getGitopsEngineInstanceById {

		var gitopsEngineInstance *db.GitopsEngineInstance
		var err error

		payload, ok := (msg.payload).(sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDRequest)
		if ok {
			gitopsEngineInstance, err = internalProcessMessage_GetGitopsEngineInstanceById(ctx, payload.gitopsEngineInstanceID, dbQueries)
		} else {
			err = fmt.Errorf("SEVERE - unexpected cast in internalSharedResourceEventLoop")
			log.Error(err, err.Error())
		}

		response := sharedResourceLoopMessage_getGitopsEngineInstanceByIdResponse{
			err:                  err,
			gitopsEngineInstance: gitopsEngineInstance,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()
	} else if msg.messageType == sharedResourceLoopMessage_reconcileRepositoryCredential {

		var err error
		var repositoryCredential *db.RepositoryCredentials

		payload, ok := (msg.payload).(sharedResourceLoopMessage_reconcileRepositoryCredentialRequest)
		if ok {

			repoCredValidationFunction := DefaultValidateRepositoryCredentials

			if srel.validateRepoURLAndCredentialsFunction != nil {
				repoCredValidationFunction = srel.validateRepoURLAndCredentialsFunction
			}

			repositoryCredential, err = internalProcessMessage_ReconcileRepositoryCredential(ctx,
				payload.repositoryCredentialCRName, msg.workspaceNamespace, repoCredValidationFunction, msg.workspaceClient, dbQueries, true, log)

		} else {
			err = fmt.Errorf("SEVERE - unexpected cast in internalSharedResourceEventLoop")
			log.Error(err, err.Error())
		}

		response := sharedResourceLoopMessage_reconcileRepositoryCredentialResponse{
			repositoryCredential: repositoryCredential,
			err:                  err,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else if msg.messageType == sharedResourceLoopMessage_reconcileAppProjectRepositories {

		payload, ok := (msg.payload).(sharedResourceLoopMessage_reconcileAppProjectRepositoriesRequest)
		if !ok {
			err := fmt.Errorf("SEVERE: unexpected payload")
			log.Error(err, "")
			// Reply on a separate goroutine so cancelled callers don't block the event loop
			go func() {
				msg.responseChannel <- sharedResourceLoopMessage_getOrCreateSharedResourcesResponse{
					err: err,
				}
			}()

			return
		}

		databaseUpdated, err := internalProcessMessage_reconcileAppProjectRepositories(ctx, payload, msg.workspaceNamespace, msg.workspaceClient, dbQueries, log)

		response := sharedResourceLoopMessage_reconcileAppProjectRepositoryResponse{
			err:                     err,
			appProjectDBRowsUpdated: databaseUpdated,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else {
		log.Error(nil, "SEVERE: unrecognized sharedResourceLoopMessageType: "+string(msg.messageType))
	}

}

func internalProcessMessage_reconcileAppProjectRepositories(ctx context.Context, payload sharedResourceLoopMessage_reconcileAppProjectRepositoriesRequest, namespace corev1.Namespace, workspaceClient client.Client, dbQueries db.DatabaseQueries, l logr.Logger) (bool, error) {

	return reconcileAppProjectRepositories(ctx, namespace, workspaceClient, dbQueries, l)
}

// reconcileAppProjectRepositories ensures that the necessary AppProjectRepository database rows exists in the database, and that they are consistent with the GitOpsDeployment/GitOpsDeploymentRepositoryCredentials defined in the Namespace.
func reconcileAppProjectRepositories(ctx context.Context, namespace corev1.Namespace, workspaceClient client.Client, dbQueries db.DatabaseQueries, l logr.Logger) (bool, error) {

	clusterUser, _, err := internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx, namespace, dbQueries, l)
	if err != nil || clusterUser == nil {
		return false, fmt.Errorf("unable to retrieve cluster user in reconcileAppProjectRepositories, from namespace '%s': %w", namespace.Name, err)
	}

	// Retrieve all the AppProjectRepository for this namespace
	var appProjectReposForThisNamespaceInDB []db.AppProjectRepository

	if err := dbQueries.ListAppProjectRepositoryByClusterUserId(ctx, clusterUser.Clusteruser_id, &appProjectReposForThisNamespaceInDB); err != nil {
		return false, fmt.Errorf("unable to list app project repositories from DB when reconciling AppProjectRepo: %w", err)
	}

	// Retrieve all the GitOpsDeployments/RepositoryCredentials for this Namespace
	var gitopsDeployments managedgitopsv1alpha1.GitOpsDeploymentList
	if err := workspaceClient.List(ctx, &gitopsDeployments, &client.ListOptions{Namespace: namespace.Name}); err != nil {
		return false, fmt.Errorf("unable to list GitOpsDeployments when reconciling AppProjectRepo in Namespace '%s': %w", namespace.Name, err)
	}

	var repoCreds managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialList
	if err := workspaceClient.List(ctx, &repoCreds, &client.ListOptions{Namespace: namespace.Name}); err != nil {
		return false, fmt.Errorf("unable to list GitOpsDeploymentRepositoryCredentials when reconciling AppProjectRepo in Namespace '%s': %w", namespace.Name, err)
	}

	// For each GitOpsDeployment/RepositoryCredential, generate the corresponding expected AppProjectRepository

	// map: normalized git url -> expected db entry
	expectedDBEntries := map[string]db.AppProjectRepository{}

	for _, repoCred := range repoCreds.Items {
		gitURLOfRepoCred := NormalizeGitURL(repoCred.Spec.Repository)

		expectedEntry := db.AppProjectRepository{
			Clusteruser_id: clusterUser.Clusteruser_id,
			RepoURL:        gitURLOfRepoCred,
		}
		expectedDBEntries[gitURLOfRepoCred] = expectedEntry
	}

	for _, gitopsDepl := range gitopsDeployments.Items {
		gitURLOfGitOpsDepl := NormalizeGitURL(gitopsDepl.Spec.Source.RepoURL)

		expectedEntry := db.AppProjectRepository{
			Clusteruser_id: clusterUser.Clusteruser_id,
			RepoURL:        gitURLOfGitOpsDepl,
		}
		expectedDBEntries[gitURLOfGitOpsDepl] = expectedEntry
	}

	resDatabaseUpdated := false // Whether or not the database was updated by this call

	// For each existing entry in the database, find the corresponding expected entry, and reconcile any differences

	for idx := range appProjectReposForThisNamespaceInDB {

		appProjectRepoInNamepaceFromDB := appProjectReposForThisNamespaceInDB[idx]

		gitURLOfDatabaseRow := NormalizeGitURL(appProjectRepoInNamepaceFromDB.RepoURL)

		if _, exists := expectedDBEntries[gitURLOfDatabaseRow]; !exists {

			// A) The database entry for this repo URL exists, but there is no corresponding CR in the namespace.
			// - thus we should delete the DB entry

			if numDeleted, err := dbQueries.DeleteAppProjectRepositoryByClusterUserAndRepoURL(ctx, &appProjectRepoInNamepaceFromDB); err != nil {
				return false, fmt.Errorf("unable to delete AppProjectRepository which was identified as no longer being required: %w. cluster-user: '%s' repo-url '%s'", err, appProjectRepoInNamepaceFromDB.Clusteruser_id, appProjectRepoInNamepaceFromDB.RepoURL)

			} else if numDeleted == 0 {
				l.V(logutil.LogLevel_Warn).Info("unexpected number of results when deleting AppProjectRepository which was identified as no longer being required", "cluster-user", appProjectRepoInNamepaceFromDB.Clusteruser_id, "repo-url", appProjectRepoInNamepaceFromDB.RepoURL)
			} else {
				l.Info("deleted AppProjectRepository which was identified as no longer in use", "cluster-user", appProjectRepoInNamepaceFromDB.Clusteruser_id, "repo-url", appProjectRepoInNamepaceFromDB.RepoURL)
				resDatabaseUpdated = true
			}

		} else {
			// B) Otherwise, the entry already exists in the DB, and as a CR, so there is no more work to do.
			delete(expectedDBEntries, gitURLOfDatabaseRow)
		}
	}

	// C) Finally, the entries that are still defined in 'expectedDBEntries' need to be created, as they exist as a CR, but not in the DB.
	for _, expectedToExist := range expectedDBEntries {

		gitURLOfMissingAppProjectRepoRow := NormalizeGitURL(expectedToExist.RepoURL)

		newAppProjectRepo := db.AppProjectRepository{
			Clusteruser_id: clusterUser.Clusteruser_id,
			RepoURL:        gitURLOfMissingAppProjectRepoRow,
		}
		if err := dbQueries.CreateAppProjectRepository(ctx, &newAppProjectRepo); err != nil {
			return false, fmt.Errorf("unable to create AppProjectRepository: %w . repository: %v", err, newAppProjectRepo)
		}

		l.Info("created new AppProjectRepository", "cluster-user", newAppProjectRepo.Clusteruser_id, "repo-url", newAppProjectRepo.RepoURL)
		resDatabaseUpdated = true
	}

	return resDatabaseUpdated, nil
}

func deleteRepoCredFromDB(ctx context.Context, repoCredRow db.RepositoryCredentials, repositoryCredentialCRNamespace corev1.Namespace, apiNamespaceClient client.Client, dbQueries db.DatabaseQueries, l logr.Logger) (bool, error) {
	const retry, noRetry = true, false

	l = l.WithValues("repositoryCredentialID", repoCredRow.RepositoryCredentialsID)

	if _, err := reconcileAppProjectRepositories(ctx, repositoryCredentialCRNamespace, apiNamespaceClient, dbQueries, l); err != nil {
		// Log the error and retry
		l.Error(err, "Error deleting app appProjectRepository from database: ")
		return retry, err
	}

	rowsDeleted, err := dbQueries.DeleteRepositoryCredentialsByID(ctx, repoCredRow.RepositoryCredentialsID)

	if err != nil {
		// Log the error and retry
		l.Error(err, "Error deleting repository credential from database:")
		return retry, err
	}

	if rowsDeleted == 0 {
		// Log the error, but continue to delete the other Repository Credentials
		l.Info("No rows deleted from the database", "rowsDeleted", rowsDeleted)
		return noRetry, err
	}

	// meaning: err == nil && rowsDeleted > 0
	l.Info("Deleted Repository Credential from the database")
	return noRetry, nil
}

func compareAndModifyClusterResourceWithDatabaseRow(cr managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential,
	dbr *db.RepositoryCredentials, secret *corev1.Secret, l logr.Logger) bool {

	var isSecretUpdateNeeded bool
	if cr.Spec.Secret != dbr.SecretObj {
		l.Info("Secret name changed", "old", dbr.SecretObj, "new", cr.Spec.Secret)
		dbr.SecretObj = cr.Spec.Secret
		isSecretUpdateNeeded = true
	}

	var isRepoUpdateNeeded bool
	if cr.Spec.Repository != dbr.PrivateURL {
		l.Info("Repository URL changed", "old", dbr.PrivateURL, "new", cr.Spec.Repository)
		dbr.PrivateURL = cr.Spec.Repository
		isRepoUpdateNeeded = true
	}

	// Fetch these data from the secret
	authUsername := string(secret.Data["username"])
	authPassword := string(secret.Data["password"])
	authSSHKey := string(secret.Data["sshPrivateKey"])

	// Compare the data from the secret with the data from the DB
	var isAuthUsernameUpdateNeeded bool
	if authUsername != dbr.AuthUsername {
		l.Info("AuthUsername changed")
		dbr.AuthUsername = authUsername
		isAuthUsernameUpdateNeeded = true
	}

	var isAuthPasswordUpdateNeeded bool
	if authPassword != dbr.AuthPassword {
		l.Info("AuthPassword changed")
		dbr.AuthPassword = authPassword
		isAuthPasswordUpdateNeeded = true
	}

	var isAuthSSHKeyUpdateNeeded bool
	if authSSHKey != dbr.AuthSSHKey {
		l.Info("AuthSSHKey changed")
		dbr.AuthSSHKey = authSSHKey
		isAuthSSHKeyUpdateNeeded = true
	}

	return isSecretUpdateNeeded || isRepoUpdateNeeded || isAuthUsernameUpdateNeeded ||
		isAuthPasswordUpdateNeeded || isAuthSSHKeyUpdateNeeded
}

func internalProcessMessage_GetGitopsEngineInstanceById(ctx context.Context, id string, dbq db.DatabaseQueries) (*db.GitopsEngineInstance, error) {

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: id,
	}

	err := dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)
	return &gitopsEngineInstance, err
}

// The bool return value is 'true' if ClusterUser is created; 'false' if it already exists in DB or in case of failure.
func internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx context.Context, workspaceNamespace corev1.Namespace,
	dbq db.DatabaseQueries, l logr.Logger) (*db.ClusterUser, bool, error) {
	isNewUser := false

	clusterUser := db.ClusterUser{User_name: string(workspaceNamespace.UID)}

	err := dbq.GetClusterUserByUsername(ctx, &clusterUser)
	if err != nil {
		if db.IsResultNotFoundError(err) {
			isNewUser = true
			clusterUser.Display_name = workspaceNamespace.Name
			if err := dbq.CreateClusterUser(ctx, &clusterUser); err != nil {
				l.Error(err, "Unable to create ClusterUser with User ID: "+clusterUser.Clusteruser_id, clusterUser.GetAsLogKeyValues()...)
				return nil, false, err
			}
			l.Info("Created Cluster User with User ID: "+clusterUser.Clusteruser_id, clusterUser.GetAsLogKeyValues()...)

		} else {
			return nil, false, err
		}
	} else if clusterUser.Display_name == "" {
		clusterUser.Display_name = workspaceNamespace.Name
		if err := dbq.UpdateClusterUser(ctx, &clusterUser); err != nil {
			l.Error(err, "Unable to update ClusterUser with User ID: "+clusterUser.Clusteruser_id, clusterUser.GetAsLogKeyValues()...)
			return nil, false, err
		}
		l.Info("Updated Cluster User with User ID: "+clusterUser.Clusteruser_id, clusterUser.GetAsLogKeyValues()...)
	}

	return &clusterUser, isNewUser, nil
}

const (
	serviceAccountNamespaceKubeSystem = "kube-system"
)

// Ensure the user's workspace is configured, ensure a GitOpsEngineInstance exists that will target it, and ensure
// a cluster access exists the give the user permission to target them from the engine.
func internalProcessMessage_GetOrCreateSharedResources(ctx context.Context, gitopsEngineClient client.Client,
	workspaceNamespace corev1.Namespace, dbQueries db.DatabaseQueries,
	l logr.Logger) (SharedResourceManagedEnvContainer, connectionInitializedCondition, error) {

	clusterUser, isNewUser, err := internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx, workspaceNamespace, dbQueries, l)
	if err != nil {
		return SharedResourceManagedEnvContainer{}, createUnknownErrorEnvInitCondition(),
			fmt.Errorf("unable to retrieve cluster user in processMessage, '%s': %w", string(workspaceNamespace.UID), err)
	}

	managedEnv, isNewManagedEnv, err := dbutil.GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspaceNamespace, dbQueries, l)
	if err != nil {
		return SharedResourceManagedEnvContainer{}, createUnknownErrorEnvInitCondition(),
			fmt.Errorf("unable to get or created managed env on deployment modified event: %w", err)
	}

	engineInstance, isNewInstance, gitopsEngineCluster, uerr := internalDetermineGitOpsEngineInstance(ctx, *clusterUser, gitopsEngineClient, dbQueries, l)
	if uerr != nil {
		return SharedResourceManagedEnvContainer{},
			createUnknownErrorEnvInitCondition(), fmt.Errorf("unable to determine gitops engine instance: %w", uerr.DevError())
	}

	// Create the cluster access object, to allow us to interact with the GitOpsEngine and ManagedEnvironment on the user's behalf
	ca := db.ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnv.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: engineInstance.Gitopsengineinstance_id,
	}

	isNewClusterAccess, err := internalGetOrCreateClusterAccess(ctx, &ca, dbQueries, l)
	if err != nil {
		return SharedResourceManagedEnvContainer{}, createUnknownErrorEnvInitCondition(), fmt.Errorf("unable to create cluster access: %v", err)
	}

	return SharedResourceManagedEnvContainer{
		ClusterUser:          clusterUser,
		IsNewUser:            isNewUser,
		ManagedEnv:           managedEnv,
		IsNewManagedEnv:      isNewManagedEnv,
		GitopsEngineInstance: engineInstance,
		IsNewInstance:        isNewInstance,
		ClusterAccess:        &ca,
		IsNewClusterAccess:   isNewClusterAccess,
		GitopsEngineCluster:  gitopsEngineCluster,
	}, connectionInitializedCondition{}, nil

}

// Whenever a new Argo CD Application needs to be created, we need to find an Argo CD instance
// that is available to use it. In the future, when we have multiple instances, there would
// be an algorithm that intelligently places applications -> instances, to ensure that
// there are no Argo CD instances that are overloaded (have too many users).
//
// However, at the moment we are using a single shared Argo CD instnace, so we will
// just return that.
//
// The bool return value is 'true' if GitOpsEngineInstance is created; 'false' if it already exists in DB or in case of failure.
//
// This logic would be improved by https://issues.redhat.com/browse/GITOPSRVCE-73 (and others)
func internalDetermineGitOpsEngineInstance(ctx context.Context, user db.ClusterUser, k8sClient client.Client, dbq db.DatabaseQueries, l logr.Logger) (*db.GitopsEngineInstance, bool, *db.GitopsEngineCluster, gitopserrors.ConditionError) {
	// TODO: GITOPSRVCE-73 - Once we have a way to distribute work between Argo CD instances, update this function.
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: dbutil.GetGitOpsEngineSingleInstanceNamespace()}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		devError := fmt.Errorf("unable to retrieve gitopsengine namespace in internalDetermineGitOpsEngineInstanceForNewApplication: %w", err)
		userMsg := gitopserrors.UnknownError
		return nil, false, nil, gitopserrors.NewUserConditionError(userMsg, devError, string(managedgitopsv1alpha1.ConditionReasonKubeError))
	}

	kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubeSystemNamespace), kubeSystemNamespace); err != nil {
		devError := fmt.Errorf("unable to retrieve kube-system namespace in internalDetermineGitOpsEngineInstanceForNewApplication: %w", err)
		userMsg := gitopserrors.UnknownError
		return nil, false, nil, gitopserrors.NewUserConditionError(userMsg, devError, string(managedgitopsv1alpha1.ConditionReasonKubeError))
	}

	gitopsEngineInstance, isNewInstance, gitopsEngineCluster, err := dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, string(kubeSystemNamespace.UID), dbq, l)
	if err != nil {
		devError := fmt.Errorf("unable to get or create engine instance for new application: %w", err)
		userMsg := gitopserrors.UnknownError
		return nil, false, nil, gitopserrors.NewUserConditionError(userMsg, devError, string(managedgitopsv1alpha1.ConditionReasonDatabaseError))
	}

	// When we support multiple Argo CD instance, the algorithm would initially be:
	//
	// algorithm input:
	// - user
	//
	// output:
	// - gitops engine instance
	//
	// In a way that ensures that applications are balanced between instances.
	// Preliminary thoughts: https://docs.google.com/document/d/15E8d5frNuTFEdCHMlNSk0LQr6DI7BtiypxIC2AnW-OQ/edit#

	return gitopsEngineInstance, isNewInstance, gitopsEngineCluster, nil
}

// The bool return value is 'true' if ClusterAccess is created; 'false' if it already exists in DB or in case of failure.
func internalGetOrCreateClusterAccess(ctx context.Context, ca *db.ClusterAccess, dbq db.DatabaseQueries, l logr.Logger) (bool, error) {

	if err := dbq.GetClusterAccessByPrimaryKey(ctx, ca); err != nil {

		if !db.IsResultNotFoundError(err) {
			return false, err
		}
	} else {
		return false, nil
	}

	if err := dbq.CreateClusterAccess(ctx, ca); err != nil {
		l.Error(err, "Unable to create ClusterAccess", ca.GetAsLogKeyValues()...)

		return false, err
	}
	l.Info(fmt.Sprintf("Created ClusterAccess for UserID: %s, for ManagedEnvironment: %s", ca.Clusteraccess_user_id,
		ca.Clusteraccess_managed_environment_id), ca.GetAsLogKeyValues()...)

	return true, nil
}
