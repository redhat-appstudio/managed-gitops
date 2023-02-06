package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-pg/pg/v10"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Default vs Checked vs Unsafe functions:
//
// Default:
// - Functions with no prefix and do not take an ownerId.
// - They thus do not verify that the user is able to access the database resources they are requesting.
// - They rely on the calling function to ensure that the user is authorized to perform a particular task.
//
// Checked (experimental):
// - Functions without a prefix take an ownerId string, which is a database reference to a clusterUser.
// - This clusterUser is used to verify that the user should have access to the database resource.
// - How this verification is performed differs based on the database resource.
//
// Unsafe:
// - Functions with the 'Unsafe' prefix should NEVER be used in production code; it should only be used in
//   test code, or WIP code.
// - If you use unsafe in a PR, and that code is outside of test code, it is very likely that it should be
//   removed (e.g. switch to normal or checked).
// - A database query is 'unsafe' (in a security context), and therefore only useful for debug/tests, if
//   it queries the entire database rather than being scoped to a particular user.
//
// STRATEGY: You should use a 'Default' function _where possible_.
// - Checked functions were an interesting idea, and we may reexamine them in the future, but for
//   the moment they have problems: potential for heavy performance load, high cognitive load, cycle in the table model,
//   and unergonomic API (no way to distinguish between an unauthorized resource and a missing
//   resource).
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
	UnsafeListAllDeploymentToApplicationMapping(ctx context.Context, deploymentToApplicationMappings *[]DeploymentToApplicationMapping) error
	UnsafeListAllSyncOperations(ctx context.Context, syncOperations *[]SyncOperation) error
	UnsafeListAllKubernetesResourceToDBResourceMapping(ctx context.Context, kubernetesToDBResourceMapping *[]KubernetesToDBResourceMapping) error
	UnsafeListAllAPICRToDatabaseMappings(ctx context.Context, mappings *[]APICRToDatabaseMapping) error
	UnsafeListAllRepositoryCredentials(ctx context.Context, repositoryCredentials *[]RepositoryCredentials) error
}

type AllDatabaseQueries interface {
	UnsafeDatabaseQueries
	DatabaseQueries
}

