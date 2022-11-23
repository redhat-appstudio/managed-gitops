package application_event_loop

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
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
			k8sClient         client.Client
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
				getK8sClientForGitOpsEngineInstance: func(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
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
			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(shutdownSignal).To(BeFalse())
		})

		It("should handle a valid GitOpsDeploymentSyncRun by creating a SyncRun Operation", func() {
			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(shutdownSignal).To(BeFalse())

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

			expectedErr := "deployment name field is immutable: changing it from its initial value is not supported"
			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(expectedErr))
			Expect(userDevErr.UserError()).Should(Equal(expectedErr))
			Expect(shutdownSignal).To(BeFalse())

			By("verify if the field .spec.revisionID of GitOpsDeploymentSyncRun is immutable")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDeplSyncRun), gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			gitopsDeplSyncRun.Spec.RevisionID = "main"
			err = k8sClient.Update(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			expectedErr = "deployment name field is immutable: changing it from its initial value is not supported"
			shutdownSignal, userDevErr = applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(expectedErr))
			Expect(userDevErr.UserError()).Should(Equal(expectedErr))
			Expect(shutdownSignal).To(BeFalse())
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
			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(shutdownSignal).To(BeTrue())

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
			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr.DevError().Error()).Should(Equal(expectedErr))
			Expect(shutdownSignal).To(BeFalse())
		})

		It("should return true shutdown signal if neither CR nor DB entry exists", func() {
			By("delete the SyncRun CR and the relevant DB details")
			err := k8sClient.Delete(ctx, gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(shutdownSignal).To(BeTrue())

			By("check if the shutdown signal is true")
			shutdownSignal, userDevErr = applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(shutdownSignal).To(BeTrue())
		})

		It("should throw an error if the namespace doesn't exist", func() {
			applicationAction.eventResourceNamespace = "unknown-namespace"
			shutdownSignal, userDevErr := applicationAction.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)

			expectedDevError := "unable to retrieve namespace 'unknown-namespace': namespaces \"unknown-namespace\" not found"
			expectedUserError := "unable to retrieve the contents of the namespace 'unknown-namespace' containing the API resource 'gitops-syncrun'. Does it exist?"

			Expect(userDevErr.DevError().Error()).Should(Equal(expectedDevError))
			Expect(userDevErr.UserError()).Should(Equal(expectedUserError))
			Expect(shutdownSignal).To(BeFalse())
		})
	})
})
