package binding

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"sigs.k8s.io/controller-runtime/pkg/client"

	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

func HaveStatusGitOpsDeployments(gitOpsDeployments []appstudiosharedv1.BindingStatusGitOpsDeployment) matcher.GomegaMatcher {

	// compare compares two slices, returning true if the contents are equal regardless of the order of elements in the slices
	compare := func(a []appstudiosharedv1.BindingStatusGitOpsDeployment, b []appstudiosharedv1.BindingStatusGitOpsDeployment) string {
		if len(a) != len(b) {
			return "lengths don't match"
		}

		for _, aVal := range a {

			match := false
			for _, bVal := range b {

				if reflect.DeepEqual(aVal, bVal) {
					match = true
					break
				}
			}

			if !match {
				return fmt.Sprintf("no match for %v", aVal)
			}
		}

		return ""
	}

	return WithTransform(func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&binding), &binding)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		compareContents := compare(gitOpsDeployments, binding.Status.GitOpsDeployments)

		GinkgoWriter.Println("HaveStatusGitOpsDeployments:", compareContents, "/ Expected:", gitOpsDeployments, "/ Actual:", binding.Status.GitOpsDeployments)

		return compareContents == ""
	}, BeTrue())
}
