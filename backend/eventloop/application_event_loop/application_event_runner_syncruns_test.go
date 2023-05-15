package application_event_loop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Application Event Runner SyncRuns", func() {

	Context("Handle GitOpsDeploymentSyncRun", func() {

		var (
			dbQueries         db.AllDatabaseQueries
			k8sClient         *sharedutil.ProxyClient
			gitopsDepl        *managedgitopsv1alpha1.GitOpsDeployment
			applicationAction applicationEventLoopRunner_Action
			informer          sharedutil.ListEventReceiver
			gitopsDeplSyncRun *managedgitopsv1alpha1.GitOpsDeploymentSyncRun
		)
		ctx := context.Background()

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl = &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual,
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path: "resources/test-data/sample-gitops-repository/environments/overlays/dev",
					},
				},
			}

			gitopsDeplSyncRun = &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitops-syncrun",
					Namespace: gitopsDepl.Namespace,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDepl.Name,
					RevisionID:           "HEAD",
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, argocdNamespace, kubesystemNamespace, gitopsDepl, gitopsDeplSyncRun).Build()

			informer = sharedutil.ListEventReceiver{}
			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, false)
			Expect(err).To(BeNil())

			applicationAction = applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceClient:             k8sClient,
				log:                         log.FromContext(ctx),
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			_, _, _, _, userDevErr := applicationAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			applicationAction.eventResourceName = "gitops-syncrun"
			userDevErr = applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
		})

		It("should handle a valid GitOpsDeploymentSyncRun by creating a SyncRun Operation", func() {
			userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			By("check if the SyncOperation entry is created in the DB")
			mapping := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceName:      gitopsDeplSyncRun.Name,
				APIResourceNamespace: gitopsDeplSyncRun.Namespace,
				APIResourceUID:       string(gitopsDeplSyncRun.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
			}
			err := dbQueries.GetDatabaseMappingForAPICR(ctx, &mapping)
			Expect(err).To(BeNil())

			syncOperation := db.SyncOperation{SyncOperation_id: mapping.DBRelationKey}
			err = dbQueries.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())
			Expect(syncOperation.DeploymentNameField).Should(Equal(gitopsDeplSyncRun.Spec.GitopsDeploymentName))
			Expect(syncOperation.Revision).Should(Equal(gitopsDeplSyncRun.Spec.RevisionID))

			By("verify if an Operation CR is created")
			operationCreated, operationDeleted := false, false
			for _, event := range informer.Events {
				if event.Action == sharedutil.Create && event.ObjectTypeOf() == "Operation" {
					operationCreated = true
				}
				if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "Operation" {
					operationDeleted = true
				}
			}
			Expect(operationCreated).To(BeTrue())
			Expect(operationDeleted).To(BeTrue())
		})

		It("should throw an error when users try to update immutable fields", func() {
			By("create a new GitOpsDeployment that needs to be synced")
			newGitOpsDepl := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-gitopsdepl",
					Namespace: gitopsDeplSyncRun.Namespace,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual,
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path: "resources/test-data/sample-gitops-repository/environments/overlays/dev",
					},
				},
			}

			err := k8sClient.Create(ctx, &newGitOpsDepl)
			Expect(err).To(BeNil())

			newAppAction := applicationAction
			newAppAction.eventResourceName = newGitOpsDepl.Name
			_, _, _, _, userDevErr := newAppAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			By("verify if the field .spec.GitOpsDeploymentName of GitOpsDeploymentSyncRun is immutable")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDeplSyncRun), gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			gitopsDeplSyncRun.Spec.GitopsDeploymentName = newGitOpsDepl.Name
			err = k8sClient.Update(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())
			userDevErr = applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(ErrDeploymentNameIsImmutable))
			Expect(userDevErr.UserError()).Should(Equal(ErrDeploymentNameIsImmutable))

			gitopsDeplSyncRun.Spec.GitopsDeploymentName = gitopsDepl.Name
			err = k8sClient.Update(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			By("verify if the field .spec.revisionID of GitOpsDeploymentSyncRun is immutable")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDeplSyncRun), gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			gitopsDeplSyncRun.Spec.RevisionID = "main"
			err = k8sClient.Update(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())
			userDevErr = applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(ErrRevisionIsImmutable))
			Expect(userDevErr.UserError()).Should(Equal(ErrRevisionIsImmutable))
		})

		It("should terminate the SyncOperation and create an Operation when the SyncRun CR is deleted", func() {
			mapping := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceName:      gitopsDeplSyncRun.Name,
				APIResourceNamespace: gitopsDeplSyncRun.Namespace,
				APIResourceUID:       string(gitopsDeplSyncRun.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
			}
			err := dbQueries.GetDatabaseMappingForAPICR(ctx, &mapping)
			Expect(err).To(BeNil())

			syncOperation := db.SyncOperation{SyncOperation_id: mapping.DBRelationKey}
			err = dbQueries.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())

			By("delete the GitOpsDeploymentSyncRun and check if the SyncOperation is deleted")
			err = k8sClient.Delete(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			By("check if the application event runner goroutine can be shutdown")
			userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			By("check if the sync operation row is deleted")
			err = dbQueries.GetSyncOperationById(ctx, &syncOperation)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			By("check if a new Operation is created to handle sync termination")
			operationCreated, operationDeleted := false, false
			for _, event := range informer.Events {
				if event.Action == sharedutil.Create && event.ObjectTypeOf() == "Operation" {
					operationCreated = true
				}
				if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "Operation" {
					operationDeleted = true
				}
			}
			Expect(operationDeleted).To(BeTrue())
			Expect(operationCreated).To(BeTrue())

			By("check if the APICRToDatabaseMapping row is deleted")
			err = dbQueries.GetDatabaseMappingForAPICR(ctx, &mapping)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())
		})

		It("should throw an error when a SyncRun points to a GitOpsDeployment that doesn't exist", func() {
			invalidGitOpsDeplSyncRun := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-gitops-syncrun",
					Namespace: gitopsDepl.Namespace,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: "unknown-gitops-depl",
				},
			}

			err := k8sClient.Create(ctx, &invalidGitOpsDeplSyncRun)
			Expect(err).To(BeNil())

			expectedErr := "unable to retrieve gitopsdeployment referenced in syncrun: gitopsdeployments.managed-gitops.redhat.com \"unknown-gitops-depl\" not found"

			applicationAction.eventResourceName = "invalid-gitops-syncrun"
			userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(expectedErr))
		})

		It("should return an error for a GitOpsDeployment with Automated sync policy", func() {

			By("create a GitOpsDeployment with Automated sync policy")
			gitopsDeplAutomated := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-depl-automated",
					Namespace: gitopsDepl.Namespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			err := k8sClient.Create(ctx, &gitopsDeplAutomated)
			Expect(err).To(BeNil())

			By("create a SyncRun CR pointing to the above GitOpsDeployment")
			syncRun := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "syncrun",
					Namespace: gitopsDeplAutomated.Namespace,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDeplAutomated.Name,
				},
			}

			err = k8sClient.Create(ctx, &syncRun)
			Expect(err).To(BeNil())

			By("check if an error is returned")
			expectedErr := fmt.Sprintf("invalid GitOpsDeploymentSyncRun '%s'. Syncing a GitOpsDeployment with Automated sync policy is not allowed", syncRun.Name)

			applicationAction.eventResourceName = "syncrun"
			userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(expectedErr))
			Expect(userDevErr.UserError()).Should(Equal(expectedErr))
		})

		It("should return true shutdown signal if neither CR nor DB entry exists", func() {
			By("delete the SyncRun CR and the relevant DB details")
			err := k8sClient.Delete(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			By("check if the shutdown signal is true")
			userDevErr = applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
		})

		It("should throw an error if the namespace doesn't exist", func() {
			applicationAction.eventResourceNamespace = "unknown-namespace"
			userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)

			expectedDevError := "unable to retrieve namespace 'unknown-namespace': namespaces \"unknown-namespace\" not found"
			expectedUserError := "unable to retrieve the contents of the namespace 'unknown-namespace' containing the API resource 'gitops-syncrun'. Does it exist?"

			Expect(userDevErr.DevError().Error()).Should(Equal(expectedDevError))
			Expect(userDevErr.UserError()).Should(Equal(expectedUserError))
		})
	})

	Context("Set GitOpsDeploymentSyncRun conditions", func() {

		var (
			ctx           context.Context
			k8sClient     client.Client
			syncRunCR     *managedgitopsv1alpha1.GitOpsDeploymentSyncRun
			conditionType managedgitopsv1alpha1.GitOpsDeploymentConditionType
		)

		BeforeEach(func() {
			scheme, _, _, workspace, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			ctx = context.Background()

			syncRunCR = &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-syncrun",
					Namespace: workspace.Name,
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(syncRunCR).Build()

			conditionType = managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred
		})

		var haveErrOccurredConditionSet = func(expectedSyncRunStatus managedgitopsv1alpha1.GitOpsDeploymentSyncRunStatus) matcher.GomegaMatcher {

			return WithTransform(func(syncRun *managedgitopsv1alpha1.GitOpsDeploymentSyncRun) bool {

				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(syncRun), syncRun); err != nil {
					GinkgoWriter.Println(err)
					return false
				}

				if len(expectedSyncRunStatus.Conditions) != len(syncRun.Status.Conditions) {
					return false
				}

				for i := 0; i < len(syncRun.Status.Conditions); i++ {
					condition := syncRun.Status.Conditions[i]
					expectedCondition := expectedSyncRunStatus.Conditions[i]

					res := condition.Message == expectedCondition.Message &&
						condition.Reason == expectedCondition.Reason &&
						condition.Status == expectedCondition.Status &&
						condition.Type == expectedCondition.Type

					if !res {
						GinkgoWriter.Println(condition, expectedCondition)
						return false
					}
				}

				return true

			}, BeTrue())
		}

		It("should set a new condition if it is absent", func() {
			expectedSyncRunStatus := managedgitopsv1alpha1.GitOpsDeploymentSyncRunStatus{
				Conditions: []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
					{
						Type:   managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred,
						Reason: managedgitopsv1alpha1.SyncRunReasonType(""),
						Status: managedgitopsv1alpha1.GitOpsConditionStatusFalse,
					},
				},
			}

			err := setGitOpsDeploymentSyncRunCondition(ctx, k8sClient, syncRunCR, managedgitopsv1alpha1.SyncRunConditionType(conditionType), managedgitopsv1alpha1.SyncRunReasonType(""), managedgitopsv1alpha1.GitOpsConditionStatusFalse, "")
			Expect(err).To(BeNil())

			Expect(syncRunCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedSyncRunStatus)))
		})

		It("should update an existing condition if it has changed", func() {
			syncRunStatus := managedgitopsv1alpha1.GitOpsDeploymentSyncRunStatus{
				Conditions: []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
					{
						Type:   managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred,
						Reason: managedgitopsv1alpha1.SyncRunReasonType(""),
						Status: managedgitopsv1alpha1.GitOpsConditionStatusFalse,
					},
				},
			}

			syncRunCR.Status = syncRunStatus
			Expect(k8sClient.Status().Update(ctx, syncRunCR)).To(BeNil())

			expectedSyncRunStatus := managedgitopsv1alpha1.GitOpsDeploymentSyncRunStatus{
				Conditions: []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
					{
						Type:    managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred,
						Reason:  managedgitopsv1alpha1.SyncRunReasonType(conditionType),
						Status:  managedgitopsv1alpha1.GitOpsConditionStatusTrue,
						Message: "error occured due to xyz",
					},
				},
			}

			err := setGitOpsDeploymentSyncRunCondition(ctx, k8sClient, syncRunCR, managedgitopsv1alpha1.SyncRunConditionType(conditionType), managedgitopsv1alpha1.SyncRunReasonType(conditionType), managedgitopsv1alpha1.GitOpsConditionStatusTrue, expectedSyncRunStatus.Conditions[0].Message)
			Expect(err).To(BeNil())

			Expect(syncRunCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedSyncRunStatus)))
		})

		It("shouldn't update an existing condition if it hasn't changed", func() {

			expectedSyncRunStatus := managedgitopsv1alpha1.GitOpsDeploymentSyncRunStatus{
				Conditions: []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
					{
						Type:    managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred,
						Reason:  managedgitopsv1alpha1.SyncRunReasonType(conditionType),
						Status:  managedgitopsv1alpha1.GitOpsConditionStatusTrue,
						Message: "error occured due to xyz",
					},
				},
			}

			syncRunCR.Status = expectedSyncRunStatus
			Expect(k8sClient.Status().Update(ctx, syncRunCR)).To(BeNil())

			err := setGitOpsDeploymentSyncRunCondition(ctx, k8sClient, syncRunCR, managedgitopsv1alpha1.SyncRunConditionType(conditionType), managedgitopsv1alpha1.SyncRunReasonType(conditionType), managedgitopsv1alpha1.GitOpsConditionStatusTrue, expectedSyncRunStatus.Conditions[0].Message)
			Expect(err).To(BeNil())

			Expect(syncRunCR).Should(SatisfyAll(haveErrOccurredConditionSet(expectedSyncRunStatus)))
		})
	})
})
