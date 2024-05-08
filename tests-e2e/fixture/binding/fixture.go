package binding

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiosharedv1 "github.com/github.com/konflux-ci/application-api/api/v1alpha1"
	matcher "github.com/onsi/gomega/types"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

// This is intentionally NOT exported, for now. Create another function in this file/package that calls this function, and export that.
func expectedCondition(f func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool) matcher.GomegaMatcher {

	return WithTransform(func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool {

		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		return f(binding)

	}, BeTrue())

}

// UpdateStatusWithFunction updates a SnapshotEnvironmentBinding on a K8s cluster, using the provided function.
//
// UpdateStatusWithFunction will handle interfacing with K8s to retrieve the latest value of the
// SnapshotEnvironmentBinding; all the calling function needs to do is mutate it to the desired state.
func UpdateStatusWithFunction(binding *appstudiosharedv1.SnapshotEnvironmentBinding,
	mutationFn func(binding *appstudiosharedv1.SnapshotEnvironmentBindingStatus)) error {

	GinkgoWriter.Printf("Updating SnapshotEnvironmentBindingStatus for '%v'\n", binding.ObjectMeta)
	config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
	Expect(err).ToNot(HaveOccurred())

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

// HaveStatusGitOpsDeployments waits for a SnapshotEnvironmentBinding to have the expected .status.gitopsDeployments
// - Optionally, this function can be told to ignore the health, sync status, and commit ID fields, when comparing
func HaveStatusGitOpsDeployments(gitOpsDeployments []appstudiosharedv1.BindingStatusGitOpsDeployment, ignoreSyncHealthAndCommitID bool) matcher.GomegaMatcher {

	// compare compares two slices, returning true if the contents are equal regardless of the order of elements in the slices
	compare := func(a []appstudiosharedv1.BindingStatusGitOpsDeployment, b []appstudiosharedv1.BindingStatusGitOpsDeployment) string {
		if len(a) != len(b) {
			return "lengths don't match"
		}

		for _, aVal := range a {

			match := false
			for _, bVal := range b {

				if ignoreSyncHealthAndCommitID {
					// If the test doesn't care about these fields, clone them and clear the fields, excluding them from comparison
					aVal = *aVal.DeepCopy()
					aVal.GitOpsDeploymentCommitID = ""
					aVal.GitOpsDeploymentHealthStatus = ""
					aVal.GitOpsDeploymentSyncStatus = ""
					bVal = *bVal.DeepCopy()
					bVal.GitOpsDeploymentCommitID = ""
					bVal.GitOpsDeploymentHealthStatus = ""
					bVal.GitOpsDeploymentSyncStatus = ""
				}

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

	return expectedCondition(func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool {

		compareContents := compare(gitOpsDeployments, binding.Status.GitOpsDeployments)

		GinkgoWriter.Println("HaveStatusGitOpsDeployments:", compareContents, "/ Expected:", gitOpsDeployments, "/ Actual:", binding.Status.GitOpsDeployments)

		return compareContents == ""

	})

}

// HaveGitOpsDeploymentsWithStatusProperties compares the given, expected gitOpsDeployments with the actual SnapshotEnvironmentBinding
// object's deployments from the cluster.  Unlike HaveStatusGitOpsDeployments, this function does not match exactly the contents of
// BindingStatusGitOpsDeployment. Only the commit ID field's length is checked, whereas the other fields are matched exactly. This is
// because the test does not know the commit ID in a reliable, deterministic way and because the ID can change externally.
func HaveGitOpsDeploymentsWithStatusProperties(gitOpsDeployments []appstudiosharedv1.BindingStatusGitOpsDeployment) matcher.GomegaMatcher {

	// compare compares two slices, returning true if the fields other than the CommitID are equal, regardless of the order of elements in the slices
	compare := func(a []appstudiosharedv1.BindingStatusGitOpsDeployment, b []appstudiosharedv1.BindingStatusGitOpsDeployment) string {
		if len(a) != len(b) {
			return "lengths don't match"
		}

		for _, aDeployment := range a { // Expected
			match := false
			for _, bDeployment := range b { // Actual
				// Compare exactly the names and statuses but only check the length of the Commit ID since it is not deterministic.
				if aDeployment.ComponentName == bDeployment.ComponentName && aDeployment.GitOpsDeployment == bDeployment.GitOpsDeployment &&
					aDeployment.GitOpsDeploymentHealthStatus == bDeployment.GitOpsDeploymentHealthStatus &&
					aDeployment.GitOpsDeploymentSyncStatus == bDeployment.GitOpsDeploymentSyncStatus &&
					len(bDeployment.GitOpsDeploymentCommitID) > 0 && len(bDeployment.GitOpsDeploymentCommitID) <= 40 {
					match = true
					break
				}
			}
			if !match {
				return fmt.Sprintf("no match for %v", aDeployment)
			}
		}
		return ""
	}

	return WithTransform(func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		GinkgoWriter.Println("HaveStatusGitOpsDeploymentsWithProperties:", compareContents, "\n",
			"- Expected number of deployments: ", len(gitOpsDeployments),
		)
		for _, expectedDep := range gitOpsDeployments {
			GinkgoWriter.Println("",
				"            GitOpsDeployment: ", expectedDep.GitOpsDeployment, "\n",
				"            GitOpsDeploymentSyncStatus: ", expectedDep.GitOpsDeploymentSyncStatus, "\n",
				"            GitOpsDeploymentHealthStatus: ", expectedDep.GitOpsDeploymentHealthStatus, "\n",
				"            GitOpsDeploymentCommitID: ", expectedDep.GitOpsDeploymentCommitID,
			)
		}
		GinkgoWriter.Println(
			"does not match with the following deployments (not in any specific order):\n",
			"- Actual number of deployments: ", len(binding.Status.GitOpsDeployments),
		)
		for _, actualDep := range binding.Status.GitOpsDeployments {
			GinkgoWriter.Println("",
				"            GitOpsDeployment: ", actualDep.GitOpsDeployment, "\n",
				"            GitOpsDeploymentSyncStatus: ", actualDep.GitOpsDeploymentSyncStatus, "\n",
				"            GitOpsDeploymentHealthStatus: ", actualDep.GitOpsDeploymentHealthStatus, "\n",
				"            GitOpsDeploymentCommitID: ", actualDep.GitOpsDeploymentCommitID,
			)
		}
		GinkgoWriter.Println("------------------------------------------------------------------------------------")

		return compareContents == ""
	}, BeTrue())
}

func HaveComponentDeploymentCondition(expected metav1.Condition) matcher.GomegaMatcher {
	return WithTransform(func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		if len(binding.Status.ComponentDeploymentConditions) == 0 {
			GinkgoWriter.Println("HaveComponentDeploymentCondition: ComponentDeploymentConditions is nil")
			return false
		}
		actual := binding.Status.ComponentDeploymentConditions[0]
		GinkgoWriter.Println("HaveComponentDeploymentCondition:", "expected: ", expected, "actual: ", actual)
		return actual.Type == expected.Type &&
			actual.Status == expected.Status &&
			actual.Reason == expected.Reason &&
			actual.Message == expected.Message

	}, BeTrue())
}

func HaveBindingConditions(expected metav1.Condition) matcher.GomegaMatcher {
	return WithTransform(func(binding appstudiosharedv1.SnapshotEnvironmentBinding) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		if len(binding.Status.BindingConditions) == 0 {
			GinkgoWriter.Println("HaveBindingCondition: HaveBindingConditions is nil")
			return false
		}
		actual := binding.Status.BindingConditions[0]
		GinkgoWriter.Println("HaveBindingCondition:", "expected: ", expected, "actual: ", actual)
		return actual.Type == expected.Type &&
			actual.Status == expected.Status &&
			actual.Reason == expected.Reason &&
			actual.Message == expected.Message

	}, BeTrue())
}
