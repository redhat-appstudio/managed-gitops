package routes

import (
	"log"

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

// These are the fields that the List Managed Environments query should return (eg GET /api/v1/managedenviroment)
type ManagedEnvironmentListEntry struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	URL              string `json:"url"`
	ConnectionStatus string `json:"connectionStatus"`
}

type ManagedEnvironmentListResponse struct {
	Entries []ManagedEnvironmentListEntry `json:"entries"`
}

// This is what should be returned when the user asks for information on a specific ManagedEnvironment
type ManagedEnvironmentGetSingleEntry struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	KubeConfig        string `json:"config"`
	KubeConfigContext string `json:"configContext"`
	ConnectionStatus  string `json:"connectionStatus"`
}

// This is what the user should give us as the body of a POST request
type ManagedEnvironmentPostEntry struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	KubeConfig        string `json:"config"`
	KubeConfigContext string `json:"configContext"`
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
