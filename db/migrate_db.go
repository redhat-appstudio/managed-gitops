package main

import (
	"fmt"
	"log"
	"os"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

func main() {
	addr, password := db.GetAddrAndPassword()
	port := 5432
	m, err := migrate.New(
		"file://migrations/",
		fmt.Sprintf("postgresql://postgres:%s@%s:%v/postgres?sslmode=disable", password, addr, port))
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) >= 2 {
		op_type := os.Args[1]
		if op_type == "drop_smtable" {
			dbq, err := db.ConnectToDatabaseWithPort(true, "postgres", port)
			if err != nil {
				log.Fatal(fmt.Sprintf("%v", err))
			} else {
				_, err = dbq.Exec("DROP TABLE schema_migrations")
				if err != nil {
					log.Fatal(fmt.Sprintf("%v", err))
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