type DatabaseQueries interface {
	ApplicationScopedQueries

	CreateClusterAccess(ctx context.Context, obj *ClusterAccess) error
	CreateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error
	UpdateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error
	CreateClusterCredentials(ctx context.Context, obj *ClusterCredentials) error
	CreateClusterUser(ctx context.Context, obj *ClusterUser) error
	CreateGitopsEngineCluster(ctx context.Context, obj *GitopsEngineCluster) error
	CreateGitopsEngineInstance(ctx context.Context, obj *GitopsEngineInstance) error
	CreateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error
	CreateKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error

	CheckedDeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string, ownerId string) (int, error)

	DeleteClusterAccessById(ctx context.Context, userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error)
	CheckedDeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error)
	CheckedDeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error)

	// Get functions return a single result, or an error if no results were present;
	// check the error with 'IsResultNotFoundError' to identify resource not found errors (vs other more serious errors).
	CheckedGetApplicationById(ctx context.Context, application *Application, ownerId string) error
	CheckedGetClusterCredentialsById(ctx context.Context, clusterCredentials *ClusterCredentials, ownerId string) error
	GetClusterUserById(ctx context.Context, clusterUser *ClusterUser) error
	GetClusterUserByUsername(ctx context.Context, clusterUser *ClusterUser) error

	// Get or Create a user which can be used internally by gitops-service only. If we need to perform any operation or create resources for gitops-service purposes,
	// we will use special user (dummy user/internal user) details.
	GetOrCreateSpecialClusterUser(ctx context.Context, clusterUser *ClusterUser) error
	CheckedGetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster, ownerId string) error
	CheckedGetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance, ownerId string) error
	CheckedGetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment, ownerId string) error
	CheckedGetOperationById(ctx context.Context, operation *Operation, ownerId string) error
	CheckedGetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping, ownerId string) error
	GetClusterAccessByPrimaryKey(ctx context.Context, obj *ClusterAccess) error
	GetDBResourceMappingForKubernetesResource(ctx context.Context, obj *KubernetesToDBResourceMapping) error

	GetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster) error
	GetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment) error
	GetRepositoryCredentialsByID(ctx context.Context, id string) (obj RepositoryCredentials, err error)

	DeleteKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) (int, error)
	DeleteClusterCredentialsById(ctx context.Context, id string) (int, error)
	DeleteClusterUserById(ctx context.Context, id string) (int, error)
	DeleteGitopsEngineClusterById(ctx context.Context, id string) (int, error)

	// Delete RepositoryCredentials row by ID
	DeleteRepositoryCredentialsByID(ctx context.Context, id string) (int, error)

	GetClusterCredentialsById(ctx context.Context, clusterCreds *ClusterCredentials) error

	GetDeploymentToApplicationMappingByApplicationId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error

	// Get DeploymentToApplicationMappings in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
	GetDeploymentToApplicationMappingBatch(ctx context.Context, deploymentToApplicationMappings *[]DeploymentToApplicationMapping, limit, offSet int) error

	UpdateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error
	DeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error)

	// Delete ManagedEnvironment row by ID
	DeleteManagedEnvironmentById(ctx context.Context, id string) (int, error)

	// List functions return zero or more results. If no results are found (and no errors occurred), an empty slice is set in the result parameter.
	CheckedListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx context.Context, engineClusterId string, ownerId string, gitopsEngineInstancesParam *[]GitopsEngineInstance) error
	CheckedListClusterCredentialsByHost(ctx context.Context, hostName string, clusterCredentials *[]ClusterCredentials, ownerId string) error
	ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx context.Context, clusterCredentialId string, ownerId string, managedEnvironments *[]ManagedEnvironment) error
	CheckedListGitopsEngineClusterByCredentialId(ctx context.Context, credentialId string, engineClustersParam *[]GitopsEngineCluster, ownerId string) error

	// RemoveManagedEnvironmentFromAllApplications update the 'managed_environment_id' field to null
	// for all Applications that reference a specific managed environment. This function is used while
	// deleting a managed environment.
	//
	// Note: this function is not guaranteed to update all applications: it is possible that another thread
	//       could create an Application after step 1. The logic of calling functions should expect and
	//       handle this behaviour.
	RemoveManagedEnvironmentFromAllApplications(ctx context.Context, managedEnvironmentID string, applications *[]Application) (int, error)

	ListClusterAccessesByManagedEnvironmentID(ctx context.Context, managedEnvironmentID string, clusterAccesses *[]ClusterAccess) error

	// ListApplicationsForManagedEnvironment returns a list of all Applications that reference the specified ManagedEnvironment row
	ListApplicationsForManagedEnvironment(ctx context.Context, managedEnvironmentID string, applications *[]Application) (int, error)

	// ListGitopsEngineInstancesForCluster lists the GitOpsEngineInstances that are on the given GitOpsEngineCluster
	ListGitopsEngineInstancesForCluster(ctx context.Context, gitopsEngineCluster GitopsEngineCluster, gitopsEngineInstances *[]GitopsEngineInstance) error
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

	UpdateOperation(ctx context.Context, obj *Operation) error

	CreateOperation(ctx context.Context, obj *Operation, ownerId string) error
	GetOperationById(ctx context.Context, operation *Operation) error
	ListOperationsByResourceIdAndTypeAndOwnerId(ctx context.Context, resourceID string, resourceType OperationResourceType,
		operations *[]Operation, ownerId string) error
	CheckedDeleteOperationById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteOperationById(ctx context.Context, id string) (int, error)

	// ListOperationsToBeGarbageCollected returns 'Failed'/'Completed' operations with a non-zero garbage collection expiration time
	ListOperationsToBeGarbageCollected(ctx context.Context, operations *[]Operation) error

	CreateSyncOperation(ctx context.Context, obj *SyncOperation) error
	GetSyncOperationById(ctx context.Context, syncOperation *SyncOperation) error
	DeleteSyncOperationById(ctx context.Context, id string) (int, error)
	UpdateSyncOperation(ctx context.Context, obj *SyncOperation) error

	CreateApplication(ctx context.Context, obj *Application) error
	CheckedCreateApplication(ctx context.Context, obj *Application, ownerId string) error
	GetApplicationById(ctx context.Context, application *Application) error
	UpdateApplication(ctx context.Context, obj *Application) error
	DeleteApplicationById(ctx context.Context, id string) (int, error)
	CheckedDeleteApplicationById(ctx context.Context, id string, ownerId string) (int, error)

	// Get applications in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
	GetApplicationBatch(ctx context.Context, applications *[]Application, limit, offSet int) error

	// TODO: GITOPSRVCE-19 - KCP support: All of the *ByAPINamespaceAndName database queries should only return items that are part of a specific KCP workspace.

	CreateAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) error

	// Get APICRToDatabaseMapping in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
	GetAPICRToDatabaseMappingBatch(ctx context.Context, apiCRToDatabaseMapping *[]APICRToDatabaseMapping, limit, offSet int) error

	// ListAPICRToDatabaseMappingByAPINamespaceAndName returns the DBRelationKey for a given type/name/namespace/namespace uid/db-relation-type query
	ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx context.Context, apiCRResourceType APICRToDatabaseMapping_ResourceType,
		crName string, crNamespace string, crNamespaceUID string, dbRelationType APICRToDatabaseMapping_DBRelationType,
		apiCRToDBMappingParam *[]APICRToDatabaseMapping) error

	GetDatabaseMappingForAPICR(ctx context.Context, obj *APICRToDatabaseMapping) error
	DeleteAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) (int, error)

	CreateDeploymentToApplicationMapping(ctx context.Context, obj *DeploymentToApplicationMapping) error
	GetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error
	ListDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, namespaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error

	// ListDeploymentToApplicationMappingByNamespaceUID lists all DTAMs that are in a namespace with the given UID
	ListDeploymentToApplicationMappingByNamespaceUID(ctx context.Context, namespaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error

	DeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string) (int, error)
	DeleteDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, namespaceUID string) (int, error)

	// UpdateSyncOperationRemoveApplicationField locates any SyncOperations that reference 'applicationID', and sets the
	// applicationID field to nil.
	UpdateSyncOperationRemoveApplicationField(ctx context.Context, applicationId string) (int, error)

	GetApplicationStateById(ctx context.Context, obj *ApplicationState) error
	CreateApplicationState(ctx context.Context, obj *ApplicationState) error
	UpdateApplicationState(ctx context.Context, obj *ApplicationState) error
	DeleteApplicationStateById(ctx context.Context, id string) (int, error)

	GetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment) error

	GetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance) error

	// GetAPICRForDatabaseUID retrieves the name/namespace/uid of an API Resources (such as GitOpsDeploymentManagedEnvironment)
	// based on the primary key of the corresponding database row (for example, ManagedEnvironment)
	GetAPICRForDatabaseUID(ctx context.Context, apiCRToDatabaseMapping *APICRToDatabaseMapping) error
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
	// - it queries the entire database rather than being scoped to a particular user or subset of values.
	//
	// This should be false in all cases, with the only exception being test code.
	allowUnsafe bool

	// allowClose: if true, calling Close on PostgreSQLDatabaseQueries will close the connection pool; if false,
	// the close operation will be ignored.
	allowClose bool
}

