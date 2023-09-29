package application_event_loop

import (
	"context"
	"encoding/json"
	"time"

	"fmt"
	"strings"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/gitopserrors"

	"github.com/golang/mock/gomock"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1/mocks"
	condition "github.com/redhat-appstudio/managed-gitops/backend/condition/mocks"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventloop_test_util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"

	testStructs "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1/mocks/structs"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"

	conditions "github.com/redhat-appstudio/managed-gitops/backend/condition"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ApplicationEventLoop Test", func() {

	Context("Handle sync run modified", func() {

		It("Ensure the sync run handler can handle a new sync run resource", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual,
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						RepoURL: "https://github.com/test/test",
					},
				},
			}

			gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl-sync",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDepl.Name,
					RevisionID:           "HEAD",
				},
			}

			informer := sharedutil.ListEventReceiver{}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, gitopsDeplSyncRun, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			dbQueries, err := db.NewUnsafePostgresDBQueries(true, false)
			Expect(err).ToNot(HaveOccurred())

			opts := zap.Options{
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()

			// 1) send a deployment modified event, to ensure the deployment is added to the database, and processed
			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     sharedResourceLoop,
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}
			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// 2) add a sync run modified event, to ensure the sync run is added to the database, and processed
			a = applicationEventLoopRunner_Action{
				eventResourceName:       gitopsDeplSyncRun.Name,
				eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
				workspaceClient:         k8sClient,
				log:                     log.FromContext(context.Background()),
				sharedResourceEventLoop: sharedResourceLoop,
				workspaceID:             a.workspaceID,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
				testOnlySkipCreateOperation: true,
			}
			err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Ensure the sync run handler fails when an invalid new sync run resource is passed.", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						RepoURL: "https://github.com/test/test",
					},
				},
			}

			gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("abc", 100),
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDepl.Name,
					RevisionID:           "HEAD",
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, gitopsDeplSyncRun, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			dbQueries, err := db.NewUnsafePostgresDBQueries(true, false)
			Expect(err).ToNot(HaveOccurred())

			opts := zap.Options{
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()

			// 1) send a deployment modified event, to ensure the deployment is added to the database, and processed
			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     sharedResourceLoop,
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}
			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// 2) add a sync run modified event, to ensure the sync run is added to the database, and processed
			a = applicationEventLoopRunner_Action{
				eventResourceName:       gitopsDeplSyncRun.Name,
				eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
				workspaceClient:         k8sClient,
				log:                     log.FromContext(context.Background()),
				sharedResourceEventLoop: sharedResourceLoop,
				workspaceID:             a.workspaceID,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}
			err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
			Expect(err).To(HaveOccurred())

		})
	})

	Context("Handle deployment Health/Sync status.", func() {
		var err error
		var workspaceID string
		var ctx context.Context
		var scheme *runtime.Scheme
		var workspace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var kubesystemNamespace *corev1.Namespace

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err = tests.GenericTestSetup()

			Expect(err).ToNot(HaveOccurred())

			workspaceID = string(workspace.UID)

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should update correct status of deployment after calling DeploymentStatusTick handler.", func() {
			// ----------------------------------------------------------------------------
			By("Create new deployment.")
			// ----------------------------------------------------------------------------

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						RepoURL: "https://github.com/test/test",
					},
				},
			}

			k8sClient := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Get DeploymentToApplicationMapping and Application objects, to be used later.")
			// ----------------------------------------------------------------------------
			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).ToNot(HaveOccurred())

				Expect(appMappings).To(HaveLen(1))

				deplToAppMapping = appMappings[0]
			}

			// ----------------------------------------------------------------------------
			By("Inserting dummy data into ApplicationState table, because we are not calling the Reconciler for this, which updates the status of application into db.")
			// ----------------------------------------------------------------------------
			var resourceStatus managedgitopsv1alpha1.ResourceStatus
			var resources []managedgitopsv1alpha1.ResourceStatus
			resources = append(resources, resourceStatus)

			compressedResources, err := sharedutil.CompressObject(&resources)
			Expect(err).ToNot(HaveOccurred())
			Expect(compressedResources).ToNot(BeNil())

			_, reconciledobj, err := dummyApplicationComparedToField()
			Expect(err).ToNot(HaveOccurred())

			resourceBytes, err := json.Marshal(resources)
			Expect(err).ToNot(HaveOccurred())

			fauxResources := []fauxargocd.ResourceStatus{}
			err = json.Unmarshal(resourceBytes, &fauxResources)
			Expect(err).ToNot(HaveOccurred())

			appStatus := &fauxargocd.FauxApplicationStatus{
				Resources: fauxResources,
				OperationState: &fauxargocd.OperationState{
					Message: "Sample message",
					Operation: fauxargocd.Operation{
						InitiatedBy: fauxargocd.OperationInitiator{
							Automated: true,
						},
						Retry: fauxargocd.RetryStrategy{
							Limit: -1,
						},
					},
					SyncResult: &fauxargocd.SyncOperationResult{
						Resources: fauxargocd.ResourceResults{
							{
								Group:     "",
								HookPhase: fauxargocd.OperationRunning,
								Namespace: "jane",
								Status:    fauxargocd.ResultCodeSynced,
							},
						},
					},
					RetryCount: 1,
				},
				Health: fauxargocd.HealthStatus{
					Status:  fauxargocd.HealthStatusHealthy,
					Message: "success",
				},
				Sync: fauxargocd.SyncStatus{
					Status:     fauxargocd.SyncStatusCodeSynced,
					Revision:   "abcdefg",
					ComparedTo: reconciledobj,
				},
				Conditions: []fauxargocd.ApplicationCondition{
					{
						Type:    managedgitopsv1alpha1.ApplicationConditionComparisonError,
						Message: "comparision error",
					},
					{
						Type:    managedgitopsv1alpha1.ApplicationConditionSharedResourceWarning,
						Message: "shared resource warning",
					},
				},
			}

			appStatusBytes, err := sharedutil.CompressObject(appStatus)
			Expect(err).ToNot(HaveOccurred())

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: deplToAppMapping.Application_id,
				ArgoCD_Application_Status:       appStatusBytes,
			}

			err = dbQueries.CreateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync before calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)

			Expect(clientErr).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Status.Health.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Revision).To(BeEmpty())
			Expect(gitopsDeployment.Status.Health.Message).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Namespace).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Name).To(BeEmpty())
			Expect(gitopsDeployment.Status.Conditions).To(BeNil())
			Expect(gitopsDeployment.Status.OperationState).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status.")
			// ----------------------------------------------------------------------------

			updated, err := a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, gitopsDepl.Name, gitopsDepl.Namespace, dbQueries)
			Expect(updated).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync after calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)

			Expect(clientErr).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Status.Health.Status).To(Equal(managedgitopsv1alpha1.HeathStatusCodeHealthy))
			Expect(gitopsDeployment.Status.Sync.Status).To(Equal(managedgitopsv1alpha1.SyncStatusCodeSynced))
			Expect(gitopsDeployment.Status.Sync.Revision).To(Equal(appStatus.Sync.Revision))
			Expect(gitopsDeployment.Status.Health.Message).To(Equal(appStatus.Health.Message))
			Expect(gitopsDeployment.Status.ReconciledState.Source.Path).To(Equal(reconciledobj.Source.Path))
			Expect(gitopsDeployment.Status.ReconciledState.Source.RepoURL).To(Equal(reconciledobj.Source.RepoURL))
			Expect(gitopsDeployment.Status.ReconciledState.Source.Branch).To(Equal(reconciledobj.Source.TargetRevision))
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Namespace).To(Equal(reconciledobj.Destination.Namespace))

			Expect(gitopsDeployment.Status.OperationState).ToNot(BeNil())
			opStateBytes, err := json.Marshal(appStatus.OperationState)
			Expect(err).ToNot(HaveOccurred())

			operationState := &managedgitopsv1alpha1.OperationState{}
			err = json.Unmarshal(opStateBytes, operationState)
			Expect(err).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Status.OperationState.Message).To(Equal(operationState.Message))
			Expect(gitopsDeployment.Status.OperationState.RetryCount).To(Equal(operationState.RetryCount))
			Expect(gitopsDeployment.Status.OperationState.Operation).To(Equal(operationState.Operation))
			Expect(gitopsDeployment.Status.OperationState.SyncResult).To(Equal(operationState.SyncResult))

			By("verify if conditions from both ApplicationState and GitOpsDeployment match")
			for _, c := range appStatus.Conditions {
				matchingCondition, _ := conditions.NewConditionManager().FindCondition(&gitopsDeployment.Status.Conditions, managedgitopsv1alpha1.GitOpsDeploymentConditionType(c.Type))

				Expect(matchingCondition).ToNot(BeNil())
				Expect(matchingCondition.Message).To(Equal(c.Message))
				Expect(matchingCondition.Type).To(Equal(managedgitopsv1alpha1.GitOpsDeploymentConditionType(c.Type)))
				Expect(matchingCondition.Status).To(Equal(managedgitopsv1alpha1.GitOpsConditionStatusTrue))
			}

			By("Update conditions in ApplicationState to be empty")
			appStatus.Conditions = []fauxargocd.ApplicationCondition{}
			appStatusBytes, err = sharedutil.CompressObject(appStatus)
			Expect(err).ToNot(HaveOccurred())

			applicationState = &db.ApplicationState{
				Applicationstate_application_id: deplToAppMapping.Application_id,
				ArgoCD_Application_Status:       appStatusBytes,
			}

			err = dbQueries.UpdateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			updated, err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, gitopsDepl.Name, gitopsDepl.Namespace, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated).To(BeTrue())

			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(clientErr).ToNot(HaveOccurred())

			By("Verify that the status of existing GitOpsDeployment conditions is false as applicationState.conditions is empty")
			for _, c := range gitopsDeployment.Status.Conditions {
				Expect(c).ToNot(BeNil())
				Expect(c.Message).To(BeEmpty())
				Expect(c.Status).To(Equal(managedgitopsv1alpha1.GitOpsConditionStatusFalse))
			}

			By("attempting to update the deployment status tick, even though nothing has changed.")
			updated, err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, gitopsDepl.Name, gitopsDepl.Namespace, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated).To(BeFalse(), "since nothing has changed, the GitOpsDeployment should not have been updated")

			// ----------------------------------------------------------------------------
			By("Delete GitOpsDepl to clean resources.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
		})

		It("Should report an error via status condition of deployment, when path field of gitopsDeployment is empty or '/'", func() {

			By("Create new deployment with a missing path")

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL: "https://github.com/test/test",
					},
				},
			}

			k8sClient := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr.UserError()).To(Equal(managedgitopsv1alpha1.GitOpsDeploymentUserError_PathIsRequired))

			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}

			By("Update the path to correct value, and verify error is now nil")
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDepl)
			Expect(clientErr).ToNot(HaveOccurred())

			gitopsDepl.Spec.Source.Path = "resources/test-data/sample-gitops-repository/environments/overlays/dev"
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			By("Update the path to '/' and verify that error condition is set")
			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDepl)
			Expect(clientErr).ToNot(HaveOccurred())

			gitopsDepl.Spec.Source.Path = "/"
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr.UserError()).To(Equal(managedgitopsv1alpha1.GitOpsDeploymentUserError_InvalidPathSlash))

			By("Update the path to correct value, and verify error is nil again")
			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDepl)
			Expect(clientErr).ToNot(HaveOccurred())

			gitopsDepl.Spec.Source.Path = "resources/test-data/sample-gitops-repository/environments/overlays/dev"
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

		})

		It("Verify that the .status.reconciledState value of the GitOpsDeployment resource correctly references the name of the GitOpsDeploymentManagedEnvironment resource", func() {
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/abc-org/abc-repo",
						Path:           "/abc-path",
						TargetRevision: "abc-commit"},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
				},
			}

			k8sClient := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			// ----------------------------------------------------------------------------
			By("Create ManagedEnvironment CR")
			// ----------------------------------------------------------------------------
			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
					ClusterCredentialsSecret: "fake-secret-name",
					CreateNewServiceAccount:  true,
				},
			}
			err = k8sClient.Create(ctx, &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Create ManagedEnvironment Secret")
			// ----------------------------------------------------------------------------
			kubeConfigContents := generateFakeKubeConfig()
			managedEnvSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      managedEnvCR.Spec.ClusterCredentialsSecret,
					Namespace: workspace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					shared_resource_loop.KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}
			err = k8sClient.Create(ctx, &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl.Spec.Destination.Environment = managedEnvCR.Name
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Name:                  "my-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Create apiCRToDBMapping in database")
			// ----------------------------------------------------------------------------
			apiCRToDBMapping := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:       string(managedEnvCR.UID),
				APIResourceName:      managedEnvCR.Name,
				APIResourceNamespace: managedEnvCR.Namespace,
				NamespaceUID:         string(workspace.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        managedEnvironment.Managedenvironment_id,
			}
			err = dbQueries.CreateAPICRToDatabaseMapping(ctx, &apiCRToDBMapping)
			Expect(err).ToNot(HaveOccurred())

			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnvCR.UID), k8sClient)

			mockK8sClientFactory := MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory:            mockK8sClientFactory,
			}

			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Get DeploymentToApplicationMapping and Application objects, to be used later.")
			// ----------------------------------------------------------------------------
			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).ToNot(HaveOccurred())

				Expect(appMappings).To(HaveLen(1))

				deplToAppMapping = appMappings[0]
			}

			// ----------------------------------------------------------------------------
			By("Inserting dummy data into ApplicationState table, because we are not calling the Reconciler for this, which updates the status of application into db.")
			// ----------------------------------------------------------------------------
			var resourceStatus managedgitopsv1alpha1.ResourceStatus
			var resources []managedgitopsv1alpha1.ResourceStatus
			resources = append(resources, resourceStatus)

			// Create ReconciledState
			fauxcomparedTo := fauxargocd.FauxComparedTo{
				Source: fauxargocd.ApplicationSource{
					RepoURL:        gitopsDepl.Spec.Source.RepoURL,
					Path:           gitopsDepl.Spec.Source.Path,
					TargetRevision: gitopsDepl.Spec.Source.TargetRevision,
				},
				Destination: fauxargocd.ApplicationDestination{
					Namespace: gitopsDepl.Spec.Destination.Namespace,
					Name:      managedEnvironment.Managedenvironment_id,
				},
			}

			resourceBytes, err := json.Marshal(resources)
			Expect(err).ToNot(HaveOccurred())

			fauxResources := []fauxargocd.ResourceStatus{}
			err = json.Unmarshal(resourceBytes, &fauxResources)
			Expect(err).ToNot(HaveOccurred())

			appStatus := &fauxargocd.FauxApplicationStatus{
				Health: fauxargocd.HealthStatus{
					Status:  fauxargocd.HealthStatusHealthy,
					Message: "Success",
				},
				Resources: fauxResources,
				Sync: fauxargocd.SyncStatus{
					Status:     fauxargocd.SyncStatusCodeSynced,
					Revision:   "abcdefg",
					ComparedTo: fauxcomparedTo,
				},
			}

			appStatusBytes, err := sharedutil.CompressObject(appStatus)
			Expect(err).ToNot(HaveOccurred())

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: deplToAppMapping.Application_id,
				ArgoCD_Application_Status:       appStatusBytes,
			}

			err = dbQueries.CreateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync before calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(clientErr).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Status.Health.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Revision).To(BeEmpty())
			Expect(gitopsDeployment.Status.Health.Message).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Namespace).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Name).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Namespace).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Source.Path).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Source.RepoURL).To(BeEmpty())
			Expect(gitopsDeployment.Status.ReconciledState.Source.Branch).To(BeEmpty())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status and reconciledState")
			// ----------------------------------------------------------------------------

			updated, err := a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, gitopsDeployment.Name, gitopsDeployment.Namespace, dbQueries)
			Expect(updated).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync  and reconciledState after calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(clientErr).ToNot(HaveOccurred())

			err = dbQueries.GetApplicationStateById(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Status.Health.Status).To(Equal(managedgitopsv1alpha1.HeathStatusCodeHealthy))
			Expect(gitopsDeployment.Status.Sync.Status).To(Equal(managedgitopsv1alpha1.SyncStatusCodeSynced))
			Expect(gitopsDeployment.Status.Sync.Revision).To(Equal("abcdefg"))
			Expect(gitopsDeployment.Status.Health.Message).To(Equal("Success"))
			Expect(gitopsDeployment.Status.ReconciledState.Source.Path).To(Equal(fauxcomparedTo.Source.Path))
			Expect(gitopsDeployment.Status.ReconciledState.Source.RepoURL).To(Equal(fauxcomparedTo.Source.RepoURL))
			Expect(gitopsDeployment.Status.ReconciledState.Source.Branch).To(Equal(fauxcomparedTo.Source.TargetRevision))
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Namespace).To(Equal(fauxcomparedTo.Destination.Namespace))
			Expect(gitopsDeployment.Status.ReconciledState.Destination.Name).To(Equal(apiCRToDBMapping.APIResourceName))

			// ----------------------------------------------------------------------------
			By("Delete GitOpsDepl to clean resources.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

		})

		// It("Should not return any error, if deploymentToApplicationMapping doesn't exist for given gitopsdeployment.", func() {
		// 	// Don't create Deployment and resources by calling applicationEventRunner_handleDeploymentModified function,
		// 	// but check for Health/Sync status of deployment which doesn't exist.
		// 	k8sClientOuter := fake.NewClientBuilder().Build()
		// 	k8sClient := &sharedutil.ProxyClient{
		// 		InnerClient: k8sClientOuter,
		// 	}

		// a := applicationEventLoopRunner_Action{
		// 	getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
		// 		return k8sClient, nil
		// 	},
		// 	eventResourceName:           "dummy-deployment",
		// 	eventResourceNamespace:      workspace.Namespace,
		// 	workspaceClient:             k8sClient,
		// 	log:                         log.FromContext(context.Background()),
		// 	sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
		// 	workspaceID:                 workspaceID,
		// 	testOnlySkipCreateOperation: true,
		// 	k8sClientFactory: MockSRLK8sClientFactory{
		// 		fakeClient: k8sClient,
		// 	},
		// }

		// 	// ----------------------------------------------------------------------------
		// 	By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status of a deployment which doesn't exist.")
		// 	// ----------------------------------------------------------------------------
		// 	err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, "dummy-deployment", dbQueries)
		// 	Expect(err).ToNot(HaveOccurred())
		// })

		It("Should not return an error, if the GitOpsDeployment resource with name/namespace doesn't exist", func() {

			// Don't create Deployment and resources by calling applicationEventRunner_handleDeploymentModified function,
			// But check for Health/Sync status of a deployment having invalid name.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			a := applicationEventLoopRunner_Action{
				eventResourceName:           "dummy-deployment",
				eventResourceNamespace:      workspace.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status of a deployment that doesn't exist.")
			// ----------------------------------------------------------------------------

			updated, err := a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, a.eventResourceName, a.eventResourceNamespace, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated).To(BeFalse())
		})

		It("Should not return error, if GitOpsDeployment doesn't exist in given namespace.", func() {
			// ----------------------------------------------------------------------------
			By("Create new deployment, even though it will be deleted later, but we need to do this to create resources to pass initial checks of applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						RepoURL: "https://github.com/test/test",
					},
				},
			}

			k8sClientOuter := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Delete deployment, but we don't want to delete other DB entries, hence not calling applicationEventRunner_handleDeploymentModified after deleting deployment.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status for a deployment which doesn'r exist in given namespace.")
			// ----------------------------------------------------------------------------
			updated, err := a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, gitopsDepl.Name, gitopsDepl.Namespace, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated).To(BeFalse())

			// ----------------------------------------------------------------------------
			By("Deployment is already been deleted in previous step, now delete related db entries and clean resources.")
			// ----------------------------------------------------------------------------
			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
		})

		It("Should not return error, if ApplicationState doesnt exists for given GitOpsDeployment.", func() {
			// ----------------------------------------------------------------------------
			By("Create new deployment.")
			// ----------------------------------------------------------------------------

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						RepoURL: "https://github.com/test/test",
					},
				},
			}

			k8sClientOuter := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			a := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status for deployment which is missing ApplicationState entries.")
			// ----------------------------------------------------------------------------

			updated, err := a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, gitopsDepl.Name, gitopsDepl.Namespace, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated).To(BeFalse())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync, it should not have any Health/Sync status.")
			// ----------------------------------------------------------------------------

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)

			Expect(clientErr).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Status.Health.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Revision).To(BeEmpty())
			Expect(gitopsDeployment.Status.Health.Message).To(BeEmpty())

			// ----------------------------------------------------------------------------
			By("Delete GitOpsDepl to clean resources.")
			// ----------------------------------------------------------------------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
		})

	})

	Context("Test extractResourceStatus function", func() {
		var (
			inputResource = fauxargocd.ResourceStatus{
				Kind:      "sample-kind",
				Group:     "sample-resource",
				Version:   "v1",
				Namespace: "sample-ns",
				Name:      "sample-name",
				Status:    fauxargocd.SyncStatusCodeSynced,
				Health: &fauxargocd.HealthStatus{
					Status: fauxargocd.HealthStatusHealthy,
				},
			}

			expectedResource = managedgitopsv1alpha1.ResourceStatus{
				Kind:      inputResource.Kind,
				Group:     inputResource.Group,
				Version:   inputResource.Version,
				Namespace: inputResource.Namespace,
				Name:      inputResource.Name,
				Status:    managedgitopsv1alpha1.SyncStatusCode(inputResource.Status),
				Health: &managedgitopsv1alpha1.HealthStatus{
					Status: managedgitopsv1alpha1.HealthStatusCode(inputResource.Health.Status),
				},
			}
		)
		It("should return an empty resourceStatus if the input slice is empty", func() {
			resourcesStatus := extractResourceStatus([]fauxargocd.ResourceStatus{})
			Expect(resourcesStatus).To(BeEmpty())
		})

		It("should convert the input faux ResourceStatus to GitOpsDeployment ResourceStatus", func() {
			resourceStatus := extractResourceStatus([]fauxargocd.ResourceStatus{inputResource})
			Expect(resourceStatus).To(HaveLen(1))
			Expect(resourceStatus).To(Equal([]managedgitopsv1alpha1.ResourceStatus{expectedResource}))
		})

		It("should not update health status if the input resource health status is nil", func() {
			inputResource.Health = nil
			expectedResource.Health = nil
			resourceStatus := extractResourceStatus([]fauxargocd.ResourceStatus{inputResource})
			Expect(resourceStatus).To(HaveLen(1))
			Expect(resourceStatus[0].Health).To(BeNil())
			Expect(resourceStatus).To(Equal([]managedgitopsv1alpha1.ResourceStatus{expectedResource}))
		})
	})

	Context("Test extractOperationState function", func() {
		It("should convert the faux OperationState to GitOpsDeployment OperationState", func() {
			inputState := fauxargocd.OperationState{
				RetryCount: 1,
				Message:    "sample",
				Operation: fauxargocd.Operation{
					Sync: &fauxargocd.SyncOperation{
						Revision: "abc",
					},
				},
				Phase: fauxargocd.OperationRunning,
				SyncResult: &fauxargocd.SyncOperationResult{
					Resources: fauxargocd.ResourceResults{
						{
							Namespace: "sample-ns",
							Kind:      "sample-kind",
						},
					},
				},
			}

			expectedState := managedgitopsv1alpha1.OperationState{
				RetryCount: 1,
				Message:    "sample",
				Operation: managedgitopsv1alpha1.ApplicationOperation{
					Sync: &managedgitopsv1alpha1.SyncOperation{
						Revision: "abc",
					},
				},
				Phase: managedgitopsv1alpha1.OperationRunning,
				SyncResult: &managedgitopsv1alpha1.SyncOperationResult{
					Resources: managedgitopsv1alpha1.ResourceResults{
						{
							Namespace: "sample-ns",
							Kind:      "sample-kind",
						},
					},
				},
			}

			opState, err := extractOperationState(&inputState)
			Expect(err).ToNot(HaveOccurred())
			Expect(*opState).To(Equal(expectedState))
		})
	})

	Context("Test removeFinalizerIfExist function", func() {

		var (
			ctx        context.Context
			k8sClient  *sharedutil.ProxyClient
			gitopsDepl *managedgitopsv1alpha1.GitOpsDeployment
			informer   sharedutil.ListEventReceiver
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme, _, _, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl = &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/abc-org/abc-repo",
						Path:           "/abc-path",
						TargetRevision: "abc-commit"},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
				},
			}

			innerClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, gitopsDepl).Build()

			informer = sharedutil.ListEventReceiver{}
			k8sClient = &sharedutil.ProxyClient{
				InnerClient: innerClient,
				Informer:    &informer,
			}
		})

		It("should remove the finalizer when it is found", func() {
			testFinalizer := "managed.gitops.test/test"
			gitopsDepl.Finalizers = append(gitopsDepl.Finalizers, testFinalizer)
			err := k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			err = removeFinalizerIfExists(ctx, k8sClient, gitopsDepl, testFinalizer)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeplUpdated := false
			for _, event := range informer.Events {
				if event.Action == sharedutil.Update && event.ObjectTypeOf() == "GitOpsDeployment" {
					gitopsDeplUpdated = true
				}
			}
			Expect(gitopsDeplUpdated).To(BeTrue())
		})

		It("should not update if the finalizer is not found", func() {
			err := removeFinalizerIfExists(ctx, k8sClient, gitopsDepl, managedgitopsv1alpha1.DeletionFinalizer)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeplUpdated := false
			for _, event := range informer.Events {
				if event.Action == sharedutil.Update && event.ObjectTypeOf() == "GitOpsDeployment" {
					gitopsDeplUpdated = true
				}
			}
			Expect(gitopsDeplUpdated).To(BeFalse())
		})

		It("should not update if the GitOpsDeployment is already deleted", func() {
			err := k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl.Finalizers = append(gitopsDepl.Finalizers, managedgitopsv1alpha1.DeletionFinalizer)
			err = removeFinalizerIfExists(ctx, k8sClient, gitopsDepl, managedgitopsv1alpha1.DeletionFinalizer)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeplUpdated := false
			for _, event := range informer.Events {
				if event.Action == sharedutil.Update && event.ObjectTypeOf() == "GitOpsDeployment" {
					gitopsDeplUpdated = true
				}
			}
			Expect(gitopsDeplUpdated).To(BeFalse())
		})

		It("should retry when there is a conflict", func() {
			By("introduce a conflict by updating the object")
			gitopsDeplClone := gitopsDepl.DeepCopy()
			gitopsDeplClone.Finalizers = append(gitopsDeplClone.Finalizers, managedgitopsv1alpha1.DeletionFinalizer)
			err := k8sClient.Update(ctx, gitopsDeplClone)
			Expect(err).ToNot(HaveOccurred())

			By("check if there will be a conflict on update")
			gitopsDepl.Spec.Source.Path = "/sample"
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsConflict(err)).To(BeTrue())

			By("verify if the conflict will be handled by retrying")
			err = removeFinalizerIfExists(ctx, k8sClient, gitopsDepl, managedgitopsv1alpha1.DeletionFinalizer)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeplUpdated := false
			for _, event := range informer.Events {
				if event.Action == sharedutil.Update && event.ObjectTypeOf() == "GitOpsDeployment" {
					gitopsDeplUpdated = true
				}
			}
			Expect(gitopsDeplUpdated).To(BeTrue())
		})
	})

	Context("Generic Application Event Loop Runner tests", func() {

		var ctx context.Context
		var cancelFunc context.CancelFunc
		var namespace *corev1.Namespace
		var k8sClient client.Client
		var dbQueries db.AllDatabaseQueries

		var clusterUser *db.ClusterUser

		var sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop

		// findDBApplicationIDforGitOpsDeployment locates the Application row primary key for a particular GitOpsDeployment
		findDBApplicationIDforGitOpsDeployment := func(ctx context.Context, gitopsDeploymentName string, gitopsDeploymentNamespace string, namespace corev1.Namespace) string {

			var deplToAppMappings []db.DeploymentToApplicationMapping

			if err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, string(namespace.GetUID()), &deplToAppMappings); err != nil {
				fmt.Println(err)
				return ""
			}

			if len(deplToAppMappings) != 1 {
				fmt.Println("waiting for dtams, currently", len(deplToAppMappings))
				return ""
			}

			return deplToAppMappings[0].Application_id
		}

		BeforeEach(func() {
			ctx, cancelFunc = context.WithCancel(context.Background())

			var err error
			var scheme *runtime.Scheme
			var argocdNamespace *corev1.Namespace
			var kubesystemNamespace *corev1.Namespace

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err = tests.GenericTestSetup()

			Expect(err).ToNot(HaveOccurred())

			k8sClient = fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).ToNot(HaveOccurred())

			sharedResourceEventLoop = shared_resource_loop.NewSharedResourceLoop()

			clusterUser, _, err = sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, log.FromContext(ctx))
			Expect(err).To(Succeed())

		})

		AfterEach(func() {
			cancelFunc()
		})

		When("the application event loop runner receives a GitOpsDeployment event for a new GitOpsDeployment", func() {

			It("verifies the event is processed and that Application and Operation are created", func() {

				By("creating a generic GitOpsDeployment")

				gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gitops-depl",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/abc-org/abc-repo",
							Path:           "/abc-path",
							TargetRevision: "abc-commit"},
						Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
						Destination: managedgitopsv1alpha1.ApplicationDestination{},
					},
				}

				Expect(k8sClient.Create(ctx, gitopsDepl)).To(Succeed())

				informWorkCompleteChan := make(chan RequestMessage)

				gitopsDeploymentName := gitopsDepl.Name
				gitopsDeploymentNamespace := gitopsDepl.Namespace
				namespaceID := (string)(namespace.UID)

				fakeFactory := MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				}

				By("starting the application event runner, and giving it our fake k8s client")
				inputChannel := startNewApplicationEventLoopRunner(ctx, informWorkCompleteChan, sharedResourceEventLoop, gitopsDeploymentName, gitopsDeploymentNamespace, namespaceID, "", fakeFactory)

				operationCreated := make(chan bool)

				go func() {

					defer GinkgoRecover()

					for {

						// End the goroutine on cancel
						select {
						case <-ctx.Done():
							return
						default:
						}

						// Wait for the Application row to exist
						applId := findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
						if applId == "" {
							time.Sleep(200 * time.Millisecond)
							continue
						}

						var operations []db.Operation

						// Wait for the Operation to exist
						Expect(dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, applId, db.OperationResourceType_Application, &operations, clusterUser.Clusteruser_id)).To(Succeed())

						if len(operations) != 1 {
							fmt.Println("waiting for operation, currently there are ", len(operations), "operation(s)")
							time.Sleep(200 * time.Millisecond)
							continue
						}

						operation := operations[0]

						operation.State = db.OperationState_Completed

						Expect(dbQueries.UpdateOperation(ctx, &operation)).To(Succeed())

						fmt.Println("Updated operation to Complete")

						// Inform that the operation was updated sucecssfully
						operationCreated <- true
					}
				}()

				By("sending a standard event notification for the GitOpsDeployment")
				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDepl.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: namespaceID,
				}

				resp := <-informWorkCompleteChan

				By("verifying the event response is a work complete on the initial resource")
				Expect(resp.Message.Event).ToNot(BeNil())
				Expect(resp.Message.Event.EventType).To(Equal(eventlooptypes.DeploymentModified))
				Expect(resp.Message.Event.Request.Name).To(Equal(gitopsDepl.Name))
				Expect(resp.Message.Event.Request.Namespace).To(Equal(gitopsDepl.Namespace))
				Expect(resp.Message.MessageType).To(Equal(eventlooptypes.ApplicationEventLoopMessageType_WorkComplete))

				wasOperationCreated := <-operationCreated
				Expect(wasOperationCreated).To(BeTrue())

				applId := findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
				Expect(applId).ToNot(BeEmpty(), "Application row should exist in the DB, for the initial GitOpsDeployment")

			})
		})

		When("the application event loop runner receives a GitOpsDeploymentSyncRun event for a GitOpsDeployment", func() {

			It("verifies the event is processed", func() {

				By("creating a generic GitOpsDeployment of type manual sy=nc")

				gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gitops-depl",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/abc-org/abc-repo",
							Path:           "/abc-path",
							TargetRevision: "abc-commit"},
						Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual,
						Destination: managedgitopsv1alpha1.ApplicationDestination{},
					},
				}

				Expect(k8sClient.Create(ctx, gitopsDepl)).To(Succeed())

				informWorkCompleteChan := make(chan RequestMessage)

				gitopsDeploymentName := gitopsDepl.Name
				gitopsDeploymentNamespace := gitopsDepl.Namespace
				namespaceID := (string)(namespace.UID)

				fakeFactory := MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				}

				By("starting the application event runner, and giving it our fake k8s client")
				inputChannel := startNewApplicationEventLoopRunner(ctx, informWorkCompleteChan, sharedResourceEventLoop, gitopsDeploymentName, gitopsDeploymentNamespace, namespaceID, "", fakeFactory)

				By("creating a go routine that will mark Operations as completed, to simulate cluster-agent")
				go func() {

					defer GinkgoRecover()

					for {

						// End the goroutine on cancel
						select {
						case <-ctx.Done():
							return
						default:
						}

						applId := findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
						if applId == "" {
							fmt.Println("waitin for application for gitopsdepl")
							time.Sleep(500 * time.Millisecond)
							continue
						}

						var operations []db.Operation

						Expect(dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, applId, db.OperationResourceType_Application, &operations, clusterUser.Clusteruser_id)).To(Succeed())

						match := false
						for _, operation := range operations {

							if operation.State == db.OperationState_Completed { // skip already completed
								continue
							}

							operation.State = db.OperationState_Completed

							Expect(dbQueries.UpdateOperation(ctx, &operation)).To(Succeed())

							fmt.Println("Updated Application operation to Complete", operation.Operation_id)

							match = true

						}

						if !match {
							fmt.Println("waiting for Application operations", len(operations))
							time.Sleep(500 * time.Millisecond)
						}

					}
				}()

				By("sending a standard event notification for the GitOpsDeployment")

				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDepl.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: namespaceID,
				}

				resp := <-informWorkCompleteChan
				Expect(resp.Message.Event).ToNot(BeNil())
				Expect(resp.Message.Event.EventType).To(Equal(eventlooptypes.DeploymentModified))

				By("creating a GitOpsDeploymentSyncRun to trigger the Deployment")
				gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gitops-depl-syncrun",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
						GitopsDeploymentName: gitopsDeploymentName,
						RevisionID:           "main",
					},
				}
				Expect(k8sClient.Create(ctx, gitopsDeplSyncRun)).To(Succeed())

				By("sending a standard event notification for the GitOpsDeploymentSyncRun, targeting the above GitOpDeploymentSyncRun")

				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.SyncRunModified,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDeplSyncRun.Namespace,
							Name:      gitopsDeplSyncRun.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentSyncRunTypeName,
					WorkspaceID: namespaceID,
				}

				var applId string
				Eventually(func() bool {
					applId = findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
					return applId != ""

				}, "20s", "1s").Should(BeTrue(), "an Applicaton row should be created for the GitOpDeployment")

				var syncOperation db.SyncOperation
				Eventually(func() bool {
					var syncOperations []db.SyncOperation
					Expect(dbQueries.UnsafeListAllSyncOperations(ctx, &syncOperations)).To(Succeed())

					for _, syncOperationEntry := range syncOperations {

						if syncOperationEntry.Application_id == applId {
							syncOperation = syncOperationEntry
							return true
						}
					}

					fmt.Println("waiting for syncoperations", len(syncOperations))

					return false

				}, "20s", "1s").Should(BeTrue(), "a SyncOperation pointing to the Application (of the GitOpDeployment) should be created")

				Eventually(func() bool {

					var operations []db.Operation

					Expect(dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, syncOperation.SyncOperation_id, db.OperationResourceType_SyncOperation, &operations, clusterUser.Clusteruser_id)).To(Succeed())

					if len(operations) != 1 {
						fmt.Println("waiting for operation pointing to SyncOperation", len(operations))
						return false
					}

					operations[0].State = db.OperationState_Completed
					Expect(dbQueries.UpdateOperation(ctx, &operations[0])).To(Succeed())

					return true
				}, "20s", "1s").Should(BeTrue(), "an operation should be created pointing to the SyncOperation")

				resp = <-informWorkCompleteChan

				Expect(resp.Message.Event).ToNot(BeNil())
				Expect(resp.Message.Event.EventType).To(Equal(eventlooptypes.SyncRunModified))
				Expect(resp.Message.Event.Request.Name).To(Equal(gitopsDeplSyncRun.Name))
				Expect(resp.Message.Event.Request.Namespace).To(Equal(gitopsDeplSyncRun.Namespace))
				Expect(resp.Message.MessageType).To(Equal(eventlooptypes.ApplicationEventLoopMessageType_WorkComplete))

			})
		})

		When("the application event loop runner receives a GitOpsDeploymentManagedEnvironment event for a GitOpsDeployment", func() {

			It("verifies the event is processed and we receive a WorkCompleted response", func() {

				By("creating the GitOpsDeploymentManagedEnvironment CR and the corresponding Secret")

				kubeConfigContents := generateFakeKubeConfig()
				managedEnvSecret2 := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster-secret-2",
						Namespace: namespace.Name,
					},
					Type: sharedutil.ManagedEnvironmentSecretType,
					Data: map[string][]byte{
						shared_resource_loop.KubeconfigKey: ([]byte)(kubeConfigContents),
					},
				}
				Expect(k8sClient.Create(ctx, &managedEnvSecret2)).ToNot(HaveOccurred())

				gitopsDeplManagedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-env",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
						APIURL:                     "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
						ClusterCredentialsSecret:   managedEnvSecret2.Name,
						AllowInsecureSkipTLSVerify: true,
						CreateNewServiceAccount:    true,
					},
				}
				Expect(k8sClient.Create(ctx, gitopsDeplManagedEnv)).To(Succeed())

				By("simulating K8s ServiceAccount behaviour in our fake client, so ManagedEnvironment can successfully reconcile the fake cluster defined above")
				eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(gitopsDeplManagedEnv.UID), k8sClient)

				By("creating a corresponding GitOpsDeployment targeting the ManagedEnvironment")
				gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gitops-depl",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/abc-org/abc-repo",
							Path:           "/abc-path",
							TargetRevision: "abc-commit"},
						Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual,
						Destination: managedgitopsv1alpha1.ApplicationDestination{
							Namespace:   "some-namespace",
							Environment: "my-env",
						},
					},
				}
				Expect(k8sClient.Create(ctx, gitopsDepl)).To(Succeed())

				informWorkCompleteChan := make(chan RequestMessage)

				gitopsDeploymentName := gitopsDepl.Name
				gitopsDeploymentNamespace := gitopsDepl.Namespace
				namespaceID := (string)(namespace.UID)

				fakeFactory := MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				}

				By("starting the application event runner, and giving it our fake k8s client")
				inputChannel := startNewApplicationEventLoopRunner(ctx, informWorkCompleteChan, sharedResourceEventLoop, gitopsDeploymentName, gitopsDeploymentNamespace, namespaceID, "", fakeFactory)

				applicationIdChan := make(chan string, 10)

				By("creating a go routine that will simulate cluster-agent: mark Operations as complete")
				go func() {

					defer GinkgoRecover()

					for {

						// End the goroutine on cancel
						select {
						case <-ctx.Done():
							return
						default:
						}

						// wait for Applicaton row to exist for GitopsDeployment
						applId := findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
						if applId == "" {
							fmt.Println("waitin for application for gitopsdepl")
							time.Sleep(250 * time.Millisecond)
							continue
						}

						// send the application id back on the channel
						applicationIdChan <- applId

						// Wait for Operations to exist that target the Application (and mark them as Complete)
						var operations []db.Operation
						Expect(dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, applId, db.OperationResourceType_Application, &operations, clusterUser.Clusteruser_id)).To(Succeed())

						match := false
						for _, operation := range operations {

							if operation.State == db.OperationState_Completed { // skip already completed
								continue
							}

							operation.State = db.OperationState_Completed

							Expect(dbQueries.UpdateOperation(ctx, &operation)).To(Succeed())

							fmt.Println("Updated Application operation to Complete", operation.Operation_id)

							match = true

						}

						if !match {
							fmt.Println("waiting for Application operations", len(operations))
							time.Sleep(500 * time.Millisecond)
						}
					}
				}()

				By("sending a standard event notification for the GitOpsDeployment")

				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDepl.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: namespaceID,
				}

				resp := <-informWorkCompleteChan
				Expect(resp.Message.Event).ToNot(BeNil())
				Expect(resp.Message.Event.EventType).To(Equal(eventlooptypes.DeploymentModified))

				By("sending a standard event notification for the GitOpsDeploymentManagedEnvironment")

				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.ManagedEnvironmentModified,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDeplManagedEnv.Namespace,
							Name:      gitopsDeplManagedEnv.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
					WorkspaceID: namespaceID,
				}

				resp = <-informWorkCompleteChan

				By("verifying the ManagedEnvironment event was processed successfully")
				Expect(resp.Message.Event).ToNot(BeNil())
				Expect(resp.Message.Event.EventType).To(Equal(eventlooptypes.ManagedEnvironmentModified))
				Expect(resp.Message.Event.Request.Name).To(Equal(gitopsDeplManagedEnv.Name))
				Expect(resp.Message.Event.Request.Namespace).To(Equal(gitopsDeplManagedEnv.Namespace))
				Expect(resp.Message.MessageType).To(Equal(eventlooptypes.ApplicationEventLoopMessageType_WorkComplete))

				applicationId := <-applicationIdChan

				applRow := db.Application{
					Application_id: applicationId,
				}

				By("verifying the ManagedEnvironment was created")
				Expect(dbQueries.GetApplicationById(ctx, &applRow)).To(Succeed())

				Expect(applRow.Managed_environment_id).ToNot(BeEmpty())

				managedEnv := db.ManagedEnvironment{
					Managedenvironment_id: applRow.Managed_environment_id,
				}

				Expect(dbQueries.GetManagedEnvironmentById(ctx, &managedEnv)).To(Succeed())

				Expect(managedEnv.Name).To(Equal(gitopsDeplManagedEnv.Name))

			})
		})

		When("the application event loop runner receives a status tick event for a GitOpsDeployment", func() {

			It("verifies the event is processed", func() {

				By("creating the GitOpsDeploymentManagedEnvironment CR and the corresponding Secret")
				gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-gitops-depl",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/abc-org/abc-repo",
							Path:           "/abc-path",
							TargetRevision: "abc-commit"},
						Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual,
						Destination: managedgitopsv1alpha1.ApplicationDestination{},
					},
				}

				Expect(k8sClient.Create(ctx, gitopsDepl)).To(Succeed())

				informWorkCompleteChan := make(chan RequestMessage)

				gitopsDeploymentName := gitopsDepl.Name
				gitopsDeploymentNamespace := gitopsDepl.Namespace
				namespaceID := (string)(namespace.UID)

				fakeFactory := MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				}

				inputChannel := startNewApplicationEventLoopRunner(ctx, informWorkCompleteChan, sharedResourceEventLoop, gitopsDeploymentName, gitopsDeploymentNamespace, namespaceID, "", fakeFactory)

				By("creating a go routine that will mark Operations as completed, to simulate cluster-agent")
				go func() {

					defer GinkgoRecover()

					for {

						// End the goroutine on cancel
						select {
						case <-ctx.Done():
							return
						default:
						}

						applId := findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
						if applId == "" {
							fmt.Println("waitin for application for gitopsdepl")
							time.Sleep(500 * time.Millisecond)
							continue
						}

						var operations []db.Operation

						Expect(dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, applId, db.OperationResourceType_Application, &operations, clusterUser.Clusteruser_id)).To(Succeed())

						match := false
						for _, operation := range operations {

							if operation.State == db.OperationState_Completed { // skip already completed
								continue
							}

							operation.State = db.OperationState_Completed

							Expect(dbQueries.UpdateOperation(ctx, &operation)).To(Succeed())

							fmt.Println("Updated Application operation to Complete", operation.Operation_id)

							match = true

						}

						if !match {
							fmt.Println("waiting for Application operations", len(operations))
							time.Sleep(500 * time.Millisecond)
						}

					}
				}()

				By("sending a standard event notification for the GitOpsDeployment")

				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDepl.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: namespaceID,
				}

				resp := <-informWorkCompleteChan
				Expect(resp.Message.Event).ToNot(BeNil())
				Expect(resp.Message.Event.EventType).To(Equal(eventlooptypes.DeploymentModified))

				By("creating the ApplicationState row for the GitOpsDeployment, so we can send the tick event for it")
				Eventually(func() bool {
					applId := findDBApplicationIDforGitOpsDeployment(ctx, gitopsDeploymentName, gitopsDeploymentNamespace, *namespace)
					if applId == "" {
						fmt.Println("waiting for application for gitopsdepl")
						return false
					}

					appStatus := &fauxargocd.FauxApplicationStatus{
						Health: fauxargocd.HealthStatus{
							Status:  fauxargocd.HealthStatusHealthy,
							Message: "Success",
						},
						Resources: []fauxargocd.ResourceStatus{},
						Sync: fauxargocd.SyncStatus{
							Status:   fauxargocd.SyncStatusCodeSynced,
							Revision: "abcdefg",
							ComparedTo: fauxargocd.FauxComparedTo{
								Source: fauxargocd.ApplicationSource{
									RepoURL:        gitopsDepl.Spec.Source.RepoURL,
									Path:           gitopsDepl.Spec.Source.Path,
									TargetRevision: gitopsDepl.Spec.Source.TargetRevision,
								},
								Destination: fauxargocd.ApplicationDestination{},
							},
						},
					}

					appStatusBytes, err := sharedutil.CompressObject(appStatus)
					Expect(err).ToNot(HaveOccurred())

					Expect(dbQueries.CreateApplicationState(ctx, &db.ApplicationState{
						Applicationstate_application_id: applId,
						ArgoCD_Application_Status:       appStatusBytes,
					})).To(Succeed())

					return true

				}, "20s", "200ms").Should(BeTrue())

				By("sending the tick event")
				inputChannel <- eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.UpdateDeploymentStatusTick,
					Request: ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDepl.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: namespaceID,
				}

				Eventually(func() bool {

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)).To(Succeed())

					fmt.Println("waiting for status of GitOpsDeployment to be updated")

					return gitopsDepl.Status.ReconciledState.Source.RepoURL == gitopsDepl.Spec.Source.RepoURL

				}, "20s", "1s").Should(BeTrue(), "the .status field of the GitOpsDeployment should be updated due to the processing of the tick event")
			})
		})

	})

})

