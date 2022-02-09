package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/emicklei/go-restful/v3/log"
	"github.com/go-pg/pg/v10"
	"github.com/google/uuid"
)

// TODO: GITOPS-1678 - ENHANCEMENT - Add logging of database entity creation, so that we can track state changes.

// Default vs Unchecked vs Unsafe functions:
//
// Default:
// - Functions without a prefix take an ownerId string, which is a database reference to a clusterUser.
// - This clusterUser is user to verify that the user should have access to the database resource.
//
// Unchecked:
// - Functions with the 'Unchecked' prefix do not take an ownerId.
// - They thus do not verify that the user is able to access the database resources they are requesting.
//
// Unsafe:
// - Functions with the 'Unchecked' prefix should NEVER be used in production code; it should only be used in
//   test code, or WIP code.
// - If you use unsafe in a PR, and that code is outside of test code, it is very likely that you will be asked
//   to remove it (e.g. switch to normal or unchecked).
// - A database query is 'unsafe' (in a security context), and therefore only useful for debug/tests, if
//   it queries the entire database rather than being scoped to a particular user.
//
// STRATEGY: You should use a 'Default' function _if possible_. But, in cases where there is no user, or
//           during setup, you may need to use unchecked.
// - If you 'Unchecked', be prepared to justify why you could not use a 'Default' function. (HOWEVER: I am
//   still not sure how often default functions will be usefuol, so this will be an experiment/learning
//   experience for all of us :) - jgw)
// - 'Unsafe' should only be used in test code.

type UnsafeDatabaseQueries interface {
	UnsafeListAllApplications(ctx context.Context, applications *[]Application) error
	UnsafeListAllApplicationStates(ctx context.Context, applicationStates *[]ApplicationState) error
	UnsafeListAllClusterAccess(ctx context.Context, clusterAccess *[]ClusterAccess) error
	UnsafeListAllClusterCredentials(ctx context.Context, clusterCredentials *[]ClusterCredentials) error
	UnsafeListAllClusterUsers(ctx context.Context, clusterUsers *[]ClusterUser) error
	UnsafeListAllGitopsEngineInstances(ctx context.Context, gitopsEngineInstances *[]GitopsEngineInstance) error
	UnsafeListAllManagedEnvironments(ctx context.Context, managedEnvironments *[]ManagedEnvironment) error
	UnsafeListAllOperations(ctx context.Context, operations *[]Operation) error
	UnsafeListAllGitopsEngineClusters(ctx context.Context, gitopsEngineClusters *[]GitopsEngineCluster) error
}

type AllDatabaseQueries interface {
	UnsafeDatabaseQueries
	DatabaseQueries
}

type DatabaseQueries interface {
	ApplicationScopedQueries

	CreateClusterAccess(ctx context.Context, obj *ClusterAccess) error
	CreateClusterCredentials(ctx context.Context, obj *ClusterCredentials) error
	CreateClusterUser(ctx context.Context, obj *ClusterUser) error
	CreateGitopsEngineCluster(ctx context.Context, obj *GitopsEngineCluster) error
	CreateGitopsEngineInstance(ctx context.Context, obj *GitopsEngineInstance) error
	CreateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error
	CreateKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error

	DeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string, ownerId string) (int, error)

	// TODO: GITOPS-1678 - DEBT - I think this should still have an owner, even if it presumed that it is user id:
	DeleteClusterAccessById(ctx context.Context, userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error)
	DeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error)

	// Get functions return a single result, or an error if no results were present;
	// check the error with 'IsResultNotFoundError' to identify resource not found errors (vs other more serious errors).
	GetApplicationById(ctx context.Context, application *Application, ownerId string) error
	GetClusterCredentialsById(ctx context.Context, clusterCredentials *ClusterCredentials, ownerId string) error
	GetClusterUserById(ctx context.Context, clusterUser *ClusterUser) error
	GetClusterUserByUsername(ctx context.Context, clusterUser *ClusterUser) error
	GetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster, ownerId string) error
	GetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance, ownerId string) error
	GetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment, ownerId string) error
	GetOperationById(ctx context.Context, operation *Operation, ownerId string) error
	GetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping, ownerId string) error
	GetClusterAccessByPrimaryKey(ctx context.Context, obj *ClusterAccess) error
	GetDBResourceMappingForKubernetesResource(ctx context.Context, obj *KubernetesToDBResourceMapping) error

	// See definition of unchecked, above.
	// I'm still figuring out the difference between unchecked and unsafe: For now, they are very similar.
	// - In the best case, both are useful and can co-exist.
	// - In the worst case, the idea of unsafe is fundamentally flawed due to cycles in the table model. - jgwest
	UncheckedGetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance) error
	UncheckedGetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster) error
	UncheckedGetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment) error

	UncheckedDeleteKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) (int, error)
	UncheckedDeleteClusterCredentialsById(ctx context.Context, id string) (int, error)
	UncheckedDeleteClusterUserById(ctx context.Context, id string) (int, error)
	UncheckedDeleteGitopsEngineClusterById(ctx context.Context, id string) (int, error)

	UncheckedGetClusterCredentialsById(ctx context.Context, clusterCreds *ClusterCredentials) error

	UncheckedGetDeploymentToApplicationMappingByApplicationId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error

	UncheckedDeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error)

	UncheckedDeleteManagedEnvironmentById(ctx context.Context, id string) (int, error)

	// List functions return zero or more results. If no results are found (and no errors occurred), an empty slice is set in the result parameter.
	ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx context.Context, engineClusterId string, ownerId string, gitopsEngineInstancesParam *[]GitopsEngineInstance) error
	ListClusterCredentialsByHost(ctx context.Context, hostName string, clusterCredentials *[]ClusterCredentials, ownerId string) error
	ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx context.Context, clusterCredentialId string, ownerId string, managedEnvironments *[]ManagedEnvironment) error
	ListGitopsEngineClusterByCredentialId(ctx context.Context, credentialId string, engineClustersParam *[]GitopsEngineCluster, ownerId string) error
}

