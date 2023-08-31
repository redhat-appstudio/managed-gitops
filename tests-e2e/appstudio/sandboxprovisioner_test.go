package appstudio

import (
	"context"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Sandbox Provisioner controller tests", func() {
	Context("Testing Sandbox Provisioner controller", func() {
		var (
			k8sClient client.Client
			ctx       context.Context
			namespace string
			dtc       appstudiosharedv1.DeploymentTargetClaim
			dtcls     appstudiosharedv1.DeploymentTargetClass
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			ctx = context.Background()
			namespace = fixture.GitOpsServiceE2ENamespace

			dtcls = appstudiosharedv1.DeploymentTargetClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "sandbox-provisioner",
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: appstudiosharedv1.DeploymentTargetClassSpec{
					Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
					ReclaimPolicy: appstudiosharedv1.ReclaimPolicy_Retain,
				},
			}

			dtc = appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sandbox-dtc",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName(dtcls.Name),
				},
			}
		})

		It("should create a SpaceRequest for a DTC with dynamic provisioning", func() {
			By("create a DTCLS with a sandbox provisioner")
			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			By("create a DTC with a sandbox provisioned DTCLS")
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("check if the provisioner annotation is added")
			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnTargetProvisioner,
					string(dtc.Spec.DeploymentTargetClassName)),
			)

			By("check if a matching SpaceRequest has been created")
			Eventually(dtc, "2m", "1s").Should(dtcfixture.HasANumberOfMatchingSpaceRequests(1))
		})

		It("should not create an extra SpaceRequest for a DTC when one already exists", func() {
			By("create a DTCLS with a sandbox provisioner")
			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			By("create the matching SpaceRequest for the DTC")
			spaceRequest := codereadytoolchainv1alpha1.SpaceRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-sandbox-spacerequest",
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{
					TierName:           "appstudio-env",
					TargetClusterRoles: []string{},
				},
			}

			spaceRequest.Labels = map[string]string{
				appstudiocontrollers.DeploymentTargetClaimLabel: dtc.Name,
			}

			err = k8sClient.Create(ctx, &spaceRequest)
			Expect(err).ToNot(HaveOccurred())

			By("create a DTC with a sandbox provisioned DTCLS")
			dtc.Name = "new-sandbox-dtc"
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("check there is still just one matching SpaceRequest")
			Eventually(dtc, "2m", "1s").Should(dtcfixture.HasANumberOfMatchingSpaceRequests(1))
		})

		It("should not create a SpaceRequest for a DTC with a non-sandbox DTCLS", func() {
			By("create a DTCLS with a non-sandbox provisioner")
			dtcls.Spec.Provisioner = "non-sandbox"
			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			By("create a DTC with a non-sandbox provisioned DTCLS")
			dtc.Name = "new-non-sandbox-dtc"
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("check if the provisioner annotation is added")
			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnTargetProvisioner,
					string(dtc.Spec.DeploymentTargetClassName)),
			)

			By("check there is zero matching SpaceRequests")
			Consistently(dtc, "1m", "10s").Should(dtcfixture.HasANumberOfMatchingSpaceRequests(0))
		})
	})
})
