package utils

import (
	"io/ioutil"
	"log"
	"net/http"
)

// GET Request to retrive API Server metrics from the route endpoint
func GetAPIServerMetrics(routeEndpoint string) string {
	resp, err := http.Get(routeEndpoint)
	if err != nil {
		log.Fatalln(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	sb := string(body)
	log.Printf(sb)
	return sb
}

// GET Request to retrive Repo Server metrics from the route endpoint
func GetRepoServerMetrics(routeEndpoint string) string {
	resp, err := http.Get(routeEndpoint)
	if err != nil {
		log.Fatalln(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	sb := string(body)
	log.Printf(sb)
	return sb
}

// GET Request to retrive Application Controller metrics from the route endpoint
func GetApplicationControllerMetrics(routeEndpoint string) string {
	resp, err := http.Get(routeEndpoint)
	if err != nil {
		log.Fatalln(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	sb := string(body)
	log.Printf(sb)
	return resp.Status
}
