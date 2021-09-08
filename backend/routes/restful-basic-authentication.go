package routes

import (
	"io"
	"log"
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
)

// This example shows how to create a (Route) Filter that performs Basic Authentication on the Http request.
//
// GET http://localhost:8080/secret
// and use admin,admin for the credentials

func StartRoutes() {
	ws := new(restful.WebService)
	// ws.Consumes(restful.MIME_JSON, restful.MIME_XML)
	// ws.Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.GET("/"). /*.Filter(basicAuthenticate)*/ To(secret))
	// ws.Param(ws.PathParameter("medium", "digital or paperback").DataType("string")).
	// ws.Param(ws.QueryParameter("language", "en,nl,de").DataType("string")).
	// ws.Param(ws.HeaderParameter("If-Modified-Since", "last known timestamp").DataType("datetime")).
	restful.Add(ws)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// func basicAuthenticate(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
// 	// usr/pwd = admin/admin
// 	u, p, ok := req.Request.BasicAuth()
// 	if !ok || u != "admin" || p != "admin" {
// 		resp.AddHeader("WWW-Authenticate", "Basic realm=Protected Area")
// 		resp.WriteErrorString(401, "401: Not Authorized")
// 		return
// 	}
// 	chain.ProcessFilter(req, resp)
// }

//nolint
func secret(req *restful.Request, resp *restful.Response) {

	_, err := io.WriteString(resp, "42")
	if err != nil {
		log.Fatal(err)
	}
}
