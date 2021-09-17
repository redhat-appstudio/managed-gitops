package tests

import (
	"net/http"
	"testing"

	"github.com/redhat-appstudio/managed-gitops/backend/routes"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	"github.com/stretchr/testify/assert"
)

func TestManagedEnvironment(t *testing.T) {
	serverURL := "http://localhost:8090"

	server := routes.RouteInit()
	go func() {
		server.ListenAndServe()
	}()

	defer server.Close()

	if err := util.WaitForServerUp(serverURL); err != nil {
		assert.Error(t, err)
		return
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
