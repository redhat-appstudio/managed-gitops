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
	resp, err := http.Get(serverURL + "/operations/")
	if err != nil {
		t.Errorf("unexpected error in GET /operations/: %v", err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}

	// Send a POST request.
	var jsonStr = []byte(`{"id":"1","name":"operation1"}`)
	req, err := http.NewRequest("POST", serverURL+"/operations/", bytes.NewBuffer(jsonStr))
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
	resp, err = http.Get(serverURL + "/operations/1")
	if err != nil {
		t.Errorf("unexpected error in GET /operations/1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}
}