// ApplicationScopedQueries are the set of database queries that act on application DB resources:
// - Application
// - ApplicateState
// - Operation
// - SyncOperation
// - APICRToDatabaseMapping
// - DeploymentToApplicationMapping
//
// Application resources are the resources that:
// - will never be shared between GitOpsDeployment(/SyncRun) objects
// - The lifetime of these resources will be equivalent to the lifetime of a single parent application/gitopsdeployment/gitopsdeploymentsyncrn
//
// Resources that are not application resources, are those can be shared between multiple applications/gitopsdeployments,
// and thus we must be careful when making concurrent modifications to them:
// - managed environment
// - gitops engine instance
// - gitops engine cluster
// - cluster credentials
// - cluster user
// - kubernetesresourcetobmapping
//
// For example: multiple gitopsdeployments must reference a single gitops engine instance, or a single target managed environment.
type ApplicationScopedQueries interface {
	CloseableQueries

	DeleteOperationById(ctx context.Context, id string, ownerId string) (int, error)
	CreateOperation(ctx context.Context, obj *Operation, ownerId string) error
	UncheckedGetOperationById(ctx context.Context, operation *Operation) error
	UncheckedDeleteOperationById(ctx context.Context, id string) (int, error)
	ListOperationsByResourceIdAndTypeAndOwnerId(ctx context.Context, resourceID string, resourceType string, operations *[]Operation, ownerId string) error

	UncheckedCreateSyncOperation(ctx context.Context, obj *SyncOperation) error
	UncheckedDeleteSyncOperationById(ctx context.Context, id string) (int, error)
	UncheckedGetSyncOperationById(ctx context.Context, syncOperation *SyncOperation) error

	UncheckedDeleteApplicationById(ctx context.Context, id string) (int, error)
	DeleteApplicationById(ctx context.Context, id string, ownerId string) (int, error)
	CreateApplication(ctx context.Context, obj *Application, ownerId string) error
	UncheckedGetApplicationById(ctx context.Context, application *Application) error
	UncheckedUpdateApplication(ctx context.Context, obj *Application) error

	UncheckedDeleteApplicationStateById(ctx context.Context, id string) (int, error)

	CreateAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) error
	UncheckedListAPICRToDatabaseMappingByAPINamespaceAndName(ctx context.Context, apiCRResourceType string, crName string, crNamespace string, crWorkspaceUID string, dbRelationType string, apiCRToDBMappingParam *[]APICRToDatabaseMapping) error
	GetDatabaseMappingForAPICR(ctx context.Context, obj *APICRToDatabaseMapping) error
	UncheckedDeleteAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) (int, error)

	CreateDeploymentToApplicationMapping(ctx context.Context, obj *DeploymentToApplicationMapping) error
	UncheckedGetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error
	UncheckedListDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, workspaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error
	UncheckedListDeploymentToApplicationMappingByWorkspaceUID(ctx context.Context, workspaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error
	UncheckedDeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string) (int, error)

	UpdateSyncOperationRemoveApplicationField(ctx context.Context, applicationId string) (int, error)

	UncheckedGetApplicationStateById(ctx context.Context, obj *ApplicationState) error
	UncheckedCreateApplicationState(ctx context.Context, obj *ApplicationState) error
	UncheckedUpdateApplicationState(ctx context.Context, obj *ApplicationState) error

	UncheckedUpdateOperation(ctx context.Context, obj *Operation) error
	UncheckedDeleteDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, workspaceUID string) (int, error)
}

