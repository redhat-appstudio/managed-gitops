package main

import (
	"log"
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
	db "github.com/redhat-appstudio/managed-gitops/backend/config/db"
	application "github.com/redhat-appstudio/managed-gitops/backend/routes/application"
	managedenviroment "github.com/redhat-appstudio/managed-gitops/backend/routes/managedenviroment"
	operations "github.com/redhat-appstudio/managed-gitops/backend/routes/operations"
)

func RouteInit() {
	wsContainer := restful.NewContainer()
	wsContainer.Router(restful.CurlyRouter{})

	// Registering operation resource to the wsContainer
	o := operations.OperationResource{Operations: map[string]operations.Operation{}}
	o.Register(wsContainer)

	// Registering application resource to the wsContainer
	a := application.ApplicationResource{Applications: map[string]application.Application{}}
	a.Register(wsContainer)

	// Registering managed enviroment resource to the wsContainer
	ws := new(restful.WebService)
	ws.
		Path("/api/v1/managedenvironment").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("").To(managedenviroment.HandleListManagedEnvironments).
		Returns(200, "OK", "List of Managed Clusters").
		Returns(404, "Error Occured, Not Found", nil))

	ws.Route(ws.POST("").To(managedenviroment.HandlePostManagedEnvironment).
		Returns(200, "OK", "URL for the WebUI").
		Returns(404, "Error Occured, Not Found", nil))

	ws.Route(ws.GET("/{managedenv-id}").To(managedenviroment.HandleGetASpecificManagementEnvironment))

	wsContainer.Add(ws)

	log.Print("The server is up, and listening to port 8090 on your host.")
	server := &http.Server{Addr: ":8090", Handler: wsContainer}
	log.Fatal(server.ListenAndServe())
}

func main() {
	// Connect DB
	db.ConnectToDatabase()
	RouteInit()
	// Intializing the server for routing endpoints

}
