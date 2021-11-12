package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/emicklei/go-restful/v3/log"
	"github.com/go-pg/pg/v10"
	"github.com/google/uuid"
)

// A database query is 'unsafe' (in a security context), and therefore only useful for debug/tests,
// if it queries the entire database rather than being scoped to a particular user, or the database
// rows returned by the query are not user-scoped.
//
// These should not be used, except by test code.
type UnsafeDatabaseQueries interface {
	UnsafeDeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error)
	UnsafeDeleteManagedEnvironmentById(ctx context.Context, id string) (int, error)
	UnsafeGetClusterCredentialsById(ctx context.Context, id string) (*ClusterCredentials, error)
	UnsafeListAllApplications(ctx context.Context) ([]Application, error)
	UnsafeListAllApplicationStates(ctx context.Context) ([]ApplicationState, error)
	UnsafeListAllClusterAccess(ctx context.Context) ([]ClusterAccess, error)
	UnsafeListAllClusterCredentials(ctx context.Context) ([]ClusterCredentials, error)
	UnsafeListAllClusterUsers(ctx context.Context) ([]ClusterUser, error)
	UnsafeGetApplicationById(ctx context.Context, id string) (*Application, error)
	UnsafeListAllGitopsEngineClusters(ctx context.Context) ([]GitopsEngineCluster, error)
	UnsafeListAllGitopsEngineInstances(ctx context.Context) ([]GitopsEngineInstance, error)
	UnsafeListAllManagedEnvironments(ctx context.Context) ([]ManagedEnvironment, error)
	UnsafeListAllOperations(ctx context.Context) ([]Operation, error)
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

	DeleteApplicationStateById(ctx context.Context, id string) (int, error)
	DeleteApplicationById(ctx context.Context, id string) (int, error)

	DeleteClusterAccessById(ctx context.Context, userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error)
	DeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error)
	DeleteOperationById(ctx context.Context, id string, ownerId string) (int, error)

	GetClusterCredentialsById(ctx context.Context, id string, ownerId string) (*ClusterCredentials, error)
	GetClusterCredentialsByHost(ctx context.Context, hostName string, ownerId string) ([]ClusterCredentials, error)
	GetClusterUserById(ctx context.Context, id string) (*ClusterUser, error)
	GetClusterUserByUsername(ctx context.Context, userName string) (*ClusterUser, error)
	GetGitopsEngineClusterById(ctx context.Context, id string, ownerId string) (*GitopsEngineCluster, error)
	GetGitopsEngineClusterByCredentialId(ctx context.Context, credentialId string, ownerId string) ([]GitopsEngineCluster, error)
	GetManagedEnvironmentByClusterCredentials(ctx context.Context, clusterCredentialId string, ownerId string) ([]ManagedEnvironment, error)
	GetGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (*GitopsEngineInstance, error)
	GetManagedEnvironmentById(ctx context.Context, id string, ownerId string) (*ManagedEnvironment, error)
	GetOperationById(ctx context.Context, id string, ownerId string) (*Operation, error)
	GetDeploymentToApplicationMappingById(ctx context.Context, id string) (*DeploymentToApplicationMapping, error)

	// TODO: Should this be Get*, or should some of the Get* be List* (Get implies it returns a single result.)
	ListAllGitopsEngineInstancesByGitopsEngineCluster(ctx context.Context, engineClusterId string, ownerId string) ([]GitopsEngineInstance, error)

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
