package gitopsdeployment

import (
	"fmt"

	. "github.com/onsi/gomega"

	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
)

func HaveHealthStatusCode(status managedgitopsv1alpha1.HealthStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			return false
		}

		res := status == gitopsDepl.Status.Health.Status

		fmt.Println("HaveHealthStatusCode:", res, "/ Expected:", status, "/ Actual:", gitopsDepl.Status.Health.Status)

		return res
	}, BeTrue())
}

func HaveSyncStatusCode(status managedgitopsv1alpha1.SyncStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			return false
		}

		res := status == gitopsDepl.Status.Sync.Status
		fmt.Println("HaveSyncStatusCode:", res, "/ Expected:", status, "/ Actual:", gitopsDepl.Status.Sync.Status)

		return res
	}, BeTrue())
}
