package shared_resource_loop

import (
	"context"
	"fmt"
	"reflect"
	"strings"

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

	Context("Test isSSHUrl function", func() {

		DescribeTable("Test scenarios for normalizeGitURL", func(repoUrl string, expected bool) {
			isSSH, _ := isSSHURL(repoUrl)
			Expect(expected).To(Equal(isSSH))
		},
			Entry("Url1", "git://github.com/redhat-appstudio/test.git", false),
			Entry("Url2", "git@GITHUB.com:redhat-appstudio/test.git", true),
			Entry("Url3", "git@github.com:test", true),
			Entry("Url4", "git@github.com:test.git", true),
			Entry("Url5", "https://github.com/redhat-appstudio/test", false),
			Entry("Url6", "https://github.com/redhat-appstudio/test.git", false),
			Entry("Url7", "ssh://git@GITHUB.com:redhat-appstudio/test", true),
			Entry("Url8", "ssh://git@GITHUB.com:redhat-appstudio/test.git", true),
			Entry("Url9", "ssh://git@github.com:test.git", true),
		)
	})

	Context("Test normalizeGitURL function", func() {

		DescribeTable("Test scenarios for normalizeGitURL", func(repoUrl, normalizedRepoUrl string) {

			Expect(normalizeGitURL(repoUrl)).To(Equal(normalizedRepoUrl))
		},
			Entry("Https Url", "https://github.com/redhat-appstudio/test.git", "https://github.com/redhat-appstudio/test"),
			Entry("Git Url", "git@github.com:redhat-appstudio/managed-gitops.git", "git@github.com/redhat-appstudio/managed-gitops"),
			Entry("Invalid Url", "https://@github.com:test:git", ""),
		)
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

		var haveErrOccurredConditionSet = func(expectedRepoCredStatus managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus, checkLastTransitionTime bool) matcher.GomegaMatcher {

			// sanitizeCondition removes ephemeral fields from the GitOpsDeploymentCondition which should not be compared using
			// reflect.DeepEqual
			sanitizeCondition := func(cond metav1.Condition) metav1.Condition {

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
							if checkLastTransitionTime {
								if resourceCondition.LastTransitionTime.Equal(&existingCondition.LastTransitionTime) {
									continue
								}
							}
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
				Conditions: []metav1.Condition{
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
				Conditions: []metav1.Condition{
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

			Expect(gitopsDeploymentRepositoryCredentialCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedRepoCredStatus, false)))
		})

		It("shouldn't update an existing condition if it hasn't changed", func() {
			gitopsDeploymentRepositoryCredentialCR.Spec.Secret = "test"

			transitionTime := metav1.Now()

			expectedRepoCredStatus := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus{
				Conditions: []metav1.Condition{
					{
						Type:               managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
						Reason:             managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:             metav1.ConditionTrue,
						Message:            "Repository does not exist: repository not found",
						LastTransitionTime: transitionTime,
					}, {
						Type:               managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
						Reason:             managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:             metav1.ConditionFalse,
						Message:            "Repository does not exist: repository not found",
						LastTransitionTime: transitionTime,
					}, {
						Type:               managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
						Reason:             managedgitopsv1alpha1.RepositoryCredentialReasonInValidRepositoryUrl,
						Status:             metav1.ConditionFalse,
						Message:            "Repository does not exist: repository not found",
						LastTransitionTime: transitionTime,
					},
				},
			}

			gitopsDeploymentRepositoryCredentialCR.Status = expectedRepoCredStatus
			Expect(k8sClient.Status().Update(ctx, gitopsDeploymentRepositoryCredentialCR)).To(BeNil())

			err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, k8sClient, &corev1.Secret{}, log.FromContext(ctx))
			Expect(err).To(BeNil())

			Expect(gitopsDeploymentRepositoryCredentialCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedRepoCredStatus, true)))
		})

		It("should set error conditions if secret was not defined in RepositoryCredentials", func() {

			expectedRepoCredStatus := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus{
				Conditions: []metav1.Condition{
					{
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotSpecified,
						Status:  metav1.ConditionTrue,
						Message: "Secret field is missing value",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotSpecified,
						Status:  metav1.ConditionFalse,
						Message: "Secret field is missing value",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotSpecified,
						Status:  metav1.ConditionFalse,
						Message: "Secret field is missing value",
					},
				},
			}

			err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, k8sClient, nil, log.FromContext(ctx))
			Expect(err).To(BeNil())

			Expect(gitopsDeploymentRepositoryCredentialCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedRepoCredStatus, false)))
		})

		It("should set error conditions if secret specified in RepositoryCredentials was not found", func() {
			gitopsDeploymentRepositoryCredentialCR.Spec.Secret = "test"

			expectedRepoCredStatus := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialStatus{
				Conditions: []metav1.Condition{
					{
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotFound,
						Status:  metav1.ConditionTrue,
						Message: "Secret specified not found",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotFound,
						Status:  metav1.ConditionFalse,
						Message: "Secret specified not found",
					}, {
						Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
						Reason:  managedgitopsv1alpha1.RepositoryCredentialReasonSecretNotFound,
						Status:  metav1.ConditionFalse,
						Message: "Secret specified not found",
					},
				},
			}

			err := UpdateGitopsDeploymentRepositoryCredentialStatus(ctx, gitopsDeploymentRepositoryCredentialCR, k8sClient, nil, log.FromContext(ctx))
			Expect(err).To(BeNil())

			Expect(gitopsDeploymentRepositoryCredentialCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedRepoCredStatus, false)))
		})
	})

	Context("Test validateRepositoryCredentials", func() {

		DescribeTable("Test scenarios for validateRepositoryCredentials", func(repoUrl string, secret *corev1.Secret, expectedString string) {

			err := validateRepositoryCredentials(repoUrl, secret)

			Expect(err).NotTo(BeNil())
			Expect(strings.Contains(err.Error(), expectedString)).To(BeTrue())

		},
			Entry("Test for Invalid Url", "git@github.com:redhat-appstudio/test.git", &corev1.Secret{Data: map[string][]byte{"username": []byte("username"), "password": []byte("password")}}, "not found"),
			Entry("Test for Valid Url and Invalid Secret", "git@github.com:redhat-appstudio/managed-gitops.git", &corev1.Secret{Data: map[string][]byte{"username": []byte("username"), "password": []byte("password")}}, "not found"),
		)
	})
})