var internalSharedDBEntity internalSharedDBConnectionPool

const (
	sharedDBPoolMapKey_standard = "standard"
	sharedDBPoolMapKey_verbose  = "verbose"
)

// internalSharedDBConnectionPool maintains a list of DatabaseQueries, at present, one for verbose log output, and one for non-verbose log output
type internalSharedDBConnectionPool struct {
	mutex sync.Mutex

	// At present, we maintain two separate pools: one for verbose log output, and one for non-verbose log output
	// key is 'sharedDBPoolMapKey_*'
	pools map[string]DatabaseQueries
}

// NewSharedProductionPostgresDBQueries returns a connection to the database using go-pg's built-in connection pooling
// functionality.
func NewSharedProductionPostgresDBQueries(verbose bool) (DatabaseQueries, error) {

	internalSharedDBEntity.mutex.Lock()
	defer internalSharedDBEntity.mutex.Unlock()

	if internalSharedDBEntity.pools == nil {
		// Ensure the pools map is intialized
		internalSharedDBEntity.pools = map[string]DatabaseQueries{}
	}

	// At present, we maintain two pools: one for verbose output, and one for non-verbose output
	mapKey := sharedDBPoolMapKey_standard
	if verbose {
		mapKey = sharedDBPoolMapKey_verbose
	}

	dbQueries, exists := internalSharedDBEntity.pools[mapKey]
	if !exists {
		// If we haven't created a database connection pool for this mapKey yes, then create one.
		var err error
		dbQueries, err = internalNewProductionPostgresDBQueriesWithPort(verbose, DEFAULT_PORT, false)
		if err != nil {
			return nil, fmt.Errorf("unable to connect to database using shared function: %v", err)
		}
		internalSharedDBEntity.pools[mapKey] = dbQueries
	}

	if os.Getenv("ENABLE_UNRELIABLE_DB") == "true" {
		return &ChaosDBClient{InnerClient: dbQueries}, nil
	}

	return dbQueries, nil
}

