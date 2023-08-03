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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func HaveStatusCondition(conditionType string) matcher.GomegaMatcher {
	return WithTransform(func(menv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

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

// HaveClusterResources checks if the ClusterResources field of GitOpsDeploymentManagedEnvironment is equal to clusterResouces.
func HaveClusterResources(clusterResources bool) matcher.GomegaMatcher {
	return WithTransform(func(menv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		res := clusterResources == menv.Spec.ClusterResources

		fmt.Println("HaveClusterResources:", res, "/ Expected:", clusterResources, "/ Actual:", menv.Spec.ClusterResources)

		return res

	}, BeTrue())
}

// HaveNamespaces checks if the HaveNamespaces field of GitOpsDeploymentManagedEnvironment is equal to namespaces.
func HaveNamespaces(namespaces []string) matcher.GomegaMatcher {
	return WithTransform(func(menv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		res := reflect.DeepEqual(namespaces, menv.Spec.Namespaces)

		fmt.Println("HaveNamespaces:", res, "/ Expected:", namespaces, "/ Actual:", menv.Spec.Namespaces)

		return res

	}, BeTrue())
}

func BuildManagedEnvironment(apiServerURL string, kubeConfigContents string, createNewServiceAccount bool) (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-managed-env-secret",
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Type:       "managed-gitops.redhat.com/managed-environment",
		StringData: map[string]string{"kubeconfig": kubeConfigContents},
	}

	managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-managed-env",
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			APIURL:                     apiServerURL,
			ClusterCredentialsSecret:   secret.Name,
			AllowInsecureSkipTLSVerify: true,
			CreateNewServiceAccount:    createNewServiceAccount,
		},
	}

	return *managedEnv, *secret
}
