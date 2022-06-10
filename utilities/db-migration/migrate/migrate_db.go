package migrate

import (
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

func Migrate(opType string, migrationPath string) error {
	addr, password := db.GetAddrAndPassword()
	port := 5432

	m, err := migrate.New(
		migrationPath,
		fmt.Sprintf("postgresql://postgres:%s@%s:%v/postgres?sslmode=disable", password, addr, port))
	if err != nil {
		return fmt.Errorf("unable to connect to DB: %v", err)
	}

	if opType == "" {
		// applies every migrations till the lastest migration-sql present.
		// Automatically makes sure about the version the current database is on and updates it.
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("SEVERE: migration could not be applied; %v", err)
		}
		return nil

	} else if opType == "drop_smtable" {
		dbq, err := db.ConnectToDatabaseWithPort(true, "postgres", port)
		if err != nil {
			return fmt.Errorf("unable to connect to DB: %v", err)
		} else {
			_, err = dbq.Exec("DROP TABLE schema_migrations")
			if err != nil {
				return fmt.Errorf("unable to Drop table: %v", err)
			}
		}
		return nil

	} else if opType == "drop" {
		if err := m.Drop(); err != nil {
			return fmt.Errorf("unable to Drop DB: %v", err)
		}
		return nil

	} else {
		return fmt.Errorf("invalid argument passed")
	}

}
