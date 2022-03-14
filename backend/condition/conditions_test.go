package condition

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ConditionManager", func() {
	var sut []gitopsv1alpha1.GitOpsDeploymentCondition
	conditionManager := NewConditionManager()
	errorOccured := gitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred
	reason := gitopsv1alpha1.GitopsDeploymentReasonErrorOccurred
	message := "fake error"

	Context("when SetCondition() called with status condition true", func() {
		status := gitopsv1alpha1.GitOpsConditionStatusTrue // set condition status true
		BeforeEach(func() {
			sut = []gitopsv1alpha1.GitOpsDeploymentCondition{}
		})
		It("should update the condition list", func() {
			conditionManager.SetCondition(&sut, errorOccured, status, reason, message)

			Expect(len(sut)).To(Equal(1))
			obj := getFirst(sut)
			Expect(obj.Status).To(Equal(status))
			Expect(obj.Message).To(Equal(message))
			Expect(obj.Reason).To(Equal(reason))
			Expect(obj.Type).To(Equal(errorOccured))
		})
		It("should update the fields with given parameters except for LastProbeTime if there's one existing condition", func() {
			// Set Existing condition
			conditionManager.SetCondition(&sut, errorOccured, status, reason, message)
			// Get current values
			obj := getFirst(sut)
			probe := obj.LastTransitionTime
			transition := obj.LastTransitionTime

			conditionManager.SetCondition(&sut, errorOccured, status, reason, message)
			obj = getFirst(sut)

			Expect(len(sut)).To(Equal(1))
			Expect(obj.LastProbeTime).NotTo(Equal(probe))
			Expect(obj.LastTransitionTime).To(Equal(transition))
			Expect(obj.Message).To(Equal(message))
			Expect(obj.Reason).To(Equal(reason))
		})
	})

	Context("when SetCondition() called with status condition false", func() {
		status := gitopsv1alpha1.GitOpsConditionStatusFalse
		now := metav1.Now()
		BeforeEach(func() {
			sut = []gitopsv1alpha1.GitOpsDeploymentCondition{}
		})
		It("should mark the existing condition as resolved", func() {
			// Set existing condition
			sut = append(sut, gitopsv1alpha1.GitOpsDeploymentCondition{
				Message:            "Dummy Error Fake Message",
				Status:             gitopsv1alpha1.GitOpsConditionStatusTrue,
				LastTransitionTime: &now,
				LastProbeTime:      now,
				Reason:             "Dummy",
				Type:               errorOccured,
			})

			conditionManager.SetCondition(&sut, errorOccured, status, gitopsv1alpha1.GitOpsDeploymentReasonType("DummyResolved"), "Dummy Error Fake Message")
			obj := getFirst(sut)
			Expect(obj.Message).To(Equal("Dummy Error Fake Message"))
			Expect(obj.Reason).To(Equal(gitopsv1alpha1.GitOpsDeploymentReasonType("DummyResolved")))
			Expect(obj.Status).To(Equal(status))
			Expect(obj.LastProbeTime).NotTo(Equal(now))
			Expect(obj.LastTransitionTime).NotTo(Equal(now))
		})
	})

	Context("when SetCondition() called with status condition true and a new err message", func() {
		status := gitopsv1alpha1.GitOpsConditionStatusTrue
		now := metav1.Now()
		BeforeEach(func() {
			sut = []gitopsv1alpha1.GitOpsDeploymentCondition{}
		})
		It("should set a new error condition", func() {
			// Set existing condition
			sut = append(sut, gitopsv1alpha1.GitOpsDeploymentCondition{
				Message:            "DummyError",
				Status:             gitopsv1alpha1.GitOpsConditionStatusFalse,
				LastTransitionTime: &now,
				LastProbeTime:      now,
				Reason:             "DummyResolved",
				Type:               errorOccured,
			})
			old := getFirst(sut)
			conditionManager.SetCondition(&sut, errorOccured, status, gitopsv1alpha1.GitOpsDeploymentReasonType("SecondFakeReconcileError"), "SecondFakeReconcileMessage")
			obj := getFirst(sut)
			Expect(obj.Message).To(Equal("SecondFakeReconcileMessage"))
			Expect(obj.Reason).To(Equal(gitopsv1alpha1.GitOpsDeploymentReasonType("SecondFakeReconcileError")))
			Expect(obj.Status).To(Equal(status))
			Expect(obj.LastTransitionTime).NotTo(Equal(old.LastTransitionTime))
			Expect(obj.LastProbeTime).NotTo(Equal(old.LastProbeTime))
		})
	})

	Context("when HasCondition() called with an already existing condition", func() {
		status := gitopsv1alpha1.GitOpsConditionStatusTrue
		BeforeEach(func() {
			sut = []gitopsv1alpha1.GitOpsDeploymentCondition{}
		})
		It("should return true", func() {
			conditionManager.SetCondition(&sut, errorOccured, status, reason, message)
			result := conditionManager.HasCondition(&sut, errorOccured)
			Expect(result).To(BeTrue())
		})
	})

	Context("when HasCondition() called without an existing condition", func() {
		BeforeEach(func() {
			sut = []gitopsv1alpha1.GitOpsDeploymentCondition{}
		})
		It("should return false", func() {
			result := conditionManager.HasCondition(&sut, errorOccured)
			Expect(result).To(BeFalse())
		})
	})
})

func getFirst(list []gitopsv1alpha1.GitOpsDeploymentCondition) gitopsv1alpha1.GitOpsDeploymentCondition {
	return list[0]
}
