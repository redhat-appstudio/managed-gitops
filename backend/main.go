package main

import (
	// "log"
	db "github.com/jgwest/managed-gitops/backend/config/db"
)

func main() {
	// Connect DB
	db.ConnectToDatabase()

	// Init Router
	//	router := gin.Default()

	// Route Handlers / Endpoints
	//	routes.Routes(router)

	//	log.Fatal(router.Run(":4747"))
}
