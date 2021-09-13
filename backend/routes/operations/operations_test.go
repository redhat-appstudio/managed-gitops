package routes

import (
	"bytes"
	"net/http"
	"testing"

	restful "github.com/emicklei/go-restful/v3"
)

func TestServer(t *testing.T) {
	serverURL := "http://localhost:8090"
	go func() {
		RunRestfulCurlyRouterServer()
	}()
	if err := waitForServerUp(serverURL); err != nil {
		t.Errorf("%v", err)
	}

	// GET should give a 405
	resp, err := http.Get(serverURL + "/api/v1/operation/")
	if err != nil {
		t.Errorf("unexpected error in GET /api/v1/operation/: %v", err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}

	// Send a POST request.
	var jsonStr = []byte(`{"id":"1","name":"operation1"}`)
	req, err := http.NewRequest("POST", serverURL+"/api/v1/operation/", bytes.NewBuffer(jsonStr))
	if err != nil {
		t.Errorf("An error occurred!!!!!!! %v", err)
	}
	req.Header.Set("Content-Type", restful.MIME_JSON)

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		t.Errorf("unexpected error in sending req: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}

	// Test that GET works.
	resp, err = http.Get(serverURL + "/api/v1/operation/1")
	if err != nil {
		t.Errorf("unexpected error in GET /api/v1/operation/1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}
}
