package main

import (
	"os"

	migrate "github.com/redhat-appstudio/managed-gitops/backend-shared/hack/db-migration/migrate"
)

func main() {
	op_type := ""
	if len(os.Args) >= 2 {
		op_type = os.Args[1]
	}
	migrate.Migrate_db(op_type)
}
