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

// UpdateStatusWithFunction updates a SnapshotEnvironmentBinding on a K8s cluster, using the provided function.
//
// UpdateStatusWithFunction will handle interfacing with K8s to retrieve the latest value of the
// SnapshotEnvironmentBinding; all the calling function needs to do is mutate it to the desired state.
func UpdateStatusWithFunction(binding *appstudiosharedv1.SnapshotEnvironmentBinding,
	mutationFn func(binding *appstudiosharedv1.SnapshotEnvironmentBindingStatus)) error {

	GinkgoWriter.Printf("Updating SnapshotEnvironmentBindingStatus for '%v'\n", binding.ObjectMeta)
	config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
	Expect(err).To(BeNil())

	k8sClient, err := fixture.GetKubeClient(config)
	if err != nil {
		fmt.Println(k8sFixture.K8sClientError, err)
		return err
	}

	return k8sFixture.UntilSuccess(k8sClient, func(k8sClient client.Client) error {

		// Retrieve the latest version of the SnapshotEnvironmentBinding resource
		err := k8sFixture.Get(binding, k8sClient)
		if err != nil {
			return err
		}

		// Call the mutation function, to set the status
		mutationFn(&binding.Status)

		// Attempt to update the object with the change made by the mutation function
		err = k8sFixture.UpdateStatus(binding, k8sClient)

		// Report back the error, if we hit one
		return err
	})

}

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

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
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
