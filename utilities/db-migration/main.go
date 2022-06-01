package main

import (
	"os"

	migrate "github.com/redhat-appstudio/managed-gitops/utilities/db-migrate/migrate"
)

func main() {
	op_type := ""
	if len(os.Args) >= 2 {
		op_type = os.Args[1]
	}
	migrate.Main(op_type)
}
