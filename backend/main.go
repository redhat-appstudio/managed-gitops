package main

import (
	"log"
	"net/http"

	"github.com/redhat-appstudio/managed-gitops/backend/routes"
)

func main() {

	// Intializing the server for routing endpoints
	router := routes.RouteInit()
	err := router.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Println("Error on ListenAndServe:", err)
	}

}
