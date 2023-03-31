package shared_resource_loop

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	matcher "github.com/onsi/gomega/types"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/gomega"
)

var _ = Describe("SharedResourceEventLoop Repository Credential Tests", func() {

	Context("Shared Resource Loop Repository Credential test", func() {

		It("Test IsSSHURL function", func() {
			data := map[string]bool{
				"git://github.com/redhat-appstudio/test.git":     false,
				"git@GITHUB.com:redhat-appstudio/test.git":       true,
				"git@github.com:test":                            true,
				"git@github.com:test.git":                        true,
				"https://github.com/redhat-appstudio/test":       false,
				"https://github.com/redhat-appstudio/test.git":   false,
				"ssh://git@GITHUB.com:redhat-appstudio/test":     true,
				"ssh://git@GITHUB.com:redhat-appstudio/test.git": true,
				"ssh://git@github.com:test.git":                  true,
			}
			for k, v := range data {
				isSSH, _ := IsSSHURL(k)
				Expect(v).To(Equal(isSSH))
			}

		})

		It("Test NormalizeUrl function", func() {
			testData := []struct {
				repoUrl           string
				normalizedRepoUrl string
			}{
				{
					repoUrl: "https://github.com/redhat-appstudio/test.git", normalizedRepoUrl: "https://github.com/redhat-appstudio/test",
				},
			}

			for _, data := range testData {
				Expect(NormalizeGitURL(data.repoUrl)).To(Equal(data.normalizedRepoUrl))
			}
		})
	})

	Context("Set GitOpsDeploymentRepositoryCredentials status conditions", func() {

		var (
			ctx                                    context.Context
			k8sClient                              client.Client
			gitopsDeploymentRepositoryCredentialCR *managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential
		)

		BeforeEach(func() {
			scheme, _, _, workspace, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			ctx = context.Background()

			gitopsDeploymentRepositoryCredentialCR = &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-repocred",
					Namespace: workspace.Name,
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDeploymentRepositoryCredentialCR).Build()

		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, gitopsDeploymentRepositoryCredentialCR)
			Expect(err).To(BeNil())
		})

		var haveErrOccurredConditionSet = func(expectedRepoCredStatus managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus) matcher.GomegaMatcher {

			// sanitizeCondition removes ephemeral fields from the GitOpsDeploymentCondition which should not be compared using
			// reflect.DeepEqual
			sanitizeCondition := func(cond *metav1.Condition) metav1.Condition {

				res := metav1.Condition{
					Type:   cond.Type,
					Status: cond.Status,
					Reason: cond.Reason,
				}

				return res

			}
			return WithTransform(func(repoCred *managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential) bool {

				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(repoCred), repoCred); err != nil {
					GinkgoWriter.Println(err)
					return false
				}

				conditionExists := false
				existingConditionList := repoCred.Status.Conditions

				if len(expectedRepoCredStatus.Conditions) != len(existingConditionList) {
					fmt.Println("HaveConditions:", conditionExists, "/ Expected:", expectedRepoCredStatus.Conditions, "/ Actual:", repoCred.Status.Conditions)
					return false
				}

				for _, resourceCondition := range expectedRepoCredStatus.Conditions {
					conditionExists = false
					for _, existingCondition := range existingConditionList {
						if reflect.DeepEqual(sanitizeCondition(resourceCondition), sanitizeCondition(existingCondition)) {
							conditionExists = true
							break
						}
					}
					if !conditionExists {
						fmt.Println("GitOpsDeploymentCondition:", conditionExists, "/ Expected:", expectedRepoCredStatus.Conditions, "/ Actual:", repoCred.Status.Conditions)
						break
					}
				}
				return conditionExists

			}, BeTrue())
		}

		It("should update an existing condition if it has changed", func() {
			gitopsDeploymentRepositoryCredentialCR.Spec.Secret = "test"
			repoCredStatus := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus{
				Conditions: []*metav1.Condition{
					{
						Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
						Reason: managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotSpecified,
						Status: metav1.ConditionTrue,
					}, {
						Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
						Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status: metav1.ConditionFalse,
					}, {
						Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
						Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status: metav1.ConditionFalse,
					},
				},
			}

			gitopsDeploymentRepositoryCredentialCR.Status = repoCredStatus
			Expect(k8sClient.Status().Update(ctx, gitopsDeploymentRepositoryCredentialCR)).To(BeNil())

			expectedRepoCredStatus := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus{
				Conditions: []*metav1.Condition{
					{
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:  metav1.ConditionTrue,
						Message: "repository not found",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:  metav1.ConditionFalse,
						Message: "repository not found",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:  metav1.ConditionFalse,
						Message: "repository not found",
					},
				},
			}

			err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, k8sClient, &corev1.Secret{}, log.FromContext(ctx))
			Expect(err).To(BeNil())

			Expect(gitopsDeploymentRepositoryCredentialCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedRepoCredStatus)))
		})

		It("shouldn't update an existing condition if it hasn't changed", func() {
			gitopsDeploymentRepositoryCredentialCR.Spec.Secret = "test"
			expectedRepoCredStatus := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus{
				Conditions: []*metav1.Condition{
					{
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:  metav1.ConditionTrue,
						Message: "repository not found",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:  metav1.ConditionFalse,
						Message: "repository not found",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:  metav1.ConditionFalse,
						Message: "repository not found",
					},
				},
			}

			gitopsDeploymentRepositoryCredentialCR.Status = expectedRepoCredStatus
			Expect(k8sClient.Status().Update(ctx, gitopsDeploymentRepositoryCredentialCR)).To(BeNil())

			err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, k8sClient, &corev1.Secret{}, log.FromContext(ctx))
			Expect(err).To(BeNil())

			Expect(gitopsDeploymentRepositoryCredentialCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedRepoCredStatus)))
		})
	})
})
