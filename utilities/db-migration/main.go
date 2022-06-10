package main

import (
	"fmt"
	"os"

	migrate "github.com/redhat-appstudio/managed-gitops/utilities/db-migration/migrate"
)

func main() {
	opType := ""
	if len(os.Args) >= 2 {
		opType = os.Args[1]
	}
	if err := migrate.Migrate(opType, "file://migrations/"); err != nil {
		fmt.Println("Unable to migrate database:", err)
		os.Exit(1)
		return
	}
}
