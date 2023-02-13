package migrate

import (
	"fmt"
	"strings"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

func Migrate(opType string, migrationPath string) error {
	addr, password := db.GetAddrAndPassword()
	port := 5432

	// Base64 strings can contain '/' characters, which mess up URL parsing.
	// So we substitute it with a URL-friendly character.
	password = strings.ReplaceAll(password, "/", "%2f")

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

	} else if opType == "downgrade_migration" {
		if err := m.Steps(-1); err != nil {
			return fmt.Errorf("unable to downgrade migration version by 1 level: %v", err)
		}
		return nil
	} else if opType == "upgrade_migration" {
		if err := m.Steps(1); err != nil {
			return fmt.Errorf("unable to upgrade migration version by 1 level: %v", err)
		}
		return nil
	} else if opType == "migrate_to" {
		if err := m.Migrate(10); err != nil {
			if err.Error() == "no change" {
				return nil
			}
			return fmt.Errorf("unable to Migrate to version 10: %v", err)
		}
		return nil
	} else {
		return fmt.Errorf("invalid argument passed")
	}

}
