package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/managedenvironment"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Managed Environment Status E2E tests", func() {

	Context("Create a ManagedEnvironment", func() {
		var k8sClient client.Client
		BeforeEach(func() {
			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())
		})

		It("should have a connection status condition of False when the host for the targeted cluster does not exist", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a target cluster that does not exist")

			apiServerURL := "https://api2.fake-e2e-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"
			managedEnv, secret := managedenvironment.BuildManagedEnvironment(apiServerURL, k8s.GenerateFakeKubeConfig(), true)

			err := k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the managed environment has a connection status condition of Failed")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Or(
				Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToCreateClient)),
				Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToInstallServiceAccount)),
			))
			Expect(condition.Message).To(ContainSubstring("no such host"))

		})

		It("should have a connection status condition of False when the credentials are missing a required field", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a secret that is missing the kubeconfig data")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())
			managedEnv, secret := managedenvironment.BuildManagedEnvironment(apiServerURL, kubeConfigContents, true)
			delete(secret.StringData, "kubeconfig")

			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the managed environment connection status condition has a reason of MissingKubeConfigField")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonMissingKubeConfigField)))
			Expect(condition.Message).To(ContainSubstring("missing kubeconfig field in Secret"))
		})

		It("should have a connection status condition of False when the credentials contain bad data", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a secret that has bad kubeconfig data")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())
			managedEnv, secret := managedenvironment.BuildManagedEnvironment(apiServerURL, kubeConfigContents, true)
			secret.StringData["kubeconfig"] = "badbadbad"

			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the managed environment connection status condition has a reason of UnableToParseKubeconfigData")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToParseKubeconfigData)))
			Expect(condition.Message).To(ContainSubstring("json parse error"))
		})

		It("should have a connection status condition of False when there is no current context in the kubeconfig", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a secret that lacks a kubeconfig context")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())
			managedEnv, secret := managedenvironment.BuildManagedEnvironment(apiServerURL, kubeConfigContents, true)
			secret.StringData["kubeconfig"] = "apiVersion: v1\nkind: Config\n"

			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the managed environment connection status condition has a reason of UnableToLocateContext")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToLocateContext)))
			Expect(condition.Message).To(ContainSubstring("the kubeconfig did not have a cluster entry that matched the API URL"))
		})

		It("should have a connection status condition of True when the connection to the target cluster is successful", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			managedEnv, secret := managedenvironment.BuildManagedEnvironment(apiServerURL, kubeConfigContents, true)

			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the managed environment has a connection status condition of True")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("Succeeded"))
			Expect(condition.Message).To(BeEmpty())
		})
	})
})