var _ = Describe("GitOpsDeployment Conditions", func() {
	var (
		adapter          *gitOpsDeploymentAdapter
		mockCtrl         *gomock.Controller
		mockClient       *mocks.MockClient
		mockStatusWriter *mocks.MockStatusWriter
		gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment
		mockConditions   *condition.MockConditions
		ctx              context.Context
	)

	BeforeEach(func() {
		gitopsDeployment = testStructs.NewGitOpsDeploymentBuilder().Initialized().GetGitopsDeployment()
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockConditions = condition.NewMockConditions(mockCtrl)
		mockStatusWriter = mocks.NewMockStatusWriter(mockCtrl)
	})
	JustBeforeEach(func() {
		adapter = newGitOpsDeploymentAdapter(gitopsDeployment, log.Log.WithName("Test Logger"), mockClient, mockConditions, ctx)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("setGitopsDeploymentCondition()", func() {
		var (
			userDevErr    = gitopserrors.NewUserDevError("reconcile error", fmt.Errorf("reconcile error"))
			reason        = managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileError")
			conditionType = managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred
		)
		Context("when no conditions defined before and the err is nil", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(false)
			})
			It("It returns nil ", func() {
				errTemp := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(errTemp).ToNot(HaveOccurred())
			})
		})
		Context("when the err comes from reconcileHandler", func() {
			It("should update the CR", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(gomock.Any(), conditionType,
					managedgitopsv1alpha1.GitOpsConditionStatusTrue, reason, userDevErr.UserError()).Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, userDevErr)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("when the err has been resolved", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(true)
				mockConditions.EXPECT().FindCondition(gomock.Any(), conditionType).Return(&managedgitopsv1alpha1.GitOpsDeploymentCondition{}, true)
			})
			It("It should update the CR condition status as resolved", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				conditions := &gitopsDeployment.Status.Conditions
				*conditions = append(*conditions, managedgitopsv1alpha1.GitOpsDeploymentCondition{})
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(conditions, conditionType, managedgitopsv1alpha1.GitOpsConditionStatusFalse, managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileErrorResolved"), "").Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})

type OperationCheck struct {
	operationEvents []managedgitopsv1alpha1.Operation
}

func (oc *OperationCheck) ReceiveEvent(event util.ProxyClientEvent) {

	if event.Obj == nil {
		return
	}

	operation, ok := (*event.Obj).(*managedgitopsv1alpha1.Operation)
	if ok {
		oc.operationEvents = append(oc.operationEvents, *operation)
	}
}

var _ = Describe("application_event_runner_deployments.go Tests", func() {

	Context("testing handling of GitOpsDeployments that reference managed environments", func() {

		var ctx context.Context
		var scheme *runtime.Scheme
		var err error

		var namespace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var kubesystemNamespace *corev1.Namespace

		operationCheck := &OperationCheck{}

		k8sClient := &util.ProxyClient{
			Informer: operationCheck,
		}

		// createManagedEnv creates a managed environment DB row and CR, connected via APICRToDBMApping
		createManagedEnv := func() (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
					ClusterCredentialsSecret: "fake-secret-name",
					CreateNewServiceAccount:  true,
				},
			}
			err = k8sClient.Create(ctx, &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			kubeConfigContents := generateFakeKubeConfig()
			managedEnvSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      managedEnvCR.Spec.ClusterCredentialsSecret,
					Namespace: namespace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					shared_resource_loop.KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}
			err = k8sClient.Create(ctx, &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())

			return managedEnvCR, managedEnvSecret
		}

		// Create a simple GitOpsDeployment CR
		createGitOpsDepl := func() *managedgitopsv1alpha1.GitOpsDeployment {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/abc-org/abc-repo",
						Path:           "/abc-path",
						TargetRevision: "abc-commit"},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
				},
			}
			err = k8sClient.Create(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			return gitopsDepl
		}

		// getManagedEnvironmentForGitOpsDeployment returns the managed environment row that is reference by the CR
		getManagedEnvironmentForGitOpsDeployment := func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) (db.ManagedEnvironment, db.Application, error) {
			dtam := []db.DeploymentToApplicationMapping{}
			if err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, gitopsDepl.Name, gitopsDepl.Namespace,
				string(namespace.UID), &dtam); err != nil {
				return db.ManagedEnvironment{}, db.Application{}, err
			}
			Expect(dtam).To(HaveLen(1))

			application := db.Application{
				Application_id: dtam[0].Application_id,
			}
			if err := dbQueries.GetApplicationById(ctx, &application); err != nil {
				return db.ManagedEnvironment{}, db.Application{}, err
			}

			managedEnvRow := db.ManagedEnvironment{
				Managedenvironment_id: application.Managed_environment_id,
			}
			if err := dbQueries.GetManagedEnvironmentById(ctx, &managedEnvRow); err != nil {
				return db.ManagedEnvironment{}, db.Application{}, err
			}
			return managedEnvRow, application, nil
		}

		listOperationRowsForResource := func(resourceId string, resourceType db.OperationResourceType) ([]db.Operation, error) {
			operations := []db.Operation{}
			if err := dbQueries.UnsafeListAllOperations(ctx, &operations); err != nil {
				return []db.Operation{}, err
			}

			res := []db.Operation{}

			for idx := range operations {
				operation := operations[idx]
				if operation.Resource_id == resourceId && operation.Resource_type == resourceType {
					res = append(res, operation)
				}
			}
			return res, nil
		}

		// findManagedEnvironmentRowFromCR locates the ManagedEnvironment row that corresponds to the ManagedEnvironment CR
		findManagedEnvironmentRowFromCR := func(managedEnvCR managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment) (string, error) {

			apiCRToDBMapping := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:       string(managedEnvCR.UID),
				APIResourceName:      managedEnvCR.Name,
				APIResourceNamespace: managedEnvCR.Namespace,
				NamespaceUID:         string(namespace.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
			}
			if err := dbQueries.GetDatabaseMappingForAPICR(ctx, &apiCRToDBMapping); err != nil {
				return "", err
			}

			return apiCRToDBMapping.DBRelationKey, nil
		}

		BeforeEach(func() {
			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			k8sClient.InnerClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, argocdNamespace, kubesystemNamespace).Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, _, err = db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

		})

		It("reconciles a GitOpsDeployment that references a managed environment, then delete the managed env and reconciles", func() {

			managedEnvCR, secretManagedEnv := createManagedEnv()

			By("Creating a GitOpsDeployment pointing to our ManagedEnvironment CR")
			gitopsDepl := createGitOpsDepl()
			gitopsDepl.Spec.Destination.Environment = managedEnvCR.Name
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnvCR.UID), k8sClient)

			mockK8sClientFactory := MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			appEventLoopRunnerAction := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 string(namespace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory:            mockK8sClientFactory,
			}

			canShutdown, appFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(canShutdown).To(BeFalse())
			Expect(appFromCall).ToNot(BeNil())
			Expect(engineInstanceFromCall).ToNot(BeNil())
			Expect(appFromCall.Managed_environment_id).ToNot(BeEmpty())
			Expect(userDevErr).To(BeNil())

			By("locating the ManagedEnvironment row that is associated with the ManagedEnvironment CR")
			managedEnvRowFromAPICRToDBMapping, err := findManagedEnvironmentRowFromCR(managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the Application row of the GitOpsDeployment matches the Managed Environment row of the ManagedEnv CR")
			managedEnvRow, application, err := getManagedEnvironmentForGitOpsDeployment(*gitopsDepl)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnvRow.Managedenvironment_id).To(Equal(managedEnvRowFromAPICRToDBMapping),
				"the managed env from the GitOpsDeployment CR should match the one from the ManagedEnvironment CR")
			Expect(application.Application_id).To(Equal(appFromCall.Application_id),
				"the application object returned from the function call should match the GitOpsDeployment CR we created")

			By("ensuring an Operation was created for the Application")
			applicationOperations, err := listOperationRowsForResource(application.Application_id, "Application")
			Expect(err).ToNot(HaveOccurred())

			Expect(applicationOperations).To(HaveLen(1))

			applicationOperationFound := false

			for _, operation := range operationCheck.operationEvents {
				if operation.Spec.OperationID == applicationOperations[0].Operation_id {
					applicationOperationFound = true
				}
			}
			Expect(applicationOperationFound).To(BeTrue())

			err = k8sClient.Delete(ctx, &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, &secretManagedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("calling handleDeploymentModified again, after deleting the managed environent and secret")
			canShutdown, appFromSecondCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(canShutdown).To(BeFalse())
			Expect(appFromCall).ToNot(BeNil())
			Expect(engineInstanceFromCall).ToNot(BeNil())
			Expect(userDevErr).To(BeNil())
			Expect(appFromSecondCall.Application_id).To(Equal(appFromCall.Application_id))

			Expect(appFromSecondCall.Managed_environment_id).To(BeEmpty(),
				"if the managed environment CR is deleted, the ManagedEnvironment field of the application row should be nil")

		})

		It("reconciles a GitOpsDeployment where the managed environment changes from local to GitOpsDeploymentManagedEnvironment", func() {

			By("Creating a GitOpsDeployment targeting the API namespace")
			gitopsDepl := createGitOpsDepl()
			mockK8sClientFactory := MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			appEventLoopRunnerAction := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 string(namespace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory:            mockK8sClientFactory,
			}

			canShutdown, originalAppFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(canShutdown).To(BeFalse())
			Expect(originalAppFromCall).ToNot(BeNil())
			Expect(engineInstanceFromCall).ToNot(BeNil())

			Expect(userDevErr).To(BeNil())

			By("ensuring an Operation was created for the Application")
			applicationOperations, err := listOperationRowsForResource(originalAppFromCall.Application_id, "Application")
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationOperations).To(HaveLen(1))

			{
				By("Modifying the GitOpsDeployment to target a GitOpsDeploymentManagedEnvironment CR, instead of targeting the API Namespace")

				managedEnvCR, _ := createManagedEnv()

				gitopsDepl.Spec.Destination.Environment = managedEnvCR.Name
				err = k8sClient.Update(ctx, gitopsDepl)
				Expect(err).ToNot(HaveOccurred())

				By("calling handleDeploymentModified with the changed GitOpsDeployment")
				eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnvCR.UID), k8sClient)
				canShutdown, appFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
				Expect(canShutdown).To(BeFalse())
				Expect(appFromCall).ToNot(BeNil())
				Expect(engineInstanceFromCall).ToNot(BeNil())
				Expect(userDevErr).To(BeNil())
				Expect(appFromCall.Application_id).To(Equal(originalAppFromCall.Application_id))

				Expect(appFromCall.Managed_environment_id).ToNot(Equal(originalAppFromCall.Managed_environment_id),
					"the managed environment on the Application row should have changed.")

				By("locating the ManagedEnvironment row that is associated with the ManagedEnvironment CR")
				managedEnvRowFromAPICRToDBMapping, err := findManagedEnvironmentRowFromCR(managedEnvCR)
				Expect(err).ToNot(HaveOccurred())

				By("ensuring the Application row of the GitOpsDeployment matches the Managed Environment row of the ManagedEnv CR")
				managedEnvRow, application, err := getManagedEnvironmentForGitOpsDeployment(*gitopsDepl)
				Expect(err).ToNot(HaveOccurred())
				Expect(managedEnvRow.Managedenvironment_id).To(Equal(managedEnvRowFromAPICRToDBMapping),
					"the managed env from the GitOpsDeployment CR should match the one from the ManagedEnvironment CR")
				Expect(application.Application_id).To(Equal(appFromCall.Application_id),
					"the application object returned from the function call should match the GitOpsDeployment CR we created")

				applicationOperations, err := listOperationRowsForResource(appFromCall.Application_id, "Application")
				Expect(err).ToNot(HaveOccurred())
				Expect(applicationOperations).To(HaveLen(2), "a new operation targeting the Application should have been created for the Application")
			}

		})

		It("reconciles a GitOpsDeployment where the managed environment changes from GitOpsDeploymentManagedEnvironment to local", func() {

			managedEnvCR, _ := createManagedEnv()

			By("Creating a GitOpsDeployment pointing to our ManagedEnvironment CR")
			gitopsDepl := createGitOpsDepl()
			gitopsDepl.Spec.Destination.Environment = managedEnvCR.Name
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnvCR.UID), k8sClient)

			mockK8sClientFactory := MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			appEventLoopRunnerAction := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 string(namespace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory:            mockK8sClientFactory,
			}

			canShutdown, originalAppFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.
				applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(canShutdown).To(BeFalse())
			Expect(originalAppFromCall).ToNot(BeNil())
			Expect(engineInstanceFromCall).ToNot(BeNil())
			Expect(userDevErr).To(BeNil())
			applicationOperations, err := listOperationRowsForResource(originalAppFromCall.Application_id, "Application")
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationOperations).To(HaveLen(1))

			By("modifying the GitOpsDeployment CR to target the local namespace, rather than a ManagedEnvironment CR")
			gitopsDepl.Spec.Destination.Environment = ""
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("calling handleDeploymentModified again, now that we have updated the GitOpsDeployment")

			canShutdown, appFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.
				applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(canShutdown).To(BeFalse())
			Expect(appFromCall).ToNot(BeNil())
			Expect(appFromCall.Application_id).To(Equal(originalAppFromCall.Application_id))
			Expect(engineInstanceFromCall).ToNot(BeNil())
			Expect(userDevErr).To(BeNil())

			Expect(appFromCall.Managed_environment_id).ToNot(Equal(originalAppFromCall.Managed_environment_id))

			By("ensuring an Operation was created for the Application")
			applicationOperations, err = listOperationRowsForResource(appFromCall.Application_id, "Application")
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationOperations).To(HaveLen(2),
				"a second Operation should have been created, since the Application should have changed")

		})

		It("reconciles a GitOpsDeployment where the managed environment changes from one GitOpsDeploymentManagedEnvironment to a second GitOpsDeploymentManagedEnvironment", func() {

			managedEnvCR, _ := createManagedEnv()

			By("Creating a GitOpsDeployment pointing to our ManagedEnvironment CR")
			gitopsDepl := createGitOpsDepl()
			gitopsDepl.Spec.Destination.Environment = managedEnvCR.Name
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnvCR.UID), k8sClient)

			mockK8sClientFactory := MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			appEventLoopRunnerAction := applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 string(namespace.UID),
				testOnlySkipCreateOperation: true,
				k8sClientFactory:            mockK8sClientFactory,
			}

			canShutdown, originalAppFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(canShutdown).To(BeFalse())
			Expect(originalAppFromCall).ToNot(BeNil())
			Expect(engineInstanceFromCall).ToNot(BeNil())
			Expect(userDevErr).To(BeNil())
			applicationOperations, err := listOperationRowsForResource(originalAppFromCall.Application_id, "Application")
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationOperations).To(HaveLen(1))

			By("creating a second GitOpsDeploymentManagedEnvironment")
			var managedEnvCR2 managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment
			{
				kubeConfigContents := generateFakeKubeConfig()
				managedEnvSecret2 := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster-secret-2",
						Namespace: namespace.Name,
					},
					Type: sharedutil.ManagedEnvironmentSecretType,
					Data: map[string][]byte{
						shared_resource_loop.KubeconfigKey: ([]byte)(kubeConfigContents),
					},
				}
				err = k8sClient.Create(ctx, &managedEnvSecret2)
				Expect(err).ToNot(HaveOccurred())

				managedEnvCR2 = managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-managed-env-2",
						Namespace: namespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
						APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
						ClusterCredentialsSecret: managedEnvSecret2.Name,
						CreateNewServiceAccount:  true,
					},
				}
				err = k8sClient.Create(ctx, &managedEnvCR2)
				Expect(err).ToNot(HaveOccurred())

				eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnvCR2.UID), k8sClient)

			}

			By("updating the GitOpsDeployment to point to the new ManagedEnvironment")
			gitopsDepl.Spec.Destination.Environment = managedEnvCR2.Name
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("calling handleDeploymentModified again, now that we have updated the GitOpsDeployment")
			canShutdown, appFromCall, engineInstanceFromCall, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(canShutdown).To(BeFalse())
			Expect(appFromCall).ToNot(BeNil())
			Expect(appFromCall.Application_id).To(Equal(originalAppFromCall.Application_id))
			Expect(engineInstanceFromCall).ToNot(BeNil())
			Expect(userDevErr).To(BeNil())
			Expect(appFromCall.Managed_environment_id).ToNot(Equal(originalAppFromCall.Managed_environment_id))

			By("locating the ManagedEnvironment row that is associated with the new ManagedEnvironment CR")
			managedEnvRowFromAPICRToDBMapping2, err := findManagedEnvironmentRowFromCR(managedEnvCR2)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the Application row of the GitOpsDeployment matches the Managed Environment row of the new ManagedEnv CR")
			managedEnvRow2, application, err := getManagedEnvironmentForGitOpsDeployment(*gitopsDepl)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnvRow2.Managedenvironment_id).To(Equal(managedEnvRowFromAPICRToDBMapping2),
				"the managed env from the GitOpsDeployment CR should match the one from the new ManagedEnvironment CR")
			Expect(application.Application_id).To(Equal(appFromCall.Application_id),
				"the application object returned from the function call should match the GitOpsDeployment CR we created")

			applicationOperations, err = listOperationRowsForResource(appFromCall.Application_id, "Application")
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationOperations).To(HaveLen(2),
				"a new operation targeting the Application should have been created for the Application")

		})
	})
})

