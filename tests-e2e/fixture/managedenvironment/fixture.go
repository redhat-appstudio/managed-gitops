package managedenvironment

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func HaveStatusCondition(conditionType string) matcher.GomegaMatcher {
	return WithTransform(func(menv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&menv), &menv)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		for _, condition := range menv.Status.Conditions {
			if condition.Type == conditionType {
				return true
			}
		}
		return false
	}, BeTrue())
}

// HaveAllowInsecureSkipTLSVerify checks if AllowInsecureSkipTLSVerify field of Environment is equal to ManagedEnvironment.
func HaveAllowInsecureSkipTLSVerify(allowInsecureSkipTLSVerify bool) matcher.GomegaMatcher {
	return WithTransform(func(menv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&menv), &menv)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		res := allowInsecureSkipTLSVerify == menv.Spec.AllowInsecureSkipTLSVerify

		fmt.Println("HaveAllowInsecureSkipTLSVerify:", res, "/ Expected:", allowInsecureSkipTLSVerify, "/ Actual:", menv.Spec.AllowInsecureSkipTLSVerify)

		return res

	}, BeTrue())
}

// HaveCredentials checks if the Managed Environment has the give cluster credentials.
func HaveCredentials(expectedEnvSpec managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec) matcher.GomegaMatcher {
	return WithTransform(func(env managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&env), &env)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		res := reflect.DeepEqual(env.Spec, expectedEnvSpec)

		fmt.Println("HaveCredentials: ", res, "/ Expected:", expectedEnvSpec, "/ Actual:", env.Spec)
		return res
	}, BeTrue())
}
