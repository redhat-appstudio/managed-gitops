package db

import (
	"testing"

	"github.com/go-pg/pg/extra/pgdebug"
	"github.com/stretchr/testify/assert"
)

// Ensure that the we are able to select on all the fields of the database.
func TestSelectOnAllTables(t *testing.T) {

	db := ConnectToDatabase()
	db.AddQueryHook(pgdebug.DebugHook{
		// Print all queries.
		Verbose: true,
	})
	defer db.Close()

	{
		var gitopsEngineClusters []GitopsEngineCluster
		err := db.Model(&gitopsEngineClusters).Select()
		assert.NoError(t, err)
	}

	{
		var gitopsEngineInstances []GitopsEngineInstance
		err := db.Model(&gitopsEngineInstances).Select()
		assert.NoError(t, err)
	}

	{
		var managedEnvironments []ManagedEnvironment
		err := db.Model(&managedEnvironments).Select()
		assert.NoError(t, err)
	}

	{
		var clusterCredentials []ClusterCredentials
		err := db.Model(&clusterCredentials).Select()
		assert.NoError(t, err)
	}

	{
		var clusterUsers []ClusterUser
		err := db.Model(&clusterUsers).Select()
		assert.NoError(t, err)
	}

	{
		var clusterAccess []ClusterAccess
		err := db.Model(&clusterAccess).Select()
		assert.NoError(t, err)
	}

	{
		var operations []Operation
		err := db.Model(&operations).Select()
		assert.NoError(t, err)
	}

	{
		var applications []Application
		err := db.Model(&applications).Select()
		assert.NoError(t, err)
	}

	{
		var appStates []ApplicationState
		err := db.Model(&appStates).Select()
		assert.NoError(t, err)
	}

}
