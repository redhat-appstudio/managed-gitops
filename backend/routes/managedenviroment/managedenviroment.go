package routes

import (
	"errors"
	"log"
	"net/http"
	"time"

	restful "github.com/emicklei/go-restful/v3"
)

/*
/api/v1/managedenvironment
POST: Create a new managed environment (which, behind the scenes, will add it to the Argo CD instance)
200 = Success, will return URL of the operation, to allow the web UI to track it
400 = An error occurred, the error will be returned in the response, and should be communicated to the user. For example, this would be if the user enters and invalid valid (which is detected by the backend, before it is handed to the Argo CD cluster instance)

GET: Retrieve the currents status of all managed clusters (or a specific subset via query params)
200 = Success, will return list of managed clusters
400 = An error occurred, the error will be returned in the response

/api/v1/managedenvironment/(id)
GET: Retrieve the current status of the given managed environment
DELETE: Stop managing an environment with Argo CD, and remove it from the product database.
*/

type ManagedEnvironmentListEntry struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	ConnectionStatus string `json:"connectionStatus"`
}

type ManagedEnvironmentListResponse struct {
	Entries []ManagedEnvironmentListEntry `json:"entries"`
}

type ManagedEnvironmentSingleEntry struct {
	Name              string `json:"name"`
	URL               string `json:"url"`
	KubeConfig        string `json:"config"`
	KubeConfigContext string `json:"configContext"`
	ConnectionStatus  string `json:"connectionStatus"`
}

type ManagedEnvironmentPostEntry struct {
	Name              string `json:"name"`
	URL               string `json:"url"`
	KubeConfig        string `json:"config"`
	KubeConfigContext string `json:"configContext"`
}

type ManagedEnvironmentResource struct {
	listResponse []ManagedEnvironmentListResponse
	singleEntry  map[string]ManagedEnvironmentSingleEntry
	postEntry    []ManagedEnvironmentPostEntry
}

// Creating a webservice for application endpoints
func (m ManagedEnvironmentResource) Register(container *restful.Container) {
	ws := new(restful.WebService)
	ws.
		Path("/api/v1/managedenvironment").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.GET("").To(m.handleListManagedEnvironments).
		Returns(200, "OK", m.listResponse).
		Returns(404, "Error Occured, Not Found", nil))

	ws.Route(ws.POST("").To(m.handlePostManagedEnvironment).
		Returns(200, "OK", "URL for the WebUI").
		Returns(404, "Error Occured, Not Found", nil))

	ws.Route(ws.GET("/{managedenv-id}").To(m.handleGetASpecificManagementEnvironment))
	//	ws.Route(ws.DELETE("/{managedenv-id}").To(m.deleteManagedEnviroment))
	container.Add(ws)
}

// Handle the GET to /api/v1/managedenvironment
// This is used by the 'List Managed Application Environments' UI and/or CLI

func (m ManagedEnvironmentResource) handleListManagedEnvironments(request *restful.Request, response *restful.Response) {
	ret := ManagedEnvironmentListResponse{
		Entries: []ManagedEnvironmentListEntry{
			// With another isssue, we would fill this in with data from the database,
			// but for now can just leave it blank, or use fake data.
		},
	}
	err := response.WriteEntity(ret)
	if err != nil {
		log.Fatal(err)
	}
	// (...)
	// return ret
}

// handle the POST to /api/v1/managedenvironment
func (m ManagedEnvironmentResource) handlePostManagedEnvironment(request *restful.Request, response *restful.Response) {
	// We would store the 'request' object from the user in the database,
	// then return the UUID of the operation.
	// But for now, just leave it blank.

	// return ""
}

// handle the GET to /api/v1/managedenvironment/{id}
func (m ManagedEnvironmentResource) handleGetASpecificManagementEnvironment(request *restful.Request, response *restful.Response) {
	//	id  := request.PathParameter("managedenv-id")
	ret := ManagedEnvironmentSingleEntry{
		// Normally we would read data from the database, and then insert it into this
		// struct, but for now just leave it blank or use fake data.
		// (...)
	}
	id := request.PathParameter("managedenv-id")
	env_check := m.singleEntry[id]
	if len(env_check.Name) == 0 {
		response.AddHeader("Content-Type", "text/plain")
		err := response.WriteErrorString(http.StatusNotFound, "Application not found!")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err := response.WriteEntity(ret)
		if err != nil {
			log.Fatal(err)
		}
	}
	// (...)
	// return ret
}

// Add function to start up the server, running against dedicated port
// Usage of CurlyRouter is done because of the efficiency while using wildcards and expressions
func RunRestfulCurlyRouterServer() {
	wsContainer := restful.NewContainer()
	wsContainer.Router(restful.CurlyRouter{})
	m := ManagedEnvironmentResource{}
	m.Register(wsContainer)

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
