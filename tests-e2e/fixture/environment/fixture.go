package environment

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1beta1 "github.com/redhat-appstudio/application-api/api/v1beta1"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

// This is intentionally NOT exported, for now. Create another function in this file/package that calls this function, and export that.
func expectedCondition(f func(env appstudiosharedv1beta1.Environment) bool) matcher.GomegaMatcher {

	return WithTransform(func(env appstudiosharedv1beta1.Environment) bool {

		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&env), &env)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		return f(env)

	}, BeTrue())

}

func HaveEmptyEnvironmentConditions() matcher.GomegaMatcher {

	return expectedCondition(func(env appstudiosharedv1beta1.Environment) bool {

		fmt.Println("EmptyEnvironmentConditions, env.Status.Conditions is:", env.Status.Conditions)

		return env.Status.Conditions == nil || len(env.Status.Conditions) == 0
	})
}

func HaveEnvironmentCondition(expected metav1.Condition) matcher.GomegaMatcher {
	sanitizeCondition := func(cond metav1.Condition) metav1.Condition {

		res := metav1.Condition{
			Type:    cond.Type,
			Message: cond.Message,
			Status:  cond.Status,
			Reason:  cond.Reason,
		}

		return res

	}

	return expectedCondition(func(env appstudiosharedv1beta1.Environment) bool {
		if len(env.Status.Conditions) == 0 {
			GinkgoWriter.Println("HaveEnvironmentCondition: Conditions is nil")
			return false
		}

		for i := range env.Status.Conditions {
			actual := sanitizeCondition(env.Status.Conditions[i])
			if actual.Type == expected.Type &&
				actual.Status == expected.Status &&
				actual.Reason == expected.Reason &&
				actual.Message == expected.Message {
				return true
			}
		}

		GinkgoWriter.Println("HaveEnvironmentCondition:", "expected to find: ", expected, "in slice: ", env.Status.Conditions)
		return false
	})
}
