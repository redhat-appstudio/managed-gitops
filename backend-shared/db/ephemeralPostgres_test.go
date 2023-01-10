//go:build !skip
// +build !skip

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Note: This function is not tested in an openshift-ci  enviroment
func TestEphemeralCode(t *testing.T) {
	skipOpenshiftCI(t)
	ctx := context.Background()
	// Create ephemeral DB
	ephemeralDB, err := NewEphemeralCreateTestFramework()
	if !assert.NoError(t, err) {
		return
	}

	dbq := ephemeralDB.db
	defer dbq.CloseDatabase()

	// test code as normal, for example:
	var applicationStates []ApplicationState
	err = dbq.UnsafeListAllApplicationStates(ctx, &applicationStates)
	assert.NoError(t, err)

	errClean := ephemeralDB.Dispose()
	assert.NoError(t, errClean)
}
