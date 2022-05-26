package dbmigrate

import (
	"context"
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Migrate_db(op_type string) {
	addr, password := db.GetAddrAndPassword()
	port := 5432
	ctx := context.Background()
	log := log.FromContext(ctx)
	fmt.Println("logger works fine!")
	m, err := migrate.New(
		"file://migrations/",
		fmt.Sprintf("postgresql://postgres:%s@%s:%v/postgres?sslmode=disable", password, addr, port))
	if err != nil {
		log.Error(err, fmt.Sprintf("%v", err))
	}
	if len(op_type) >= 0 {
		if op_type == "drop_smtable" {
			dbq, err := db.ConnectToDatabaseWithPort(true, "postgres", port)
			if err != nil {
				log.Error(err, fmt.Sprintf("%v", err))
			} else {
				_, err = dbq.Exec("DROP TABLE schema_migrations")
				if err != nil {
					log.Error(err, fmt.Sprintf("%v", err))
				}
			}
		} else if op_type == "drop" {
			if err := m.Drop(); err != nil {
				log.Error(err, fmt.Sprintf("%v", err))
			}
		} else {
			log.Info("Invalid argument passed.")
		}
	} else {
		// applies every migrations till the lastest migration
		if err := m.Up(); err != nil {
			log.Error(err, fmt.Sprintf("%v", err))
		}
	}
}
