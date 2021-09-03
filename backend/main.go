package main

import (
	// "log"
	config "github.com/jgwest/managed-gitops/backend/config"
)

func main() {
	// Connect DB
	config.Connect()

	// Init Router
	//	router := gin.Default()

	// Route Handlers / Endpoints
	//	routes.Routes(router)

	//	log.Fatal(router.Run(":4747"))
}