type MockSRLK8sClientFactory struct {
	fakeClient client.Client
}

func (f MockSRLK8sClientFactory) BuildK8sClient(restConfig *rest.Config) (client.Client, error) {
	return f.fakeClient, nil
}

func (f MockSRLK8sClientFactory) GetK8sClientForGitOpsEngineInstance(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
	return f.fakeClient, nil
}

func (f MockSRLK8sClientFactory) GetK8sClientForServiceWorkspace() (client.Client, error) {
	return f.fakeClient, nil
}

var _ = Describe("Miscellaneous application_event_runner.go tests", func() {

	Context("Test handleManagedEnvironmentModified", func() {

		var ctx context.Context
		var scheme *runtime.Scheme
		var err error

		var namespace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var kubesystemNamespace *corev1.Namespace

		var k8sClient client.Client

		var clusterCredentials *db.ClusterCredentials
		var engineInstance *db.GitopsEngineInstance

		// createManagedEnv creates a managed environment DB row and CR, connected via APICRToDBMApping
		createManagedEnv := func() (db.ManagedEnvironment, managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, db.APICRToDatabaseMapping) {

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api-url",
					ClusterCredentialsSecret: "fake-secret-name",
					CreateNewServiceAccount:  true,
				},
			}
			err = k8sClient.Create(ctx, &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Name:                  "my-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			apiCRToDBMapping := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:       string(managedEnvCR.UID),
				APIResourceName:      managedEnvCR.Name,
				APIResourceNamespace: managedEnvCR.Namespace,
				NamespaceUID:         string(namespace.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        managedEnvironment.Managedenvironment_id,
			}
			err = dbQueries.CreateAPICRToDatabaseMapping(ctx, &apiCRToDBMapping)
			Expect(err).ToNot(HaveOccurred())

			return managedEnvironment, managedEnvCR, apiCRToDBMapping
		}

		// Create a simple GitOpsDeployment CR
		createGitOpsDepl := func() *managedgitopsv1alpha1.GitOpsDeployment {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/abc-org/abc-repo",
						Path:           "/abc-path",
						TargetRevision: "abc-commit"},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
				},
			}
			err = k8sClient.Create(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			return gitopsDepl
		}

		BeforeEach(func() {
			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, argocdNamespace, kubesystemNamespace).Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			clusterCredentials, _, _, engineInstance, _, err = db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

		})

		It("return true/false if gitopsDeployment.Spec.Destination.Environment == managedEnvEvent.Request.Name", func() {

			_, managedEnvCR, _ := createManagedEnv()

			By("creating a GitOpsDeployment that initially doesn't reference the ManagedEnvironment CR")
			gitopsDepl := createGitOpsDepl()

			By("creating a new event that references the ManagedEnvironment")
			newEvent := eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.ManagedEnvironmentModified,
				Request: reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: namespace.Name, Name: managedEnvCR.Name},
				},
				Client:      k8sClient,
				ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
				WorkspaceID: string(namespace.UID),
			}

			By("calling the function with a ManagedEnvironment event")
			informGitOpsDepl, err := handleManagedEnvironmentModified_shouldInformGitOpsDeployment(ctx, *gitopsDepl, newEvent, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(informGitOpsDepl).To(BeFalse(), "GitOpsDepl runner should not be informed if the ManagedEnvironment CR doesn't reference the GitOpsDeployment CR")

			By("updating the environment field of GitOpsDeployment, which should change the test result")
			gitopsDepl.Spec.Destination = managedgitopsv1alpha1.ApplicationDestination{
				Environment: managedEnvCR.Name,
				Namespace:   gitopsDepl.Spec.Destination.Namespace,
			}
			informGitOpsDepl, err = handleManagedEnvironmentModified_shouldInformGitOpsDeployment(ctx, *gitopsDepl, newEvent, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(informGitOpsDepl).To(BeTrue(), "GitOpsDepl runner SHOULD be informed if the ManagedEnvironment CR references the GitOpsDeployment CR")

		})

		It("should return true if there are applications that match managed environments, via APICRToDBMapping and DeplToAppMapping", func() {

			managedEnvironment, managedEnvCR, apiCRToDBMapping := createManagedEnv()

			gitopsDepl := createGitOpsDepl()

			By("creating an Application row that references the ManagedEnvironment row")
			application := db.Application{
				Application_id:          "test-my-application",
				Name:                    "application",
				Spec_field:              "{}",
				Engine_instance_inst_id: engineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			By("connecting the Application row to the GitOpsDeployment CR")
			dtam := db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-dtam",
				DeploymentName:                        gitopsDepl.Name,
				DeploymentNamespace:                   gitopsDepl.Namespace,
				NamespaceUID:                          string(namespace.UID),
				Application_id:                        application.Application_id,
			}
			err = dbQueries.CreateDeploymentToApplicationMapping(ctx, &dtam)
			Expect(err).ToNot(HaveOccurred())

			By("calling the function with a ManagedEnvironment event")
			newEvent := eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.ManagedEnvironmentModified,
				Request: reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: namespace.Name, Name: managedEnvCR.Name},
				},
				Client:      k8sClient,
				ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
				WorkspaceID: string(namespace.UID),
			}

			informGitOpsDepl, err := handleManagedEnvironmentModified_shouldInformGitOpsDeployment(ctx, *gitopsDepl, newEvent, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(informGitOpsDepl).To(BeTrue(), "when the Application DB row references the corresponding ManagedEnv row, it should return true")

			By("deleting the connection from the ManagedEnv CR to the ManagedEnv row")
			rowsDeleted, err := dbQueries.DeleteAPICRToDatabaseMapping(ctx, &apiCRToDBMapping)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsDeleted).To(Equal(1))

			By("calling the function with a ManagedEnvironment event, but this time we expect a different result")
			informGitOpsDepl, err = handleManagedEnvironmentModified_shouldInformGitOpsDeployment(ctx, *gitopsDepl, newEvent, dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(informGitOpsDepl).To(BeFalse(), "when function can't locate the ManagedEnvironment row from the CR, it should return false")

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
      server: https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api2.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api2-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
contexts:
  - context:
      cluster: api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
  - context:
      cluster: api2-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api2-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api2-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
current-context: default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
preferences: {}
users:
  - name: kube:admin/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      token: sha256~ABCdEF1gHiJKlMnoP-Q19qrTuv1_W9X2YZABCDefGH4
  - name: kube:admin/api2-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      token: sha256~abcDef1gHIjkLmNOp-q19QRtUV1_w9x2yzabcdEFgh4
`
}

func dummyApplicationComparedToField() (string, fauxargocd.FauxComparedTo, error) {

	fauxcomparedTo := fauxargocd.FauxComparedTo{
		Source: fauxargocd.ApplicationSource{
			RepoURL:        "test-url",
			Path:           "test-path",
			TargetRevision: "test-branch",
		},
		Destination: fauxargocd.ApplicationDestination{
			Namespace: "test-namespace",
			Name:      "",
		},
	}

	fauxcomparedToBytes, err := json.Marshal(fauxcomparedTo)
	if err != nil {
		return "", fauxcomparedTo, err
	}

	return string(fauxcomparedToBytes), fauxcomparedTo, nil
}

func testTeardown() {
	err := db.SetupForTestingDBGinkgo()
	Expect(err).ToNot(HaveOccurred())
}
