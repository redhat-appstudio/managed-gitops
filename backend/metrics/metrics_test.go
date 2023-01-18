package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var _ = Describe("Test for Gitopsdeployment metrics counter", func() {

	Context("Prometheus metrics responds to count of active/failed GitopsDeployments", func() {
		It("tests Add/Update, Remove, SetError function on a gitops deployment", func() {

			ClearMetrics()

			numberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(GitopsdeplFailures)
			totalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(Gitopsdepl)

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: "gitops-depl-namespace",
					UID:       uuid.NewUUID(),
				},
			}
			By("simulating a new valid GitOpsDeployment being processed")
			AddOrUpdateGitOpsDeployment(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID))
			newTotalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics + 1))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))

			By("setting the GitOpsDeployment to an error state")
			SetErrorState(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID), true)
			newTotalNumberOfGitOpsDeploymentMetrics = testutil.ToFloat64(Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState = testutil.ToFloat64(GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics + 1))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState + 1))

			By("setting the GitOpsDeployment to a non-error state")
			SetErrorState(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID), false)
			newTotalNumberOfGitOpsDeploymentMetrics = testutil.ToFloat64(Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState = testutil.ToFloat64(GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics + 1))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))

			By("removing the simulated GitOpsDeployment from prometheus")
			RemoveGitOpsDeployment(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID))
			newTotalNumberOfGitOpsDeploymentMetrics = testutil.ToFloat64(Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState = testutil.ToFloat64(GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))

		})
	})

	Context("Prometheus metrics keep an accurate count of failed deployments", func() {
		It("tests AddOrUpdateGitOpsDeployment doesn't overwite error indicator for a failed deployment", func() {

			ClearMetrics()

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: "gitops-depl-namespace",
					UID:       uuid.NewUUID(),
				},
			}

			key := generateMapKey(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID))

			By("simulating a new valid GitOpsDeployment being processed")
			AddOrUpdateGitOpsDeployment(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID))
			Expect(activeGitOpsDeployments.gitOpsDeployments[key]).To(BeFalse())

			By("setting the GitOpsDeployment to an error state")
			SetErrorState(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID), true)
			Expect(activeGitOpsDeployments.gitOpsDeployments[key]).To(BeTrue())

			By("simulating the GitOpsDeployment being reprocessed")
			AddOrUpdateGitOpsDeployment(gitopsDepl.Name, gitopsDepl.Namespace, string(gitopsDepl.UID))
			Expect(activeGitOpsDeployments.gitOpsDeployments[key]).To(BeTrue())
		})
	})
})
