package routes

import (
	// "fmt"
	// "log"
	// "net/http"

	"io"
	"net/http"

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

// GET info of all the operations
func (o OperationResource) findAllOperations(request *restful.Request, response *restful.Response) {
	list := []Operation{}
	for _, each := range o.operations {
		list = append(list, each)
	}
	response.WriteEntity(list)
}

func main_dummy() {
	ws := new(restful.WebService)
	ws.Route(ws.GET("/api/v1/operation").To(operation))
	restful.Add(ws)
	http.ListenAndServe(":8080", nil)

}

func operation(req *restful.Request, resp *restful.Response) {
	io.WriteString(resp, "Operation Add")
}

/*
ws := new()
*/

// GET for one particular operation
