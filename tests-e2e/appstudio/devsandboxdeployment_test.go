package appstudio

import (
	"context"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"

	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"

	spfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/spacerequest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Devsandbox deployment controller tests", func() {
	Context("Testing Devsandbox deployment controller", func() {
		var (
			k8sClient    client.Client
			ctx          context.Context
			namespace    string
			dtc          appstudiosharedv1.DeploymentTargetClaim
			dt           appstudiosharedv1.DeploymentTarget
			spacerequest codereadytoolchainv1alpha1.SpaceRequest
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			ctx = context.Background()
			namespace = fixture.GitOpsServiceE2ENamespace

			spacerequest = codereadytoolchainv1alpha1.SpaceRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-spacerequest",
					Namespace: namespace,
					Labels: map[string]string{
						appstudiocontrollers.DeploymentTargetClaimLabel: fixture.DTCName,
					},
					Annotations: map[string]string{},
				},
				Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{
					TierName:           "test-tiername",
					TargetClusterRoles: []string{"test-role"},
				},
				Status: codereadytoolchainv1alpha1.SpaceRequestStatus{
					TargetClusterURL: "https://api-url/test-api",
					NamespaceAccess: []codereadytoolchainv1alpha1.NamespaceAccess{
						{
							Name:      "test-ns",
							SecretRef: "test-secret",
						},
					},
					Conditions: []codereadytoolchainv1alpha1.Condition{
						{
							Type:   codereadytoolchainv1alpha1.ConditionReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}

			dtc = appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.DTCName,
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class"),
				},
			}

			dt = appstudiosharedv1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:        fixture.DTName,
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: appstudiosharedv1.DeploymentTargetSpec{
					DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class"),
					KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
						DefaultNamespace:         "test-ns",
						APIURL:                   "https://api-url/test-api",
						ClusterCredentialsSecret: "test-secret",
					},
					ClaimRef: fixture.DTCName,
				},
			}
		})

		It("should create a DeploymentTarget for a SpaceRequest with dtc Label", func() {
			By("create a DTC ")
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("create a SpaceRequest")
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			patch := client.MergeFrom(spacerequest.DeepCopy())

			spacerequest.Labels[appstudiocontrollers.DeploymentTargetClaimLabel] = fixture.DTCName
			cond := codereadytoolchainv1alpha1.Condition{
				Type:   codereadytoolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
				Reason: codereadytoolchainv1alpha1.SpaceProvisioningReason,
			}
			spacerequest.Status.Conditions = append(spacerequest.Status.Conditions, cond)

			namespaceaccess := codereadytoolchainv1alpha1.NamespaceAccess{
				Name:      namespace,
				SecretRef: "test-secret",
			}
			spacerequest.Status.NamespaceAccess = append(spacerequest.Status.NamespaceAccess, namespaceaccess)

			err = k8sClient.Status().Patch(ctx, &spacerequest, patch)
			Expect(err).ToNot(HaveOccurred())

			Eventually(spacerequest, "3m", "1s").Should(spfixture.HasStatus(corev1.ConditionTrue))
			ready := condition.IsTrue(spacerequest.Status.Conditions, codereadytoolchainv1alpha1.ConditionReady)
			Expect(ready).To(BeTrue())
			By("check if a matching DeploymentTarget has been created")
			Eventually(spacerequest, "2m", "1s").Should(spfixture.HasANumberOfMatchingDTs(1))
		})

		It("should not create an extra DeploymentTarget for a SpaceRequest with an existing DT ", func() {
			By("create a DTC ")
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("create a DT")
			err = k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("create a SpaceRequest")
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			patch := client.MergeFrom(spacerequest.DeepCopy())

			spacerequest.Labels[appstudiocontrollers.DeploymentTargetClaimLabel] = fixture.DTCName
			cond := codereadytoolchainv1alpha1.Condition{
				Type:   codereadytoolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
				Reason: codereadytoolchainv1alpha1.SpaceProvisioningReason,
			}
			spacerequest.Status.Conditions = append(spacerequest.Status.Conditions, cond)

			namespaceaccess := codereadytoolchainv1alpha1.NamespaceAccess{
				Name:      namespace,
				SecretRef: "test-secret",
			}
			spacerequest.Status.NamespaceAccess = append(spacerequest.Status.NamespaceAccess, namespaceaccess)

			err = k8sClient.Status().Patch(ctx, &spacerequest, patch)
			Expect(err).ToNot(HaveOccurred())

			Eventually(spacerequest, "3m", "1s").Should(spfixture.HasStatus(corev1.ConditionTrue))
			ready := condition.IsTrue(spacerequest.Status.Conditions, codereadytoolchainv1alpha1.ConditionReady)
			Expect(ready).To(BeTrue())

			By("check if a matching DeploymentTarget has been created")
			Eventually(spacerequest, "2m", "1s").Should(spfixture.HasANumberOfMatchingDTs(1))
		})

		It("shouldn't create a DeploymentTarget for a SpaceRequest with wrong value for dtc Label", func() {
			By("create a DTC ")
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			spacerequest.Labels[appstudiocontrollers.DeploymentTargetClaimLabel] = "wrongvalue"
			By("create a SpaceRequest")
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			By("check if a matching DeploymentTarget hasn't been created")
			Eventually(spacerequest, "2m", "1s").Should(spfixture.HasANumberOfMatchingDTs(0))
		})

	})
})
