package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/emicklei/go-restful/v3/log"
	"github.com/go-pg/pg/v10"
	"github.com/google/uuid"
)

// TODO: ENHANCEMENT - Add logging of database entity creation, so that we can track state changes.

// A database query is 'unsafe' (in a security context), and therefore only useful for debug/tests,
// if it queries the entire database rather than being scoped to a particular user, or the database
// rows returned by the query are not user-scoped.
//
// These should not be used, except by test code.
type UnsafeDatabaseQueries interface {
	UnsafeDeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error)
	UnsafeDeleteManagedEnvironmentById(ctx context.Context, id string) (int, error)
	UnsafeGetClusterCredentialsById(ctx context.Context, clusterCreds *ClusterCredentials) error
	UnsafeListAllApplications(ctx context.Context, applications *[]Application) error
	UnsafeListAllApplicationStates(ctx context.Context, applicationStates *[]ApplicationState) error
	UnsafeListAllClusterAccess(ctx context.Context, clusterAccess *[]ClusterAccess) error
	UnsafeListAllClusterCredentials(ctx context.Context, clusterCredentials *[]ClusterCredentials) error
	UnsafeListAllClusterUsers(ctx context.Context, clusterUsers *[]ClusterUser) error
	UnsafeGetApplicationById(ctx context.Context, application *Application) error
	UnsafeListAllGitopsEngineInstances(ctx context.Context, gitopsEngineInstances *[]GitopsEngineInstance) error
	UnsafeListAllManagedEnvironments(ctx context.Context, managedEnvironments *[]ManagedEnvironment) error
	UnsafeListAllOperations(ctx context.Context, operations *[]Operation) error
	UnsafeListAllGitopsEngineClusters(ctx context.Context, gitopsEngineClusters *[]GitopsEngineCluster) error
	UnsafeDeleteApplicationById(ctx context.Context, id string) (int, error)
}

type AllDatabaseQueries interface {
	UnsafeDatabaseQueries
	DatabaseQueries
}

type DatabaseQueries interface {
	AdminDeleteClusterCredentialsById(ctx context.Context, id string) (int, error)
	AdminDeleteClusterUserById(ctx context.Context, id string) (int, error)
	AdminDeleteGitopsEngineClusterById(ctx context.Context, id string) (int, error)

	CreateApplication(ctx context.Context, obj *Application, ownerId string) error
	CreateClusterAccess(ctx context.Context, obj *ClusterAccess) error
	CreateClusterCredentials(ctx context.Context, obj *ClusterCredentials) error
	CreateClusterUser(ctx context.Context, obj *ClusterUser) error
	CreateDeploymentToApplicationMapping(ctx context.Context, obj *DeploymentToApplicationMapping) error
	CreateGitopsEngineCluster(ctx context.Context, obj *GitopsEngineCluster) error
	CreateGitopsEngineInstance(ctx context.Context, obj *GitopsEngineInstance) error
	CreateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error
	CreateOperation(ctx context.Context, obj *Operation, ownerId string) error
	CreateKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error

	DeleteApplicationStateById(ctx context.Context, id string) (int, error)
	DeleteApplicationById(ctx context.Context, id string, ownerId string) (int, error)

	DeleteClusterAccessById(ctx context.Context, userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error)
	DeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteOperationById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) (int, error)

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

	// I'm still figuring out the difference between unchecked and unsafe: For now, they are very similar.
	// - In the best case, both are useful and can co-exist.
	// - In the worst case, the idea of unsafe is fundamentally flawed due to cycles in the table model. - jgwest

	UncheckedGetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance) error
	UncheckedGetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster) error

	// List functions return zero or more results. If no results are found (and no errors occurred), an empty slice is set in the result parameter.
	ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx context.Context, engineClusterId string, ownerId string, gitopsEngineInstancesParam *[]GitopsEngineInstance) error
	ListClusterCredentialsByHost(ctx context.Context, hostName string, clusterCredentials *[]ClusterCredentials, ownerId string) error
	ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx context.Context, clusterCredentialId string, ownerId string, managedEnvironments *[]ManagedEnvironment) error
	ListGitopsEngineClusterByCredentialId(ctx context.Context, credentialId string, engineClustersParam *[]GitopsEngineCluster, ownerId string) error

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
	db, err := connectToDatabase(true)
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
	db, err := connectToDatabase(true)
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
