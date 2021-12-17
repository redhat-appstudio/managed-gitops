package db

import (
	"fmt"

	"github.com/go-pg/pg/extra/pgdebug"
	"github.com/go-pg/pg/v10"
)

// function to connect to Postgres
func connectToDatabase(verbose bool) (*pg.DB, error) {
	opts := &pg.Options{
		Addr:     "localhost:5432",
		User:     "postgres",
		Password: "gitops",
		Database: "postgres",
	}

	var db *pg.DB = pg.Connect(opts)

	if db == nil {
		return nil, fmt.Errorf("unable to connect to database: %s %s %s ", opts.Addr, opts.User, opts.Database)
	}

	if verbose {
		db.AddQueryHook(pgdebug.DebugHook{
			// Print all queries.
			Verbose: true,
		})
	}

	return db, nil
}
