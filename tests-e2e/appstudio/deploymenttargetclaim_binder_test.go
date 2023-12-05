package appstudio

import (
	"context"
	"fmt"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	dtfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttarget"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DeploymentTargetClaim Binding controller tests", func() {

	Context("Testing DeploymentTargetClaim binding controller with DeploymentTargetClass with Retain policy", func() {
		const dtClassName = "sandbox-provisioner"
		var (
			k8sClient client.Client
			ctx       context.Context
			namespace string
			dtc       appstudiosharedv1.DeploymentTargetClaim
			dtcls     appstudiosharedv1.DeploymentTargetClass
			dt        appstudiosharedv1.DeploymentTarget
			err       error
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			ctx = context.Background()
			namespace = fixture.GitOpsServiceE2ENamespace

			dtc = appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.DTCName,
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: dtClassName,
				},
			}

			dtcls = appstudiosharedv1.DeploymentTargetClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        dtClassName,
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: appstudiosharedv1.DeploymentTargetClassSpec{
					Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
					ReclaimPolicy: appstudiosharedv1.ReclaimPolicy_Retain,
				},
			}

			err = k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dt = appstudiosharedv1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.DTName,
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetSpec{
					DeploymentTargetClassName: dtClassName,
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
			Expect(err).ToNot(HaveOccurred())

			By("check if the provisioner annotation is added")
			Eventually(dtc, "5m", "1s").Should(
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnTargetProvisioner,
					string(dtc.Spec.DeploymentTargetClassName)),
			)

			By("create a DT with a claim ref to the DTC")
			dt.Spec.ClaimRef = dtc.Name

			err = k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "5m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			Eventually(&dt).Should(k8s.HasFinalizers([]string{appstudiocontrollers.FinalizerDT}, k8sClient))

			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))
		})

		It("should handle a DTC that targets a user created DT", func() {
			By("create a DTC with a target")
			dtc.Spec.TargetName = dt.Name

			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("check if the DTC is in Pending phase")
			Eventually(dtc, "5m", "1s").Should(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Pending),
			)

			By("create a DT that matches the above DTC")
			err = k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DT and DTC are bounded")
			Eventually(dtc, "5m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			Eventually(&dt).Should(k8s.HasFinalizers([]string{appstudiocontrollers.FinalizerDT}, k8sClient))
		})

		It("should bind with a best match DT in the absence of provisioner/user created DT", func() {
			By("create a fake DTC")
			// If the DTC doesn't exist, the DeploymentTargetReclaimer controller will
			// unset the claimRef of a DT
			fakeDTC := appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc-fake",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: dtClassName,
				},
			}
			err = k8sClient.Create(ctx, &fakeDTC)
			Expect(err).ToNot(HaveOccurred())

			By("create a fake DT that matches the above DTC")
			fakeDT := appstudiosharedv1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt-fake",
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetSpec{
					ClaimRef:                  fakeDTC.Name,
					DeploymentTargetClassName: dtClassName,
					KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
						APIURL:                   "http://api-url",
						ClusterCredentialsSecret: "sample",
						DefaultNamespace:         "test",
					},
				},
			}

			err = k8sClient.Create(ctx, &fakeDT)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the above DTC is binded with a matching DT")
			Eventually(fakeDTC, "5m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			Eventually(fakeDT, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			By("create a new DT to verify if it will be binded")
			err := k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("create a DTC without any target")
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DTC is binded with a matching DT")
			Eventually(dtc, "5m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
			Expect(err).ToNot(HaveOccurred())
			Expect(dtc.Spec.TargetName).Should(Equal(dt.Name))

			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))
		})

		It("should handle deletion of DTC and release binded DT", func() {
			By("create a DT")
			err := k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("create a DTC without any target")
			dtc := appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.DTCName,
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: dtClassName,
				},
			}

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DTC is binded with a matching DT")
			Eventually(dtc, "5m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
			Expect(err).ToNot(HaveOccurred())
			Expect(dtc.Spec.TargetName).Should(Equal(dt.Name))

			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			By("delete the DTC and verify if the binded DT is released")
			err = k8sClient.Delete(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			Eventually(&dtc, "5m", "1s").Should(k8s.NotExist(k8sClient))

			By("check if the binded DT is released")
			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Released))
		})

		It("should mark the DTC as Lost if its binded DT is not found", func() {
			By("create a DT")
			err := k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("create a DTC without any target")
			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DTC is binded with a matching DT")
			Eventually(dtc, "5m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
			Expect(err).ToNot(HaveOccurred())
			Expect(dtc.Spec.TargetName).Should(Equal(dt.Name))

			Eventually(dt, "5m", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			By("delete the DT and verify if the DTC is marked as Lost")
			err = k8sClient.Delete(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			Eventually(dtc, "5m", "1s").Should(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Lost))
		})
	})

	Context("Testing DeploymentTargetClaim binding controller with DeploymentTargetClass with Delete policy", func() {
		const dtClassName = "sandbox-provisioner"
		var (
			k8sClient client.Client
			ctx       context.Context
			namespace string
			dtc       appstudiosharedv1.DeploymentTargetClaim
			dtcls     appstudiosharedv1.DeploymentTargetClass
			err       error
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			ctx = context.Background()
			namespace = fixture.GitOpsServiceE2ENamespace

			dtc = appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fixture.DTCName,
					Namespace: namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: dtClassName,
				},
			}

			dtcls = appstudiosharedv1.DeploymentTargetClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        dtClassName,
					Namespace:   namespace,
					Annotations: map[string]string{},
				},
				Spec: appstudiosharedv1.DeploymentTargetClassSpec{
					Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
					ReclaimPolicy: appstudiosharedv1.ReclaimPolicy_Delete,
				},
			}

			err = k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

		})

		It("should handle a DTC with dynamic provisioning", func() {
			By("create a DTC without any target")
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("check if the provisioner annotation is added")
			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnTargetProvisioner,
					string(dtc.Spec.DeploymentTargetClassName)),
			)

			By("verifying a SpaceRequest was created that points to the DT")

			var spaceRequest *codereadytoolchainv1alpha1.SpaceRequest

			// A SpaceRequest should exist that references the DT and DTC
			Eventually(func() bool {
				var spaceRequestList codereadytoolchainv1alpha1.SpaceRequestList
				Expect(k8sClient.List(ctx, &spaceRequestList)).To(Succeed())

				for i, sr := range spaceRequestList.Items {

					if sr.Labels[appstudiocontrollers.DeploymentTargetClaimLabel] == dtc.Name {
						spaceRequest = &spaceRequestList.Items[i]

						return true
					}

				}

				return false

			}, "20s", "1s").Should(BeTrue())

			Expect(spaceRequest).ToNot(BeNil())

			By("simulating the SpaceRequest becoming ready")
			spaceRequest.Status.NamespaceAccess = []codereadytoolchainv1alpha1.NamespaceAccess{{Name: "namespace", SecretRef: "secret"}}
			spaceRequest.Status.TargetClusterURL = "https://fake-cluster-url"
			spaceRequest.Status.Conditions = []codereadytoolchainv1alpha1.Condition{{
				Type:   codereadytoolchainv1alpha1.ConditionReady,
				Status: "True",
			}}
			Expect(k8sClient.Status().Update(ctx, spaceRequest)).To(Succeed())

			var dt *appstudiosharedv1.DeploymentTarget
			Eventually(func() bool {

				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc); err != nil {
					fmt.Println(err)
					return false
				}

				if dtc.Spec.TargetName == "" {
					return false
				}

				dt = &appstudiosharedv1.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dtc.Spec.TargetName,
						Namespace: dtc.Namespace,
					},
				}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dt), dt); err != nil {
					fmt.Println(err)
					return false
				}

				return true

			}, "30s", "1s").Should(BeTrue())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "30s", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue),
			))

			Eventually(dt, "30s", "1s").Should(k8s.HasFinalizers([]string{appstudiocontrollers.FinalizerDT}, k8sClient))

			Eventually(*dt, "30s", "1s").Should(
				dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			Eventually(spaceRequest, "20s", "1s").Should(k8s.HasLabel(appstudiocontrollers.DeploymentTargetLabel, dt.Name, k8sClient))

			By("deleting the DTC, which should trigger the deletion of DT/SR")
			Expect(k8sClient.Delete(ctx, &dtc)).To(Succeed())

			Eventually(&dtc, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
			Eventually(dt, "30s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))
			Eventually(spaceRequest, "30s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))

			By("removing the finalizer from SpaceRequest, which should cause it to be deleted")

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(spaceRequest), spaceRequest)).Should(Succeed())
			spaceRequest.Finalizers = []string{}
			Expect(k8sClient.Update(ctx, spaceRequest)).To(Succeed())

			By("this should trigger both the DT and SpaceRequest to be deleted")

			Eventually(dt, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
			Eventually(spaceRequest, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

		})
	})
})
