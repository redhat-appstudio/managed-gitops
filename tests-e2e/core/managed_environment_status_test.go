package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/managedenvironment"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Managed Environment Status E2E tests", func() {

	Context("Create a ManagedEnvironment", func() {

		It("should have a connection status condition of False when the host for the targeted cluster does not exist", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a target cluster that does not exist")

			apiServerURL := "https://api2.fake-e2e-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"
			managedEnv, secret := buildManagedEnvironment(apiServerURL, generateFakeKubeConfig())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring the managed environment has a connection status condition of Failed")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).To(BeNil())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(shared_resource_loop.ConditionReasonUnableToCreateClient))
			Expect(condition.Message).To(ContainSubstring("no such host"))
		})

		It("should have a connection status condition of False when the credentials are missing a required field", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a secret that is missing the kubeconfig data")

			kubeConfigContents, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())
			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents)
			delete(secret.StringData, "kubeconfig")

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring the managed environment connection status condition has a reason of MissingKubeConfigField")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).To(BeNil())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(shared_resource_loop.ConditionReasonMissingKubeConfigField))
			Expect(condition.Message).To(ContainSubstring("missing kubeconfig field in Secret"))
		})

		It("should have a connection status condition of False when the credentials contain bad data", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a secret that has bad kubeconfig data")

			kubeConfigContents, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())
			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents)
			secret.StringData["kubeconfig"] = "badbadbad"

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring the managed environment connection status condition has a reason of UnableToParseKubeconfigData")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).To(BeNil())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(shared_resource_loop.ConditionReasonUnableToParseKubeconfigData))
			Expect(condition.Message).To(ContainSubstring("json parse error"))
		})

		It("should have a connection status condition of False when there is no current contex in the kubeconfig", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment with a secret that lacks a kubeconfig context")

			kubeConfigContents, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())
			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents)
			secret.StringData["kubeconfig"] = "apiVersion: v1\nkind: Config\n"

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring the managed environment connection status condition has a reason of UnableToLocateContext")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).To(BeNil())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(shared_resource_loop.ConditionReasonUnableToLocateContext))
			Expect(condition.Message).To(ContainSubstring("the kubeconfig did not have a cluster entry that matched the API URL"))
		})

		It("should have a connection status condition of True when the connection to the target cluster is successful", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment")

			kubeConfigContents, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())

			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring the managed environment has a connection status condition of True")
			Eventually(managedEnv, "2m", "1s").Should(managedenvironment.HaveStatusCondition(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			err = k8s.Get(&managedEnv, k8sClient)
			Expect(err).To(BeNil())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			condition := managedEnv.Status.Conditions[0]
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("Succeeded"))
			Expect(condition.Message).To(BeEmpty())
		})
	})
})

func generateFakeKubeConfig() string {
	// This config has been sanitized of any real credentials.
	return `
apiVersion: v1
kind: Config
clusters:
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api.fake-e2e-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api2.fake-e2e-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
contexts:
  - context:
      cluster: api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
  - context:
      cluster: api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
current-context: default/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
preferences: {}
users:
  - name: kube:admin/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      token: sha256~ABCdEF1gHiJKlMnoP-Q19qrTuv1_W9X2YZABCDefGH4
  - name: kube:admin/api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      token: sha256~abcDef1gHIjkLmNOp-q19QRtUV1_w9x2yzabcdEFgh4
`
}
