package db

import (
	"fmt"
	"strings"

	"github.com/go-pg/pg/v10"
	"github.com/google/uuid"
)

// A database query is 'unsafe' (in a security context), and therefore only useful for debug/tests,
// if it queries the entire database rather than being scoped to a particular user, or the database
// rows returned by the query are not user-scoped.
//
// These should not be used, except by test code.
type UnsafeDatabaseQueries interface {
	UnsafeDeleteGitopsEngineInstanceById(id string) (int, error)
	UnsafeDeleteManagedEnvironmentById(id string) (int, error)
	UnsafeGetClusterCredentialsById(id string) (*ClusterCredentials, error)
	UnsafeListAllApplications() ([]Application, error)
	UnsafeListAllApplicationStates() ([]ApplicationState, error)
	UnsafeListAllClusterAccess() ([]ClusterAccess, error)
	UnsafeListAllClusterCredentials() ([]ClusterCredentials, error)
	UnsafeListAllClusterUsers() ([]ClusterUser, error)
	UnsafeListAllGitopsEngineClusters() ([]GitopsEngineCluster, error)
	UnsafeListAllGitopsEngineInstances() ([]GitopsEngineInstance, error)
	UnsafeListAllManagedEnvironments() ([]ManagedEnvironment, error)
	UnsafeListAllOperations() ([]Operation, error)
}

type AllDatabaseQueries interface {
	UnsafeDatabaseQueries
	DatabaseQueries
}

type DatabaseQueries interface {
	AdminDeleteClusterCredentialsById(id string) (int, error)
	AdminDeleteClusterUserById(id string) (int, error)
	AdminDeleteGitopsEngineClusterById(id string) (int, error)

	CreateClusterAccess(obj *ClusterAccess) error
	CreateClusterCredentials(obj *ClusterCredentials) error
	CreateClusterUser(obj *ClusterUser) error
	CreateGitopsEngineCluster(obj *GitopsEngineCluster) error
	CreateGitopsEngineInstance(obj *GitopsEngineInstance) error
	CreateManagedEnvironment(obj *ManagedEnvironment) error
	CreateOperation(obj *Operation, ownerId string) error

	DeleteApplicationStateById(id string) (int, error)
	DeleteApplicationById(id string) (int, error)

	DeleteClusterAccessById(userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error)
	DeleteGitopsEngineInstanceById(id string, ownerId string) (int, error)
	DeleteManagedEnvironmentById(id string, ownerId string) (int, error)
	DeleteOperationById(id string, ownerId string) (int, error)
	GetClusterCredentialsById(id string, ownerId string) (*ClusterCredentials, error)
	GetClusterUserById(id string) (*ClusterUser, error)
	GetGitopsEngineClusterById(id string, ownerId string) (*GitopsEngineCluster, error)
	GetGitopsEngineInstanceById(id string, ownerId string) (*GitopsEngineInstance, error)
	GetManagedEnvironmentById(id string, ownerId string) (*ManagedEnvironment, error)
	GetOperationById(id string, ownerId string) (*Operation, error)
	ListAllGitopsEngineInstancesByGitopsEngineCluster(engineClusterId string, ownerId string) ([]GitopsEngineInstance, error)

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
		dbq.dbConnection.Close()
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
