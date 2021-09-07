package config

import (
	"fmt"
	"log"
	"os"

	"github.com/go-pg/pg/v10"
)

// function to connect to Postgres
func ConnectToDatabase() *pg.DB {
	fmt.Println("Connection Initiated")
	opts := &pg.Options{
		Addr:     "localhost:5432",
		User:     "postgres",
		Password: "gitops",
		Database: "managed-gitops-postgres",
	}

	var db *pg.DB = pg.Connect(opts)

	if db == nil {
		log.Printf("Failed to connect")
		os.Exit(100)
	}
	log.Printf("Connected to db")
	return db
}
