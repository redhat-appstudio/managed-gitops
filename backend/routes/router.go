package routes

import (
	"log"
	"net/http"

	restful "github.com/emicklei/go-restful/v3"

	webhooks "github.com/redhat-appstudio/managed-gitops/backend/routes/webhooks"
)

func RouteInit() *http.Server {
	wsContainer := restful.NewContainer()
	wsContainer.Router(restful.CurlyRouter{})

	// // Registering operation resource to the wsContainer
	// o := operations.OperationResource{Operations: map[string]operations.Operation{}}
	// o.Register(wsContainer)

	// // Registering application resource to the wsContainer
	// a := application.ApplicationResource{Applications: map[string]application.Application{}}
	// a.Register(wsContainer)

	// // Registering managed environment resource to the wsContainer
	// ws := new(restful.WebService)
	// ws.
	// 	Path("/api/v1/managedenvironment").
	// 	Consumes(restful.MIME_JSON).
	// 	Produces(restful.MIME_JSON)

	// ws.Route(ws.GET("").To(managedenvironment.HandleListManagedEnvironments).
	// 	Returns(200, "OK", "List of Managed Clusters").
	// 	Returns(404, "Error Occured, Not Found", nil))

	// ws.Route(ws.POST("").To(managedenvironment.HandlePostManagedEnvironment).
	// 	Returns(200, "OK", "URL for the WebUI").
	// 	Returns(404, "Error Occured, Not Found", nil))

	// ws.Route(ws.GET("/{managedenv-id}").To(managedenvironment.HandleGetASpecificManagementEnvironment))
	// wsContainer.Add(ws)

	webhookR := new(restful.WebService)
	webhookR.
		Path("/api/v1/webhookevent").
		Consumes(restful.MIME_JSON)
	webhookR.Route(webhookR.POST("").To(webhooks.ParseWebhookInfo))
	wsContainer.Add(webhookR)

	log.Print("Main: the server is up, and listening to port 8090 on your host.")
	server := &http.Server{Addr: ":8090", Handler: wsContainer}

	return server
}
