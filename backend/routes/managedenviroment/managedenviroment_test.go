package routes

import (
	"net/http"
	"testing"

	util "github.com/redhat-appstudio/managed-gitops/backend/util"
)

func TestServer(t *testing.T) {
	serverURL := "http://localhost:8090"
	go func() {
		RouteInit()
	}()
	if err := util.WaitForServerUp(serverURL); err != nil {
		t.Errorf("%v", err)
	}

	// GET should successfully pass with 200
	resp, err := http.Get(serverURL + "/api/v1/managedenvironment/")
	if err != nil {
		t.Errorf("unexpected error in GET /api/v1/managedenvironment/: %v", err)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Errorf("unexpected response: %v, expected: %v", resp.StatusCode, http.StatusOK)
	}
}
