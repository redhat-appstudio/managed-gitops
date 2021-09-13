package routes

/*
/api/v1/application
GET: Retrieve a list of applications with the most recently updated statuses (or another subset, via query params)

/api/v1/application/(id)
GET: Retrieve details on a particular application

ToDos (in future): Write Operations (POST, PUT, DELETE)
*/

import (
	"errors"
	"log"
	"net/http"
	"time"

	restful "github.com/emicklei/go-restful/v3"
)

// Creating a REST layer for application - ApplicationResource

type Application struct {
	Id, Name string
}

type ApplicationResource struct {
	applications map[string]Application
}

// Creating a webservice for application endpoints
func (a ApplicationResource) Register(container *restful.Container) {
	ws := new(restful.WebService)
	ws.
		Path("/api/v1/application").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("/{application-id}").To(a.findApplication))
	ws.Route(ws.GET("").To(a.recentApplication))
	container.Add(ws)
}

// GET Retrieve a list of applications with the most recently updated statuses
func (a ApplicationResource) recentApplication(request *restful.Request, response *restful.Response) {
	list := []Application{}
	for _, each := range a.applications {
		list = append(list, each)
	}

	err := response.WriteEntity(list)
	if err != nil {
		log.Fatal(err)
	}
}

// GET info of operations depening upon the id
func (a ApplicationResource) findApplication(request *restful.Request, response *restful.Response) {
	id := request.PathParameter("application-id")
	app := a.applications[id]
	if len(app.Id) == 0 {
		response.AddHeader("Content-Type", "text/plain")
		err := response.WriteErrorString(http.StatusNotFound, "Application not found!")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err := response.WriteEntity(app)
		if err != nil {
			log.Fatal(err)
		}
	}
}

// Add function to start up the server, running against dedicated port
// Usage of CurlyRouter is done because of the efficiency while using wildcards and expressions
func RunRestfulCurlyRouterServer() {
	wsContainer := restful.NewContainer()
	wsContainer.Router(restful.CurlyRouter{})
	a := ApplicationResource{map[string]Application{}}
	a.Register(wsContainer)

	log.Print("The server is up, and listening to port 8090 on your host.")
	server := &http.Server{Addr: ":8090", Handler: wsContainer}
	log.Fatal(server.ListenAndServe())
}

func waitForServerUp(serverURL string) error {
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(5 * time.Second) {
		_, err := http.Get(serverURL + "/")
		if err == nil {
			return nil
		}
	}
	return errors.New("Server Timed Out!")
}
