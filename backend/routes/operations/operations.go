package routes

import (
	"errors"
	"log"
	"net/http"
	"time"

	restful "github.com/emicklei/go-restful/v3"
)

/*
Operation

/api/v1/operation
POST: Create a new operation

/api/v1/operation/(id)
GET: Retrieve the given operation
*/

// Creating a REST layer as OperationResource to have all the operation

type Operation struct {
	Id, Name string
}

type OperationResource struct {
	operations map[string]Operation
}

// Creating a webservice for operation endpoints
func (o OperationResource) Register(container *restful.Container) {
	ws := new(restful.WebService)
	ws.
		Path("/api/v1/operation").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.GET("/{operation-id}").To(o.findOperation))
	ws.Route(ws.POST("").To(o.addOperation))
	container.Add(ws)
}

// GET info of all the operations
func (o OperationResource) findAllOperations(request *restful.Request, response *restful.Response) {
	list := []Operation{}
	for _, each := range o.operations {
		list = append(list, each)
	}
	response.WriteEntity(list)
}

// GET info of operations depening upon the id
func (o OperationResource) findOperation(request *restful.Request, response *restful.Response) {
	id := request.PathParameter("operation-id")
	opr := o.operations[id]
	if len(opr.Id) == 0 {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusNotFound, "Operation not found!")
	} else {
		response.WriteEntity(opr)
	}
}

// POST to create an operation
func (o *OperationResource) addOperation(request *restful.Request, response *restful.Response) {
	opr := new(Operation)
	err := request.ReadEntity(&opr)
	if err == nil {
		o.operations[opr.Id] = *opr
		response.WriteEntity(opr)
	} else {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusInternalServerError, err.Error())
	}
}

// Add function to start up the server, running against dedicated port
// Usage of CurlyRouter is done because of the efficiency while using wildcards and expressions
func RunRestfulCurlyRouterServer() {
	wsContainer := restful.NewContainer()
	wsContainer.Router(restful.CurlyRouter{})
	o := OperationResource{map[string]Operation{}}
	o.Register(wsContainer)

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