func internalNewProductionPostgresDBQueriesWithPort(verbose bool, port int, allowClose bool) (DatabaseQueries, error) {

	backoff := &sharedutil.ExponentialBackoff{
		Factor: 2,
		Min:    time.Duration(time.Millisecond * 200),
		Max:    time.Duration(time.Second * 30),
		Jitter: true,
	}

	var db *pg.DB

	taskError := sharedutil.RunTaskUntilTrue(context.Background(), backoff, "NewProductionPostgresDBQueries", log.FromContext(context.Background()), func() (bool, error) {

		var err error

		db, err = ConnectToDatabaseWithPort(verbose, "postgres", port)
		if err != nil {
			return false, err
		}

		return true, nil

	})

	if taskError != nil {
		return nil, fmt.Errorf("unable to acquire database: %v", taskError)
	}

	dbq := &PostgreSQLDatabaseQueries{
		dbConnection:   db,
		allowTestUuids: false,
		allowUnsafe:    false,
		allowClose:     allowClose,
	}

	return dbq, nil

}

func NewUnsafePostgresDBQueries(verbose bool, allowTestUuids bool) (AllDatabaseQueries, error) {
	return NewUnsafePostgresDBQueriesWithPort(verbose, allowTestUuids, DEFAULT_PORT)
}

func NewUnsafePostgresDBQueriesWithPort(verbose bool, allowTestUuids bool, port int) (AllDatabaseQueries, error) {

	// We don't add retry logic to this function (unlike the Production function above) because
	// we want to fail fast during tests.

	db, err := ConnectToDatabaseWithPort(verbose, "postgres", port)
	if err != nil {
		return nil, err
	}

	dbq := &PostgreSQLDatabaseQueries{
		dbConnection:   db,
		allowTestUuids: allowTestUuids,
		allowUnsafe:    true,
		allowClose:     true,
	}

	fmt.Printf("* WARNING: Unsafe PostgreSQLDB object was created. You should never see this outside of test suites, or personal development.\n")

	return dbq, nil
}

func (dbq *PostgreSQLDatabaseQueries) CloseDatabase() {

	if dbq.dbConnection != nil && dbq.allowClose {
		log := log.FromContext(context.Background())

		// Close closes the database client, releasing any open resources.
		//
		// It is rare to Close a DB, as the DB handle is meant to be
		// long-lived and shared between many goroutines.
		err := dbq.dbConnection.Close()
		if err != nil {
			log.Error(err, "Error occurred on CloseDatabase()")
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

	// Sanity test: it's possible to unintentionally call db.IsResultNotFoundError() when apierror.IsNotFound() is intended,
	// so we attempt to catch that here, and print a message.
	if apierr.IsNotFound(errorParam) {
		fmt.Println("SEVERE: IsResultNotFoundError called on k8s client error")
	}

	return strings.Contains(errorParam.Error(), "no rows in result set")
}
