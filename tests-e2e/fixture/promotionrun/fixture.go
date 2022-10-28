package binding

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

func HaveStatusComplete(expectedPromotionRunStatus appstudiosharedv1.PromotionRunStatus) matcher.GomegaMatcher {
	return WithTransform(func(promotionRun appstudiosharedv1.PromotionRun) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&promotionRun), &promotionRun)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		// Set same time in both objects to avoid comparison failure due to time.
		now := v1.Now()
		promotionRun.Status.PromotionStartTime = now
		expectedPromotionRunStatus.PromotionStartTime = now

		// To validate Status.Conditions field.
		for i := 0; i < len(promotionRun.Status.Conditions); i++ {
			promotionRun.Status.Conditions[i].LastTransitionTime = &now
			promotionRun.Status.Conditions[i].LastProbeTime = now
		}

		for i := 0; i < len(expectedPromotionRunStatus.Conditions); i++ {
			expectedPromotionRunStatus.Conditions[i].LastTransitionTime = &now
			expectedPromotionRunStatus.Conditions[i].LastProbeTime = now
		}

		res := reflect.DeepEqual(promotionRun.Status, expectedPromotionRunStatus)

		GinkgoWriter.Println("HaveStatusComplete:", res, "/ Expected:", expectedPromotionRunStatus, "/ Actual:", promotionRun.Status)
		return res
	}, BeTrue())
}

func HaveStatusConditions(expectedPromotionRunStatusConditions appstudiosharedv1.PromotionRunStatus) matcher.GomegaMatcher {
	return WithTransform(func(promotionRun appstudiosharedv1.PromotionRun) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&promotionRun), &promotionRun)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		// Set same time in both objects to avoid comparison failure due to time.
		now := v1.Now()
		promotionRun.Status.Conditions[0].LastProbeTime = now
		promotionRun.Status.Conditions[0].LastTransitionTime = &now

		expectedPromotionRunStatusConditions.Conditions[0].LastProbeTime = now
		expectedPromotionRunStatusConditions.Conditions[0].LastTransitionTime = &now

		res := reflect.DeepEqual(promotionRun.Status.Conditions, expectedPromotionRunStatusConditions.Conditions)

		GinkgoWriter.Println("HaveStatusConditions:", res, "/ Expected:", expectedPromotionRunStatusConditions, "/ Actual:", promotionRun.Status.Conditions)

		return res
	}, BeTrue())
}
