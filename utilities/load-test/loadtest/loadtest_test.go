package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	// appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAThing(t *testing.T) {

	// GetE2EEFixtureK8sClient is from the ApplicationSet repo.
	// You can see more examples of how to use it, here: https://github.com/argoproj-labs/applicationset/search?q=GetE2EFixtureK8sClient

	// In this example, I am retrieving all Application CRs within the 'argocd' namespace

	appList, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications("argocd").List(context.TODO(), v1.ListOptions{})

	if err != nil {
		t.Errorf("error, %v", err)
		return
	}

	for _, app := range appList.Items {

		appJson, err := json.MarshalIndent(app, "", "  ")
		if err != nil {
			t.Errorf("error, %v", err)
			return
		}

		fmt.Println(string(appJson))

	}

}
