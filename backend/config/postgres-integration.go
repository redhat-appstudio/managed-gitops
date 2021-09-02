package config

import (
	"fmt"
	"log"
	"os"

	"github.com/go-pg/pg/v10"
)

// function to connect to Postgres
func Connect2db() *pg.DB {
	fmt.Println("Connection Initiated")
	opts := &pg.Options{
		Network:               "",
		Addr:                  "localhost:5432",
		User:                  "postgres",
		Password:              "gitops",
		Database:              "managed-gitops-postgres",
		ApplicationName:       "",
		DialTimeout:           0,
		ReadTimeout:           0,
		WriteTimeout:          0,
		MaxRetries:            0,
		RetryStatementTimeout: false,
		MinRetryBackoff:       0,
		MaxRetryBackoff:       0,
		PoolSize:              0,
		MinIdleConns:          0,
		MaxConnAge:            0,
		PoolTimeout:           0,
		IdleTimeout:           0,
		IdleCheckFrequency:    0,
	}

	var db *pg.DB = pg.Connect(opts)

	if db == nil {
		log.Printf("Failed to connect")
		os.Exit(100)
	}
	log.Printf("Connected to db")
	return db
}
