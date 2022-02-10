package db

import (
	"fmt"
	"os"

	"github.com/go-pg/pg/extra/pgdebug"
	"github.com/go-pg/pg/v10"
)

const DEFAULT_PORT = 5432

func isEnvExist(key string) bool {
	if _, ok := os.LookupEnv(key); ok {
		return true
	}

	return false
}

func checkConn(db *pg.DB) error {
	var n int
	_, err := db.QueryOne(pg.Scan(&n), "SELECT 1")
	return err
}

// connectToDatabaseWithPort connects to Postgres with a defined port
func connectToDatabaseWithPort(verbose bool, dbName string, port int) (*pg.DB, error) {
	addr := "localhost"
	if isEnvExist("DB_ADDR") {
		addr = os.Getenv("DB_ADDR")
	}

	password := "gitops"
	if isEnvExist("DB_PASS") {
		password = os.Getenv("DB_PASS")
	}
	opts := &pg.Options{
		Addr:     fmt.Sprintf("%s:%v", addr, port),
		User:     "postgres",
		Password: password,
		Database: dbName,
	}

	db := pg.Connect(opts)

	if err := checkConn(db); err != nil {
		return nil, fmt.Errorf("%v, unable to connect to database: Host:'%s' User:'%s' Pass:'%s' DB:'%s' ", err, opts.Addr, opts.User, opts.Password, opts.Database)
	}

	if verbose {
		db.AddQueryHook(pgdebug.DebugHook{
			// Print all queries.
			Verbose: true,
		})
	}

	return db, nil
}
