package db

import (
	"context"
	"errors"
	"fmt"
)

var (
	errCreateRepositoryCredentials = errors.New("cannot insert repository credentials")
	errDeleteRepositoryCredentials = errors.New("cannot delete repository credentials")
	errGetRepositoryCredentials    = errors.New("cannot find repository credentials")
	errRowsAffected                = errors.New("unexpected number of affected rows")
)

func (dbq *PostgreSQLDatabaseQueries) CreateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error {
	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("%v: %w", errCreateRepositoryCredentials, err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: %d", errRowsAffected, result.RowsAffected())
	}

	return err
}

func (dbq *PostgreSQLDatabaseQueries) DeleteRepositoryCredentialsByID(ctx context.Context, id string) error {
	if err := validateQueryParams(id, dbq); err != nil {
		return err
	}

	repoCred := &RepositoryCredentials{
		PrimaryKeyID: id,
	}

	// Delete the database row, if PK is valid.
	result, err := dbq.dbConnection.Model(repoCred).WherePK().Context(ctx).Delete()
	if err != nil {
		return fmt.Errorf("%v: %w", errDeleteRepositoryCredentials, err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: %d", errRowsAffected, result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetRepositoryCredentialsByID(ctx context.Context, id string) (repoCred RepositoryCredentials, err error) {
	if err = validateQueryParams(id, dbq); err != nil {
		return repoCred, err
	}

	repoCred = RepositoryCredentials{
		PrimaryKeyID: id,
	}

	err = dbq.dbConnection.Model(&repoCred).WherePK().Context(ctx).Select()
	if err != nil {
		return repoCred, fmt.Errorf("%v: %w", errGetRepositoryCredentials, err)
	}

	return repoCred, nil
}
