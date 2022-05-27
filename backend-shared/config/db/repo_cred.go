package db

import (
	"context"
	"errors"
	"fmt"
)

var (
	errCreateRepositoryCredentials = errors.New("cannot insert repository credentials")
	errRowsAffected                = errors.New("unexpected number of rows affected")
)

func (dbq *PostgreSQLDatabaseQueries) CreateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error {
	// TODO: See similar tests and figure out what else is needed
	// TODO: If it actually is needed here and the tests didn't catch it
	// then it is either not required OR test is missing for it.

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("%v: %w", errCreateRepositoryCredentials, err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: %d", errRowsAffected, result.RowsAffected())
	}

	return err
}
