package loadtest

import (
	"fmt"
	"testing"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

func TestAThing(t *testing.T) {

	app := appv1.Application{}

	t.Log("hi!")
	fmt.Println(app)
}
