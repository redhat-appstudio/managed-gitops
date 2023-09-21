package db

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"

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

func GetAddrAndPassword() (string, string, string) {
	addr := "localhost"
	if isEnvExist("DB_ADDR") {
		addr = os.Getenv("DB_ADDR")
	}

	password := "gitops"
	if isEnvExist("DB_PASS") {
		password = os.Getenv("DB_PASS")
	}

	databaseName := "postgres"
	if isEnvExist("POSTGRESQL_DATABASE") {
		databaseName = os.Getenv("POSTGRESQL_DATABASE")
	}
	return addr, password, databaseName
}

// connectToDatabaseWithPort connects to Postgres with a defined port
func ConnectToDatabaseWithPort(verbose bool, port int) (*pg.DB, error) {
	addr, password, dbName := GetAddrAndPassword()
	opts := &pg.Options{
		Addr:     fmt.Sprintf("%s:%v", addr, port),
		User:     "postgres",
		Password: password,
		Database: dbName,
	}

	if value, isSet := os.LookupEnv("DEV_ONLY_ALLOW_NON_TLS_CONNECTION_TO_POSTGRESQL"); !isSet || strings.ToLower(value) != "true" {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	db := pg.Connect(opts)

	if err := checkConn(db); err != nil {
		return nil, fmt.Errorf("%v, unable to connect to database: Host:'%s' User:'%s' DB:'%s' ", err, opts.Addr, opts.User, opts.Database)
	}

	if verbose {
		db.AddQueryHook(pgdebug.DebugHook{
			// Print all queries.
			Verbose: true,
		})
	}

	return db, nil
}
