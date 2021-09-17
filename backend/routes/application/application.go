package routes

/*
/api/v1/application
GET: Retrieve a list of applications with the most recently updated statuses (or another subset, via query params)

/api/v1/application/(id)
GET: Retrieve details on a particular application

TODO: Write Operations (POST, PUT, DELETE)
*/

import (
	"log"
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
)

// Creating a REST layer for application - ApplicationResource

type ApplicationListEntry struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Health      string `json:"health"`
	SyncStatus  string `json:"syncStatus"`
}

type ApplicationEntry struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	SyncPolicyAutomatic bool   `json:"syncPolicyAutomatic"`
	GitRepositoryURL    string `json:"gitRepositoryURL"`
	GitRevision         string `json:"gitRevision"`
	GitPath             string `json:"gitPath"`
	DestinationCluster  string `json:"destinationCluster"`
	Namespace           string `json:"namespace"`
}

type Application struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ApplicationResource struct {
	Applications map[string]Application `json:"applications"`
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
	list := []ApplicationListEntry{}

	for _, each := range a.Applications {
		list = append(list, ApplicationListEntry{
			Id:   each.Id,
			Name: each.Name,
		})
	}

	err := response.WriteEntity(list)
	if err != nil {
		log.Fatal(err)
	}
}

// GET info of applications depening upon the id
func (a ApplicationResource) findApplication(request *restful.Request, response *restful.Response) {
	id := request.PathParameter("application-id")
	app := a.Applications[id]
	if len(app.Id) == 0 {
		response.AddHeader("Content-Type", "text/plain")
		err := response.WriteErrorString(http.StatusNotFound, "Application not found!")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err := response.WriteEntity(ApplicationEntry{
			Name: app.Name,
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}
