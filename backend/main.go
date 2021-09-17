package main

import "github.com/redhat-appstudio/managed-gitops/backend/routes"

func main() {

	// Intializing the server for routing endpoints
	router := routes.RouteInit()
	router.ListenAndServe()

}
