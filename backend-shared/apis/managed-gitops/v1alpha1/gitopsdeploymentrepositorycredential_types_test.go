package v1alpha1

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GitOpsDeploymentRepositoryCredential Type tests.", func() {

	Context("Testing SetConditions function for GitOpsDeploymentRepositoryCredential type.", func() {

		var conditions []metav1.Condition
		var status GitOpsDeploymentRepositoryCredentialStatus

		BeforeEach(func() {
			status = GitOpsDeploymentRepositoryCredentialStatus{}
			Expect(status.Conditions).To(BeEmpty())

			conditions = []metav1.Condition{
				{
					Type:    GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason:  RepositoryCredentialReasonSecretNotSpecified,
					Status:  metav1.ConditionTrue,
					Message: "Some error message.",
				},
			}
		})

		It("Should add new conditions entry in Status.Conditions field.", func() {

			By("Call function to update conditions in status.")

			status.SetConditions(conditions)

			By("Verify that status has expected conditions set.")

			Expect(status.Conditions).To(HaveLen(1))
			Expect(status.Conditions[0].Type).To(Equal(GitOpsDeploymentRepositoryCredentialConditionErrorOccurred))
			Expect(status.Conditions[0].Reason).To(Equal(RepositoryCredentialReasonSecretNotSpecified))
			Expect(status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(status.Conditions[0].Message).To(Equal("Some error message."))
		})

		It("Should not update existing condition if it is same.", func() {

			By("Call function to update conditions in status.")

			status.SetConditions(conditions)

			temp := status.Conditions[0]
			time.Sleep(1 * time.Second)

			By("Call function to update same conditions in status.")

			status.SetConditions(conditions)

			Expect(status.Conditions).To(HaveLen(1))

			By("Verify that status has same condition.")

			Expect(status.Conditions[0]).To(Equal(temp))
		})

		It("Should add new conditions entry in Status.Conditions field.", func() {

			By("Call function to update conditions in status.")

			status.SetConditions(conditions)

			temp := status.Conditions[0]

			By("Call function to update same conditions in status.")

			time.Sleep(1 * time.Second)

			conditions[0].Message = "another error message."

			status.SetConditions(conditions)

			Expect(status.Conditions).To(HaveLen(1))

			By("Verify that status has new conditions.")

			Expect(status.Conditions[0].Type).To(Equal(temp.Type))
			Expect(status.Conditions[0].Reason).To(Equal(temp.Reason))

			Expect(status.Conditions[0].Message).NotTo(Equal(temp.Message))
			Expect(status.Conditions[0].Message).To(Equal("another error message."))

			Expect(status.Conditions[0].LastTransitionTime).NotTo(Equal(temp.LastTransitionTime))
		})

		It("Should add new conditions and sort them entry in Status.Conditions field.", func() {

			conditions = append(conditions, []metav1.Condition{
				{
					Type:    GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason:  RepositoryCredentialReasonValidRepositoryUrl,
					Status:  metav1.ConditionTrue,
					Message: "Third error message.",
				}, {
					Type:    GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason:  RepositoryCredentialReasonErrorOccurred,
					Status:  metav1.ConditionTrue,
					Message: "First error message.",
				},
			}...)

			By("Call function to update conditions in status.")

			status.SetConditions(conditions)

			By("Verify that status has conditions in order.")

			Expect(status.Conditions).To(HaveLen(3))

			Expect(status.Conditions[0].Type).To(Equal(GitOpsDeploymentRepositoryCredentialConditionErrorOccurred))
			Expect(status.Conditions[0].Reason).To(Equal(RepositoryCredentialReasonErrorOccurred))
			Expect(status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(status.Conditions[0].Message).To(Equal("First error message."))

			Expect(status.Conditions[1].Type).To(Equal(GitOpsDeploymentRepositoryCredentialConditionErrorOccurred))
			Expect(status.Conditions[1].Reason).To(Equal(RepositoryCredentialReasonSecretNotSpecified))
			Expect(status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
			Expect(status.Conditions[1].Message).To(Equal("Some error message."))

			Expect(status.Conditions[2].Type).To(Equal(GitOpsDeploymentRepositoryCredentialConditionErrorOccurred))
			Expect(status.Conditions[2].Reason).To(Equal(RepositoryCredentialReasonValidRepositoryUrl))
			Expect(status.Conditions[2].Status).To(Equal(metav1.ConditionTrue))
			Expect(status.Conditions[2].Message).To(Equal("Third error message."))
		})
	})
})
