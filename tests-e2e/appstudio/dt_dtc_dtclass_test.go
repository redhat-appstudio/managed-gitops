package appstudio

import (
	"context"
	"fmt"
	"strings"

	apierr "k8s.io/apimachinery/pkg/api/errors"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"

	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	dtfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttarget"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/spacerequest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests verify that DeploymentTarget/DeploymentTargetClaim/DeploymentTargetClass/Environment all conform
// to the behaviour I would expect from them based on ADR-8  (jgwest).
//
// # The step numbers correspond to steps in the document
//
// See the full document here:
// https://hackmd.io/@_Ujs_9QpQgyQYgLo2maa2Q/ByXcz-20o
var _ = Describe("DeploymentTarget DeploymentTargetClaim and Class tests", func() {

	Context("Creation and deletion behaviour", func() {
		var (
			k8sClient client.Client
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())
		})

		createTest := func(classReclaimPolicy appstudiosharedv1.ReclaimPolicy) (appstudiosharedv1.DeploymentTargetClaim, appstudiosharedv1.DeploymentTarget, codereadytoolchainv1alpha1.SpaceRequest) {
			By("Step 0 - creating a DeploymentTargetClass based on devsandbox provisoner")
			dtclass := &appstudiosharedv1.DeploymentTargetClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-isolation-level-namespace",
				},
				Spec: appstudiosharedv1.DeploymentTargetClassSpec{
					Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
					Parameters:    appstudiosharedv1.DeploymentTargetParameters{},
					ReclaimPolicy: classReclaimPolicy,
				},
			}
			err := k8s.Create(dtclass, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("Step 1 - Create a DeploymentTargetClaim that references the class")
			dtc := appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging-dtc",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName(dtclass.Name),
				},
			}
			err = k8s.Create(&dtc, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("Step 2- Create an Environment that references the DTC")
			env := &appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					DisplayName:        "my environment",
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					Configuration: appstudiosharedv1.EnvironmentConfiguration{
						Env: []appstudiosharedv1.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
						Target: appstudiosharedv1.EnvironmentTarget{
							DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8s.Create(env, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("Step 3 - ensure the DTC has a phase .status.phase of Pending, and has the correct target provisioner annotation")
			Eventually(dtc, "60s", "1s").Should(dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			Eventually(dtc, "2m", "1s").Should(
				dtcfixture.HasAnnotation(appstudiosharedv1.AnnTargetProvisioner,
					string(dtc.Spec.DeploymentTargetClassName)),
			)

			By("Step 4 - ensure that a SpaceRequest was created")

			expectedSpaceRequest := codereadytoolchainv1alpha1.SpaceRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dtc.Name,
					Namespace: dtc.Namespace,
				},
				Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{},
			}

			Eventually(func() bool {

				spaceRequestList := codereadytoolchainv1alpha1.SpaceRequestList{}
				err = k8s.List(&spaceRequestList, dtc.Namespace, k8sClient)
				if err != nil {
					fmt.Println("Error occurred in list", err)
					return false
				}
				if len(spaceRequestList.Items) > 0 {
					fmt.Println()
				}

				for _, spaceRequest := range spaceRequestList.Items {

					if strings.HasPrefix(spaceRequest.Name, dtc.Name) {
						fmt.Println("- SpaceRequest match found:", spaceRequest.Name)
						expectedSpaceRequest = *spaceRequest.DeepCopy()
						return true
					} else {
						fmt.Println("- SpaceRequest does not match:", spaceRequest.Name)
					}
				}

				return false
			}, "60s", "1s").Should(BeTrue())

			By("Step 5 - Create a fake space request credentials secret, and update the SpaceRequest to point to that Secret, with status of Ready")

			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "space-request-secret",
					Namespace: dtc.Namespace,
				},
				StringData: map[string]string{
					"some-data": k8s.GenerateFakeKubeConfig(),
				},
			}
			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = spacerequest.UpdateStatusWithFunction(&expectedSpaceRequest, func(spaceRequestParam *codereadytoolchainv1alpha1.SpaceRequestStatus) {

				*spaceRequestParam = codereadytoolchainv1alpha1.SpaceRequestStatus{
					TargetClusterURL: "https://fake-client-api",
					NamespaceAccess: []codereadytoolchainv1alpha1.NamespaceAccess{
						{
							Name:      "new-namespace",
							SecretRef: secret.Name,
						},
					},
					Conditions: []codereadytoolchainv1alpha1.Condition{
						{
							Type:   codereadytoolchainv1alpha1.ConditionReady,
							Status: corev1.ConditionTrue,
						},
					},
				}
			})
			Expect(err).ToNot(HaveOccurred())

			By("Step 6 - Ensure there exists a DeploymentTarget based on the DTC")

			var matchingDT appstudiosharedv1.DeploymentTarget

			Eventually(func() bool {

				deploymentTargetList := appstudiosharedv1.DeploymentTargetList{}
				err = k8s.List(&deploymentTargetList, dtc.Namespace, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				for _, dt := range deploymentTargetList.Items {
					if strings.HasPrefix(dt.Name, dtc.Name+"-dt") {
						matchingDT = *dt.DeepCopy()
					} else {
						fmt.Println("- DeploymentTarget name does not match:", dt.Name)
					}
				}

				return matchingDT.Name != ""

			}, "60s", "1s").Should(BeTrue())

			Expect(&matchingDT).To(k8s.HasAnnotation("provisioner.appstudio.redhat.com/provisioned-by", "appstudio.redhat.com/devsandbox", k8sClient))

			Expect(string(matchingDT.Spec.DeploymentTargetClassName)).Should(Equal(dtclass.Name))

			Expect(matchingDT.Spec.KubernetesClusterCredentials.DefaultNamespace).
				Should(Equal(expectedSpaceRequest.Status.NamespaceAccess[0].Name))
			Expect(matchingDT.Spec.KubernetesClusterCredentials.APIURL).
				Should(Equal(expectedSpaceRequest.Status.TargetClusterURL))
			Expect(matchingDT.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret).
				Should(Equal(expectedSpaceRequest.Status.NamespaceAccess[0].SecretRef))

			By("Step 7 - DT should be bound")

			Eventually(matchingDT, "60s", "1s").Should(dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Bound))

			Eventually(&matchingDT).Should(k8s.HasFinalizers([]string{appstudiocontrollers.FinalizerDT}, k8sClient))

			By("Step 8 - DTC should be bound and condition should be true")

			Eventually(dtc, "60s", "1s").Should(dtcfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

			Expect(&dtc).Should(k8s.HasAnnotation(appstudiosharedv1.AnnBindCompleted, appstudiosharedv1.AnnBinderValueTrue, k8sClient))
			Expect(&dtc).Should(k8s.HasAnnotation(appstudiosharedv1.AnnBoundByController, appstudiosharedv1.AnnBinderValueTrue, k8sClient))
			Eventually(dtc, "3m", "1s").Should(dtcfixture.HaveDeploymentTargetClaimCondition(
				metav1.Condition{
					Type:    appstudiocontrollers.DeploymentTargetClaimConditionTypeErrorOccurred,
					Message: "",
					Status:  metav1.ConditionTrue,
					Reason:  appstudiocontrollers.DeploymentTargetClaimReasonBound,
				}))

			By("Step 9 - Ensure a managed environment exists and that it is owned by the environment")

			Eventually(func() bool {

				var managedEnvironmentList managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentList
				err = k8s.List(&managedEnvironmentList, dtc.Namespace, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				for _, item := range managedEnvironmentList.Items {
					matchFound := false

					for _, ownerRef := range item.OwnerReferences {

						if ownerRef.Name == env.Name {
							matchFound = true
						}
					}

					managedEnvSecretName := "managed-environment-secret-" + env.Name
					if matchFound {
						// ManagedEnvironment secret should match the secret created by the Environment controller.
						if item.Spec.ClusterCredentialsSecret == managedEnvSecretName {
							return true
						}
					}
				}

				return false
			}, "60s", "1s").Should(BeTrue())

			return dtc, matchingDT, expectedSpaceRequest
		}

		It("should ensure the dynamic provisioning happy path works as expected", func() {
			_, _, _ = createTest(appstudiosharedv1.ReclaimPolicy_Delete)
		})

		It("should ensure that deleting a provisioner with Delete policy will delete the SpaceRequest, DT, and DTC.", func() {

			dtc, matchingDT, expectedSpaceRequest := createTest(appstudiosharedv1.ReclaimPolicy_Delete)

			By("Deletion step 1 - Delete the DeploymentTargetClaim")
			err := k8sClient.Delete(context.Background(), &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("Deletion step 2 - Ensure the matching DT is also starting to delete")
			Eventually(&matchingDT, "60s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))

			By("Deletion step 3 - Ensure the matching SpaceRequest is also starting to delete")
			Eventually(&expectedSpaceRequest, "60s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))

			By("Deletion step 4 - Update the fake Space Request to Terminating")

			err = spacerequest.UpdateStatusWithFunction(&expectedSpaceRequest, func(spaceRequestParam *codereadytoolchainv1alpha1.SpaceRequestStatus) {

				spaceRequestParam.Conditions = []codereadytoolchainv1alpha1.Condition{
					{
						Type:   codereadytoolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
						Reason: codereadytoolchainv1alpha1.SpaceTerminatingReason,
					},
				}
			})
			Expect(err).ToNot(HaveOccurred())

			Consistently(&expectedSpaceRequest, "10s", "1s").Should(k8s.ExistByName(k8sClient),
				"the space request should not be deleted while it is terminating")
			Consistently(&matchingDT, "10s", "1s").Should(k8s.ExistByName(k8sClient), "the DT should not be deleted while it is terminating")

			By("removing the finalizer from the SpaceRequest, which should cause it to be deleted.")
			expectedSpaceRequest.Finalizers = []string{}
			err = k8s.Update(&expectedSpaceRequest, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(&expectedSpaceRequest, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

			By("deletion step 5 - no-op")

			By("deletion step 6 - the DT and DTC should also be deleted, once the SpaceRequest is removed")

			Eventually(&matchingDT, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
			Eventually(&dtc, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

		})

		It("should ensure that if the DTClass has a policy of retain, then when the DTC is deleted that neither the DT nor SpaceRequest are deleted", func() {
			dtc, matchingDT, expectedSpaceRequest := createTest(appstudiosharedv1.ReclaimPolicy_Retain)

			By("Deletion step 1 - Delete the DeploymentTargetClaim")
			err := k8sClient.Delete(context.Background(), &dtc)
			Expect(err).ToNot(HaveOccurred())

			Eventually(&dtc, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

			By("Deletion step 2 - Ensure the matching DT is not starting to delete")
			Consistently(&matchingDT, "30s", "1s").Should(k8s.ExistByName(k8sClient))
			Consistently(&matchingDT, "30s", "1s").ShouldNot(k8s.HasNonNilDeletionTimestamp(k8sClient))

			By("Deletion step 3 - Ensure the matching SpaceRequest is also not starting to delete")
			Consistently(&expectedSpaceRequest, "20s", "1s").Should(k8s.ExistByName(k8sClient))
			Consistently(&expectedSpaceRequest, "20s", "1s").ShouldNot(k8s.HasNonNilDeletionTimestamp(k8sClient))

			Eventually(matchingDT, "30s", "1s").Should(dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Released))

			By("Ensure the claimRef is unset from the DT")
			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&matchingDT), &matchingDT)
			Expect(err).ToNot(HaveOccurred())
			Expect(matchingDT.Spec.ClaimRef).Should(BeEmpty())
		})

		It("should ensure that if the SpaceRequest fails to delete, DT is set to failed, and that if the SpaceRequest is eventually deleted, so will the DT and DTC", func() {
			dtc, matchingDT, expectedSpaceRequest := createTest(appstudiosharedv1.ReclaimPolicy_Delete)

			By("Deletion step 1 - Delete the DeploymentTargetClaim")
			err := k8sClient.Delete(context.Background(), &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("Deletion step 2 - Ensure the matching DT is also starting to delete")
			Eventually(&matchingDT, "60s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))

			By("Deletion step 3 - Ensure the matching SpaceRequest is also starting to delete")
			Eventually(&expectedSpaceRequest, "60s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))

			By("Deletion step 4 - Update the fake Space Request to Terminating")

			err = spacerequest.UpdateStatusWithFunction(&expectedSpaceRequest, func(spaceRequestParam *codereadytoolchainv1alpha1.SpaceRequestStatus) {

				spaceRequestParam.Conditions = []codereadytoolchainv1alpha1.Condition{
					{
						Type:   codereadytoolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
						Reason: codereadytoolchainv1alpha1.SpaceTerminatingReason,
					},
				}
			})
			Expect(err).ToNot(HaveOccurred())

			Consistently(&expectedSpaceRequest, "10s", "1s").Should(k8s.ExistByName(k8sClient),
				"the space request should not be deleted while it is terminating")
			Consistently(&matchingDT, "10s", "1s").Should(k8s.ExistByName(k8sClient), "the DT should not be deleted while it is terminating")

			err = spacerequest.UpdateStatusWithFunction(&expectedSpaceRequest, func(spaceRequestParam *codereadytoolchainv1alpha1.SpaceRequestStatus) {

				spaceRequestParam.Conditions = []codereadytoolchainv1alpha1.Condition{
					{
						Type:   codereadytoolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
						Reason: codereadytoolchainv1alpha1.SpaceTerminatingFailedReason,
					},
				}
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(matchingDT, "3m", "1s").Should(dtfixture.HasStatusPhase(appstudiosharedv1.DeploymentTargetPhase_Failed))

			By("We now simulate the SpaceRequest finally cleaning itself up, which should cause the DT and DTC to be deleted")

			By("removing the finalizer from the SpaceRequest, which should cause it to be deleted.")
			expectedSpaceRequest.Finalizers = []string{}
			err = k8s.Update(&expectedSpaceRequest, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(&expectedSpaceRequest, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

			By("deletion step 5 - no-op")

			By("deletion step 6 - the DT and DTC should also be deleted, once the SpaceRequest is removed")

			Eventually(&matchingDT, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
			Eventually(&dtc, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

		})

		It("ensures that when deleting a DT with DeploymentTargetClass of Delete, that the DTC is not deleted", func() {
			dtc, matchingDT, _ := createTest(appstudiosharedv1.ReclaimPolicy_Delete)

			Eventually(func() bool {

				// Remove the finalizer from the DeploymentTarget
				err := k8s.Get(&matchingDT, k8sClient)
				if err != nil {
					fmt.Println(err)
					return false
				}
				matchingDT.Finalizers = []string{}
				err = k8s.Update(&matchingDT, k8sClient)
				if err != nil {
					fmt.Println(err)
					return false
				}

				// Delete the DeploymentTarget
				err = k8s.Delete(&matchingDT, k8sClient)
				if err != nil {
					fmt.Println(err)
					return false
				}

				// The DeploymentTarget should be deleted
				err = k8s.Get(&matchingDT, k8sClient)
				return err != nil && apierr.IsNotFound(err) == true
			}, "60s", "1s").Should(BeTrue())

			// After the DeploymentTarget is deleted, the DeploymentTargetClaim should continue to exist
			Consistently(&dtc, "30s", "1s").Should(k8s.ExistByName(k8sClient))
		})
	})

	Context("Check for status conditions", func() {
		It("should have expected conditions.", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("Create a DeploymentTargetClaim which is going to fail.")

			dtc := appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging-dtc",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName("abc"),
				},
			}

			err = k8s.Create(&dtc, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("DeploymentTargetClaim status condition should have expected condition.")

			Eventually(dtc, "3m", "1s").Should(dtcfixture.HaveDeploymentTargetClaimCondition(
				metav1.Condition{
					Type:    appstudiocontrollers.DeploymentTargetClaimConditionTypeErrorOccurred,
					Message: "unable to handle provisioning of space request for dynamic DTC",
					Status:  metav1.ConditionFalse,
					Reason:  appstudiocontrollers.DeploymentTargetClaimReasonErrorOccurred,
				}))
		})
	})
})
