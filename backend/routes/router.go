package routes

import (
	"log"
	"net/http"
	"time"

	restful "github.com/emicklei/go-restful/v3"

	webhooks "github.com/redhat-appstudio/managed-gitops/backend/routes/webhooks"
)

func RouteInit() *http.Server {
	wsContainer := restful.NewContainer()
	wsContainer.Router(restful.CurlyRouter{})

	webhookR := new(restful.WebService)
	webhookR.
		Path("/api/v1/webhookevent").
		Consumes(restful.MIME_JSON)
	webhookR.Route(webhookR.POST("").To(webhooks.ParseWebhookInfo))
	wsContainer.Add(webhookR)

	log.Print("Main: the server is up, and listening to port 8090 on your host.")
	server := &http.Server{Addr: ":8090", Handler: wsContainer, ReadHeaderTimeout: time.Second * 30}

	return server
}
