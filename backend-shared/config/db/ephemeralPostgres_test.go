// +build !skip

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEphemeralCode(t *testing.T) {

	ctx := context.Background()
	// Create ephemeral DB
	ephemeralDB, err := NewEphemeralCreateTestFramework()
	assert.NoError(t, err)

	dbq := ephemeralDB.db
	defer dbq.CloseDatabase()

	// test code as normal, for example:
	var applicationStates []ApplicationState
	err = dbq.UnsafeListAllApplicationStates(ctx, &applicationStates)
	assert.NoError(t, err)

	errClean := ephemeralDB.Dispose()
	assert.NoError(t, errClean)
}
