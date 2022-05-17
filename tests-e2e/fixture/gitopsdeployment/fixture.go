package gitopsdeployment

import (
	"fmt"

	. "github.com/onsi/gomega"

	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
)

// HaveHealthStatusCode waits for the given GitOpsDeployment to have the expected Health status (e.g. "Healthy"/"Unhealthy").
//
// This indicates whether the GitOpsDeployment (based on the Argo CD Application) is 'healthy',
// that is, all resources are working as expected by Argo CD's definition.
func HaveHealthStatusCode(status managedgitopsv1alpha1.HealthStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status == gitopsDepl.Status.Health.Status

		fmt.Println("HaveHealthStatusCode:", res, "/ Expected:", status, "/ Actual:", gitopsDepl.Status.Health.Status)

		return res
	}, BeTrue())
}

// HaveSyncStatusCode waits for the given GitOpsDeployment to have the expected Sync status (e.g. "Unknown"/"Synced"/"OutOfSync")
//
// This value indicates whether the K8s resources defined in the GitOps repository are equal to (in sync with) the resources
// on the target cluster, according to Argo CD.
func HaveSyncStatusCode(status managedgitopsv1alpha1.SyncStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status == gitopsDepl.Status.Sync.Status
		fmt.Println("HaveSyncStatusCode:", res, "/ Expected:", status, "/ Actual:", gitopsDepl.Status.Sync.Status)

		return res
	}, BeTrue())
}
