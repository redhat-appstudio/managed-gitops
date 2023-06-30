package utils

import (
	"io"
	"net/http"
)

// GET Request to retrieve API Server metrics from the route endpoint
func GetAPIServerMetrics(routeEndpoint string) (string, error) {
	// #nosec G107
	resp, err := http.Get(routeEndpoint)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	sb := string(body)
	return sb, err
}

// GET Request to retrieve Repo Server metrics from the route endpoint
func GetRepoServerMetrics(routeEndpoint string) (string, error) {
	// #nosec G107
	resp, err := http.Get(routeEndpoint)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	sb := string(body)
	return sb, err
}

// GET Request to retrieve Application Controller metrics from the route endpoint
func GetApplicationControllerMetrics(routeEndpoint string) (string, error) {
	// #nosec G107
	resp, err := http.Get(routeEndpoint)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	sb := string(body)
	return sb, err
}
