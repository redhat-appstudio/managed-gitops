package appstudio

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	dtfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttarget"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DeploymentTargetClaim Binding controller tests", func() {
	Context("Testing DeploymentTargetClaim binding controller", func() {
		var (
			k8sClient client.Client
			ctx       context.Context
			namespace string
			dtc       appstudiosharedv1.DeploymentTargetClaim
			dtcls     appstudiosharedv1.DeploymentTargetClass
			dt        appstudiosharedv1.DeploymentTarget
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			ctx = context.Background()
			namespace = fixture.GitOpsServiceE2ENamespace

			dtc = appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: "sandbox-provisioner",
				},
			}

			dtcls = appstudiosharedv1.DeploymentTargetClass{
                                ObjectMeta: metav1.ObjectMeta{
                                        Name:        "sandbox-provisioner",
                                        Namespace:   namespace,
                                        Annotations: map[string]string{},
                                },
                                Spec: appstudiosharedv1.DeploymentTargetClassSpec{
                                        Provisioner: appstudiosharedv1.Provisioner_Devsandbox,
                                        ReclaimPolicy: "Retain",
                                },
                        }

			err = k8sClient.Create(ctx, &dtcls)
			Expect(err).To(BeNil())

			dt = appstudiosharedv1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetSpec{
					DeploymentTargetClassName: "sandbox-provisioner",
					KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
						APIURL:                   "http://api-url",
						ClusterCredentialsSecret: "sample",
						DefaultNamespace:         "test",
					},
				},
			}

		})

		It("should handle a DTC with dynamic provisioning", func() {
			By("create a DTC without any target")
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("check if the provisioner annotation is added")
			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnTargetProvisioner,
					string(dtc.Spec.DeploymentTargetClassName)),
			)

			By("create a DT with a claim ref to the DTC")
			dt.Spec.ClaimRef = dtc.Name

			err = k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))
		})

		It("should handle a DTC that targets a user created DT", func() {
			By("create a DTC with a target")
			dtc.Spec.TargetName = dt.Name

			err := k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("check if the DTC is in Pending phase")
			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Pending),
			)

			By("create a DT that matches the above DTC")
			err = k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			By("verify if the DT and DTC are bounded")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))
		})

		It("should bind with a best match DT in the absence of provisioner/user created DT", func() {
			By("create a DT")
			err := k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			By("create a fake DT that matches a random DTC")
			fakedt := appstudiosharedv1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt-fake",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetSpec{
					ClaimRef:                  "random-dtc",
					DeploymentTargetClassName: "sandbox-provisioner",
					KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
						APIURL:                   "http://api-url",
						ClusterCredentialsSecret: "sample",
						DefaultNamespace:         "test",
					},
				},
			}

			err = k8sClient.Create(ctx, &fakedt)
			Expect(err).To(BeNil())

			By("create a DTC without any target")
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("verify if the DTC is binded with a matching DT")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
			Expect(err).To(BeNil())
			Expect(dtc.Spec.TargetName).Should(Equal(dt.Name))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))
		})

		It("should handle deletion of DTC and release binded DT", func() {
			By("create a DT")
			err := k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			By("create a DTC without any target")
			dtc := appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: "sandbox-provisioner",
				},
			}

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("verify if the DTC is binded with a matching DT")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
			Expect(err).To(BeNil())
			Expect(dtc.Spec.TargetName).Should(Equal(dt.Name))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			By("delete the DTC and verify if the binded DT is released")
			err = k8sClient.Delete(ctx, &dtc)
			Expect(err).To(BeNil())

			Eventually(&dtc, "2m", "1s").Should(k8s.NotExist(k8sClient))

			By("check if the binded DT is released")
			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Released))
		})

		It("should mark the DTC as Lost if its binded DT is not found", func() {
			By("create a DT")
			err := k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			By("create a DTC without any target")
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("verify if the DTC is binded with a matching DT")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
			Expect(err).To(BeNil())
			Expect(dtc.Spec.TargetName).Should(Equal(dt.Name))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			By("delete the DT and verify if the DTC is marked as Lost")
			err = k8sClient.Delete(ctx, &dt)
			Expect(err).To(BeNil())

			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Lost))
		})
	})
})
