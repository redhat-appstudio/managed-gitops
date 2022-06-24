package argocd

import (
	"fmt"

	. "github.com/onsi/gomega"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func HaveAppHealthStatusCode(status appv1.ApplicationStatus) matcher.GomegaMatcher {

	return WithTransform(func(app appv1.Application) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status.Health.Status == app.Status.Health.Status

		fmt.Println("HaveHealthStatusCode:", res, "/ Expected:", status, "/ Actual:", app.Status.Health.Status)

		return res
	}, BeTrue())
}
func HaveAppSyncStatusCode(status appv1.ApplicationStatus) matcher.GomegaMatcher {

	return WithTransform(func(app appv1.Application) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status.Sync.Status == app.Status.Sync.Status
		fmt.Println("HaveSyncStatusCode:", res, "/ Expected:", status, "/ Actual:", app.Status.Sync.Status)

		return res
	}, BeTrue())
}
