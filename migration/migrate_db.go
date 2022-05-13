package main

import (
	"fmt"
	"log"
	"os"

	"github.com/go-pg/pg/v10"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

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

func getAddrandPassword() (string, string, int) {
	port := 5432
	addr := "localhost"
	if isEnvExist("DB_ADDR") {
		addr = os.Getenv("DB_ADDR")
	}

	password := "gitops"
	if isEnvExist("DB_PASS") {
		password = os.Getenv("DB_PASS")
	}
	return addr, password, port
}

func connectToDatabaseWithPort() (*pg.DB, error) {
	addr, password, port := getAddrandPassword()
	opts := &pg.Options{
		Addr:     fmt.Sprintf("%s:%v", addr, port),
		User:     "postgres",
		Password: password,
		Database: "postgres",
	}

	db := pg.Connect(opts)

	if err := checkConn(db); err != nil {
		return nil, fmt.Errorf("%v, unable to connect to database: Host:'%s' User:'%s' Pass:'%s' DB:'%s' ", err, opts.Addr, opts.User, opts.Password, opts.Database)
	}

	return db, nil
}

func main() {
	addr, password, port := getAddrandPassword()
	m, err := migrate.New(
		"file://migrations/",
		fmt.Sprintf("postgresql://postgres:%s@%s:%v/postgres?sslmode=disable", password, addr, port))
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) >= 2 {
		op_type := os.Args[1]
		if op_type == "drop_smtable" {
			dbq, err := connectToDatabaseWithPort()
			if err != nil {
				log.Fatal("%v", err)
			} else {
				_, err = dbq.Exec("DROP TABLE schema_migrations")
				if err != nil {
					log.Fatal("%v", err)
				}
			}
		} else if op_type == "drop" {
			if err := m.Drop(); err != nil {
				log.Fatal(err)
			}
		} else {
			log.Fatal("Invalid argument passed.")
		}
	} else {
		// applies every migrations till the lastest migration
		if err := m.Up(); err != nil {
			log.Fatal(err)
		}
	}
}
