package routes

import (
	"log"
	"net/http"

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

// Creating a webservice for application endpoints
func RouteInit() {
	ws := new(restful.WebService)
	ws.
		Path("/api/v1/managedenvironment").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.GET("").To(HandleListManagedEnvironments).
		Returns(200, "OK", "List of Managed Clusters").
		Returns(404, "Error Occured, Not Found", nil))

	ws.Route(ws.POST("").To(HandlePostManagedEnvironment).
		Returns(200, "OK", "URL for the WebUI").
		Returns(404, "Error Occured, Not Found", nil))

	ws.Route(ws.GET("/{managedenv-id}").To(HandleGetASpecificManagementEnvironment))
	//	ws.Route(ws.DELETE("/{managedenv-id}").To(m.deleteManagedEnviroment))
	restful.Add(ws)

	log.Print("The server is up, and listening to port 8090 on your host.")
	server := &http.Server{Addr: ":8090"}
	log.Fatal(server.ListenAndServe())

}

// Handle the GET to /api/v1/managedenvironment
// This is used by the 'List Managed Application Environments' UI and/or CLI

func HandleListManagedEnvironments(request *restful.Request, response *restful.Response) {

	ret := ManagedEnvironmentListResponse{
		Entries: []ManagedEnvironmentListEntry{
			// With another isssue, we would fill this in with data from the database,
			// but for now can just leave it blank, or use fake data.
			// {
			// 	Name:             "my cluster",
			// 	URL:              "https://kubernetes.default.svc",
			// 	ConnectionStatus: "connected",
			// },
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
func HandlePostManagedEnvironment(request *restful.Request, response *restful.Response) {
	// We would store the 'request' object from the user in the database,
	// then return the UUID of the operation.
	// But for now, just leave it blank.

	// return ""
}

// handle the GET to /api/v1/managedenvironment/{id}
func HandleGetASpecificManagementEnvironment(request *restful.Request, response *restful.Response) {
	//	id  := request.PathParameter("managedenv-id")
	// ret := ManagedEnvironmentSingleEntry{
	// 	// Normally we would read data from the database, and then insert it into this
	// 	// struct, but for now just leave it blank or use fake data.
	// 	Name:              "my cluster",
	// 	URL:               "https://kubernetes.default.svc",
	// 	KubeConfig:        "...",
	// 	KubeConfigContext: "k3d-my-cluster",
	// 	ConnectionStatus:  "connected", // or disconnected
	// }

	// test := ManagedEnvironmentSingleEntry{}

	// function needed to implement for id comparison

	// id := request.PathParameter("managedenv-id")
	// env_check := ret.Name
	// if len(env_check.Name) == 0 {
	// 	response.AddHeader("Content-Type", "text/plain")
	// 	err := response.WriteErrorString(http.StatusNotFound, "Application not found!")
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// } else {
	// 	err := response.WriteEntity(ret)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// }
	// (...)
	// return ret
}
