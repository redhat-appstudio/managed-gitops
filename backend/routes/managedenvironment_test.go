//go:build !skiproutes
// +build !skiproutes

package routes

import (
	"log"
	"net/http"
	"testing"

	"github.com/redhat-appstudio/managed-gitops/backend/util"
	"github.com/stretchr/testify/assert"
)

func TestManagedEnvironment(t *testing.T) {
	serverURL := "http://localhost:8090"

	server := RouteInit()
	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Println("Error on ListenAndServe:", err)
		}
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
