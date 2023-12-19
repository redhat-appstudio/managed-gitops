package deploymenttargetclaim

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasStatusPhase checks if the DeploymentTarget is in the given phase.
func HasStatusPhase(phase appstudiosharedv1.DeploymentTargetPhase) matcher.GomegaMatcher {
	return WithTransform(func(dt appstudiosharedv1.DeploymentTarget) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println("HasStatusPhase:", k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&dt), &dt)
		if err != nil {
			fmt.Println("HasStatusPhase:", k8sFixture.K8sClientError, err)
			return false
		}

		if dt.Status.Phase != phase {
			fmt.Printf("Phase mismatch for DT %s: Expected: %s, Actual: %s\n", dt.Name, phase, dt.Status.Phase)
			return false
		}

		return true
	}, BeTrue())
}

func HaveDeploymentTargetCondition(expected metav1.Condition) matcher.GomegaMatcher {
	return WithTransform(func(dt appstudiosharedv1.DeploymentTarget) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&dt), &dt)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		if len(dt.Status.Conditions) == 0 {
			fmt.Println("HaveDeploymentTargetCondition: DeploymentTargetClaimCondition is nil")
			return false
		}
		actual := dt.Status.Conditions[0]
		fmt.Println("HaveDeploymentTargetCondition:", "expected: ", expected, "actual: ", actual)
		return actual.Type == expected.Type &&
			actual.Status == expected.Status &&
			actual.Reason == expected.Reason &&
			actual.Message == expected.Message

	}, BeTrue())
}