type CloseableQueries interface {
	CloseDatabase()
}

var _ UnsafeDatabaseQueries = &PostgreSQLDatabaseQueries{}
var _ DatabaseQueries = &PostgreSQLDatabaseQueries{}

type PostgreSQLDatabaseQueries struct {
	dbConnection *pg.DB

	// allowTestUuids, if true, will allow callers to pass an id value into the db create methods.
	// This is useful for test cases, and this setting must only be enabled for unit tests.
	allowTestUuids bool

	// A database query is 'unsafe' (in a security context), and therefore only useful
	// for debug/tests if:
	// - it queries the entire database rather than being scoped to a particular user.
	// - or, it returns values that should not be accessible in the context of serving a
	//   particular user (for example, that user should not have access to that database
	//   resource)
	//
	// This should be false in all cases, with the only exception being test code.
	allowUnsafe bool
}

func NewProductionPostgresDBQueries(verbose bool) (DatabaseQueries, error) {
	return NewProductionPostgresDBQueriesWithport(verbose, DEFAULT_PORT)
}

func NewProductionPostgresDBQueriesWithport(verbose bool, port int) (DatabaseQueries, error) {
	db, err := connectToDatabaseWithPort(verbose, "postgres", port)
	if err != nil {
		return nil, err
	}

	dbq := &PostgreSQLDatabaseQueries{
		dbConnection:   db,
		allowTestUuids: false,
		allowUnsafe:    false,
	}

	return dbq, nil

}

func NewUnsafePostgresDBQueries(verbose bool, allowTestUuids bool) (AllDatabaseQueries, error) {
	return NewUnsafePostgresDBQueriesWithPort(verbose, allowTestUuids, DEFAULT_PORT)
}

func NewUnsafePostgresDBQueriesWithPort(verbose bool, allowTestUuids bool, port int) (AllDatabaseQueries, error) {
	db, err := connectToDatabaseWithPort(verbose, "postgres", port)
	if err != nil {
		return nil, err
	}

	dbq := &PostgreSQLDatabaseQueries{
		dbConnection:   db,
		allowTestUuids: allowTestUuids,
		allowUnsafe:    true,
	}

	fmt.Printf("* WARNING: Unsafe PostgreSQLDB object was created. You should never see this outside of test suites, or personal development.\n")

	return dbq, nil
}

func (dbq *PostgreSQLDatabaseQueries) CloseDatabase() {

	if dbq.dbConnection != nil {
		// Close closes the database client, releasing any open resources.
		//
		// It is rare to Close a DB, as the DB handle is meant to be
		// long-lived and shared between many goroutines.
		err := dbq.dbConnection.Close()
		if err != nil {
			log.Printf("Error occurred on CloseDatabase(): %v", err)
		}
	}
}

// NewResultNotFoundError returns an error that will be matched by IsAccessDeniedError
func NewAccessDeniedError(errString string) error {
	return fmt.Errorf("%s: results found, but access denied", errString)
}

func IsAccessDeniedError(errorParam error) bool {
	return strings.Contains(errorParam.Error(), "results found, but access denied")
}

// NewResultNotFoundError returns an error that will be matched by IsResultNotFoundError
func NewResultNotFoundError(errString string) error {
	return fmt.Errorf("%s: no rows in result set", errString)
}

func IsResultNotFoundError(errorParam error) bool {
	return strings.Contains(errorParam.Error(), "no rows in result set")
}

func generateUuid() string {
	return uuid.New().String()
}

func isEmpty(str string) bool {
	return len(strings.TrimSpace(str)) == 0
}
