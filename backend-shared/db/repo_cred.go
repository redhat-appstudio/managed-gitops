package db

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	errCreateRepositoryCredentials = errors.New("cannot create repository credentials")
	errDeleteRepositoryCredentials = errors.New("cannot delete repository credentials")
	errGetRepositoryCredentials    = errors.New("cannot find repository credentials")
	errUpdateRepositoryCredentials = errors.New("cannot update repository credentials")
	errRowsAffected                = errors.New("unexpected number of affected rows")
)

func (dbq *PostgreSQLDatabaseQueries) CreateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error {
	if dbq.allowTestUuids {
		if IsEmpty(obj.RepositoryCredentialsID) {
			obj.RepositoryCredentialsID = "test-" + generateUuid()
		}
	} else {
		if !IsEmpty(obj.RepositoryCredentialsID) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.RepositoryCredentialsID = generateUuid()
	}

	if err := obj.hasEmptyValues("RepositoryCredentialsID"); err != nil {
		return err
	}

	obj.Created_on = time.Now()

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("%v: %w", errCreateRepositoryCredentials, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: %d", errRowsAffected, result.RowsAffected())
	}
	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteRepositoryCredentialsByID(ctx context.Context, id string) (int, error) {
	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	repoCred := &RepositoryCredentials{
		RepositoryCredentialsID: id,
	}

	result, err := dbq.dbConnection.Model(repoCred).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("%v: %w", errDeleteRepositoryCredentials, err)
	}

	return result.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) GetRepositoryCredentialsByID(ctx context.Context, id string) (obj RepositoryCredentials, err error) {
	if err = validateQueryParams(id, dbq); err != nil {
		return obj, err
	}

	obj = RepositoryCredentials{
		RepositoryCredentialsID: id,
	}

	err = dbq.dbConnection.Model(&obj).WherePK().Context(ctx).Select()
	if err != nil {
		return obj, fmt.Errorf("%v: %w", errGetRepositoryCredentials, err)
	}

	return obj, nil
}

func (dbq *PostgreSQLDatabaseQueries) UpdateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error {
	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}
	if err := obj.hasEmptyValues(); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("%v: %w", errUpdateRepositoryCredentials, err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: %d", errRowsAffected, result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllRepositoryCredentials(ctx context.Context, repositoryCredentials *[]RepositoryCredentials) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	err := dbq.dbConnection.Model(repositoryCredentials).Context(ctx).Select()

	if err != nil {
		return err
	}

	return nil
}

func (obj *RepositoryCredentials) Dispose(ctx context.Context, dbq DatabaseQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in RepositoryCredentials dispose")
	}

	_, err := dbq.DeleteRepositoryCredentialsByID(ctx, obj.RepositoryCredentialsID)
	return err
}

// Get RepositoryCredentials in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
// For example if you want RepositoryCredentials starting from 51-150 then set the limit to 100 and offset to 50.
func (dbq *PostgreSQLDatabaseQueries) GetRepositoryCredentialsBatch(ctx context.Context, repositoryCredentials *[]RepositoryCredentials, limit, offSet int) error {
	return dbq.dbConnection.
		Model(repositoryCredentials).
		Order("seq_id ASC").
		Limit(limit).   // Batch size
		Offset(offSet). // offset+1 is starting point of batch
		Context(ctx).
		Select()
}
