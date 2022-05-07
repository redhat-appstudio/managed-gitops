package application_event_loop

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1/mocks"
	condition "github.com/redhat-appstudio/managed-gitops/backend/condition/mocks"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"gopkg.in/yaml.v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	testStructs "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1/mocks/structs"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("ApplicationEventLoop Test", func() {

	Context("Handle deployment modified", func() {
		var err error
		var workspaceID string
		var ctx context.Context
		var scheme *runtime.Scheme
		var workspace *v1.Namespace
		var argocdNamespace *v1.Namespace
		var dbQueries db.AllDatabaseQueries
		var k8sClientOuter client.WithWatch
		var k8sClient *sharedutil.ProxyClient
		var kubesystemNamespace *v1.Namespace
		var informer sharedutil.ListEventReceiver
		var gitopsDepl *managedgitopsv1alpha1.GitOpsDeployment
		var appEventLoopRunnerAction applicationEventLoopRunner_Action

		BeforeEach(func() {
			ctx = context.Background()
			informer = sharedutil.ListEventReceiver{}

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err = eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			workspaceID = string(workspace.UID)

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

			k8sClientOuter = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}
		})

		It("Should update existing deployment, instead of creating new.", func() {

			// ----------------------------------------------------------------------------
			By("Create new deployment.")
			// ----------------------------------------------------------------------------
			var message deploymentModifiedResult
			_, _, _, message, err = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(err).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			// ----------------------------------------------------------------------------
			By("Verify that database entries are created.")
			// ----------------------------------------------------------------------------

			var appMappingsFirst []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsFirst)

			Expect(err).To(BeNil())
			Expect(len(appMappingsFirst)).To(Equal(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)

			Expect(err).To(BeNil())

			//############################################################################

			// ----------------------------------------------------------------------------
			By("Update existing deployment.")
			// ----------------------------------------------------------------------------

			gitopsDepl.Spec.Source.Path = "/def-path"
			gitopsDepl.Spec.Source.RepoURL = "https://github.com/def-org/def-repo"
			gitopsDepl.Spec.Source.TargetRevision = "def-commit"
			gitopsDepl.Spec.Destination.Namespace = "def-namespace"

			// Create new client and application runner, but pass existing gitOpsDeployment object.
			k8sClientOuter = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()
			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			appEventLoopRunnerActionSecond := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ----------------------------------------------------------------------------
			By("This should update the existing application.")
			// ----------------------------------------------------------------------------

			_, _, _, message, err = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Updated))

			// ----------------------------------------------------------------------------
			By("Verify that the database entries have been updated.")
			// ----------------------------------------------------------------------------

			var appMappingsSecond []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsSecond)

			Expect(err).To(BeNil())
			Expect(len(appMappingsSecond)).To(Equal(1))

			deplToAppMappingSecond := appMappingsSecond[0]
			applicationSecond := db.Application{Application_id: deplToAppMappingSecond.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationSecond)

			Expect(err).To(BeNil())
			Expect(applicationFirst.SeqID).To(Equal(applicationSecond.SeqID))
			Expect(applicationFirst.Spec_field).NotTo(Equal(applicationSecond.Spec_field))

			clusterUser := db.ClusterUser{User_name: string(workspace.UID)}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).To(BeNil())

			gitopsEngineInstance := db.GitopsEngineInstance{Gitopsengineinstance_id: applicationSecond.Engine_instance_inst_id}
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			managedEnvironment := db.ManagedEnvironment{Managedenvironment_id: applicationSecond.Managed_environment_id}
			err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).To(BeNil())

			//############################################################################

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDepl and verify that the corresponding DB entries are removed.")
			// ----------------------------------------------------------------------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, message, err = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Deleted))

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &applicationSecond)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).To(BeNil())
			Expect(len(appMappings)).To(Equal(0))

			// GitopsEngine instance should still be reachable
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			operationCreated := false
			operationDeleted := false
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

		It("Should not deploy application, as request data is not valid.", func() {

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// This should fail while creating new application
			_, _, _, _, err = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).NotTo(BeNil())

			// ----------------------------------------------------------------------------
			By("Verify that the database entry is not created.")
			// ----------------------------------------------------------------------------

			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)

			Expect(err).To(BeNil())
			Expect(len(appMappings)).To(Equal(0))
		})

		It("Should not update existing deployment, if no changes were done in fields.", func() {
			// This should create new application
			var message deploymentModifiedResult
			_, _, _, message, err = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(err).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			// ----------------------------------------------------------------------------
			By("Verify that the database entries have been created.")
			// ----------------------------------------------------------------------------

			var appMappingsFirst []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsFirst)

			Expect(err).To(BeNil())
			Expect(len(appMappingsFirst)).To(Equal(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)
			Expect(err).To(BeNil())

			//--------------------------------------------------------------------------------------
			// Now create new client and event loop runner object.
			//--------------------------------------------------------------------------------------

			k8sClientOuter = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			appEventLoopRunnerActionSecond := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			//--------------------------------------------------------------------------------------
			// Pass same gitOpsDeployment again, but no changes should be done in the application.
			//--------------------------------------------------------------------------------------
			_, _, _, message, err = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(err).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_NoChange))

			//############################################################################

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDepl and verify that the corresponding DB entries are removed.")
			// ----------------------------------------------------------------------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, message, err = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Deleted))

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &applicationFirst)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).To(BeNil())
			Expect(len(appMappings)).To(Equal(0))

			// GitopsEngine instance should still be reachable
			gitopsEngineInstance := db.GitopsEngineInstance{Gitopsengineinstance_id: applicationFirst.Engine_instance_inst_id}
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			operationCreated := false
			operationDeleted := false
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

		It("create a deployment and ensure it processed, then delete it an ensure that is processed", func() {
			_, _, _, _, err = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// Verify that the database entries have been created -----------------------------------------

			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).To(BeNil())

				Expect(len(appMappings)).To(Equal(1))

				deplToAppMapping = appMappings[0]
			}

			clusterUser := db.ClusterUser{
				User_name: string(workspace.UID),
			}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).To(BeNil())

			application := db.Application{
				Application_id: deplToAppMapping.Application_id,
			}

			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).To(BeNil())

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: application.Engine_instance_inst_id,
			}

			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: application.Managed_environment_id,
			}
			err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).To(BeNil())

			// Delete the GitOpsDepl and verify that the corresponding DB entries are removed -------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, _, err = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &application)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).To(BeNil())
			Expect(len(appMappings)).To(Equal(0))

			// GitopsEngine instance should still be reachable
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			operatorCreated := false
			operatorDeleted := false

			for idx, event := range informer.Events {

				if event.Action == sharedutil.Create && event.ObjectTypeOf() == "Operation" {
					operatorCreated = true
				}
				if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "Operation" {
					operatorDeleted = true
				}

				fmt.Printf("%d) %v\n", idx, event)
			}

			Expect(operatorCreated).To(BeTrue())
			Expect(operatorDeleted).To(BeTrue())

		})

		It("create an invalid deployment and ensure it fails.", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("abc", 100),
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ------

			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).NotTo(BeNil())

		})

	})

	Context("Handle sync run modified", func() {

		It("Ensure the sync run handler can handle a new sync run resource", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
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
			Expect(err).To(BeNil())

			opts := zap.Options{
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()

			// 1) send a deployment modified event, to ensure the deployment is added to the database, and processed
			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     sharedResourceLoop,
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
			}
			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// 2) add a sync run modified event, to ensure the sync run is added to the database, and processed
			a = applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:       gitopsDeplSyncRun.Name,
				eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
				workspaceClient:         k8sClient,
				log:                     log.FromContext(context.Background()),
				sharedResourceEventLoop: sharedResourceLoop,
				workspaceID:             a.workspaceID,
			}
			_, err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Ensure the sync run handler fails when an invalid new sync run resource is passed.", func() {
			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
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
			Expect(err).To(BeNil())

			opts := zap.Options{
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()

			// 1) send a deployment modified event, to ensure the deployment is added to the database, and processed
			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     sharedResourceLoop,
				workspaceID:                 string(workspace.UID),
				testOnlySkipCreateOperation: true,
			}
			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// 2) add a sync run modified event, to ensure the sync run is added to the database, and processed
			a = applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:       gitopsDeplSyncRun.Name,
				eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
				workspaceClient:         k8sClient,
				log:                     log.FromContext(context.Background()),
				sharedResourceEventLoop: sharedResourceLoop,
				workspaceID:             a.workspaceID,
			}
			_, err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
			Expect(err).NotTo(BeNil())

		})
	})

	Context("Handle deployment Health/Sync status.", func() {
		var err error
		var workspaceID string
		var ctx context.Context
		var scheme *runtime.Scheme
		var workspace *v1.Namespace
		var argocdNamespace *v1.Namespace
		var dbQueries db.AllDatabaseQueries
		var kubesystemNamespace *v1.Namespace

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err = eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			workspaceID = string(workspace.UID)

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())
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
			}

			k8sClient := fake.
				NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			a := applicationEventLoopRunner_Action{
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Get DeploymentToApplicationMapping and Application objects, to be used later.")
			// ----------------------------------------------------------------------------
			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).To(BeNil())

				Expect(len(appMappings)).To(Equal(1))

				deplToAppMapping = appMappings[0]
			}

			// ----------------------------------------------------------------------------
			By("Inserting dummy data into ApplicationState table, because we are not calling the Reconciler for this, which updates the status of application into db.")
			// ----------------------------------------------------------------------------
			var resourceStatus managedgitopsv1alpha1.ResourceStatus
			var resources []managedgitopsv1alpha1.ResourceStatus
			resources = append(resources, resourceStatus)

			var buffer bytes.Buffer
			// Convert ResourceStatus object into String.
			resourceStr, err := yaml.Marshal(&resources)
			Expect(err).To(BeNil())

			// Compress the data
			gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
			Expect(err).To(BeNil())

			_, err = gzipWriter.Write([]byte(string(resourceStr)))
			Expect(err).To(BeNil())

			err = gzipWriter.Close()
			Expect(err).To(BeNil())

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: deplToAppMapping.Application_id,
				Health:                          string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				Sync_Status:                     string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				Revision:                        "abcdefg",
				Message:                         "Success",
				Resources:                       buffer.Bytes(),
			}

			err = dbQueries.CreateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync before calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)

			Expect(clientErr).To(BeNil())

			Expect(gitopsDeployment.Status.Health.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Revision).To(BeEmpty())
			Expect(gitopsDeployment.Status.Health.Message).To(BeEmpty())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status.")
			// ----------------------------------------------------------------------------

			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, string(gitopsDepl.UID), dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync after calling applicationEventRunner_handleUpdateDeploymentStatusTick function.")
			// ----------------------------------------------------------------------------

			clientErr = a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)

			Expect(clientErr).To(BeNil())

			Expect(gitopsDeployment.Status.Health.Status).To(Equal(managedgitopsv1alpha1.HeathStatusCodeHealthy))
			Expect(gitopsDeployment.Status.Sync.Status).To(Equal(managedgitopsv1alpha1.SyncStatusCodeSynced))
			Expect(gitopsDeployment.Status.Sync.Revision).To(Equal("abcdefg"))
			Expect(gitopsDeployment.Status.Health.Message).To(Equal("Success"))

			// ----------------------------------------------------------------------------
			By("Delete GitOpsDepl to clean resources.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Should not return any error, if deploymentToApplicationMapping doesn't exist for given gitopsdeployment.", func() {
			// Don't create Deployment and resources by calling applicationEventRunner_handleDeploymentModified function,
			// but check for Health/Sync status of deployment which doesn't exist.
			k8sClientOuter := fake.NewClientBuilder().Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			a := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           "dummy-deployment",
				eventResourceNamespace:      workspace.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status of a deployment which doesn't exist.")
			// ----------------------------------------------------------------------------
			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, "dummy-deployment", dbQueries)
			Expect(err).To(BeNil())
		})

		It("Should return error, if deploymentToApplicationMapping object is invalid.", func() {
			// Don't create Deployment and resources by calling applicationEventRunner_handleDeploymentModified function,
			// But check for Health/Sync status of a deployment having invalid name.
			k8sClientOuter := fake.NewClientBuilder().Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			a := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           "dummy-deployment",
				eventResourceNamespace:      workspace.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status of a deployment having invalid name.")
			// ----------------------------------------------------------------------------

			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, "", dbQueries)
			Expect(err).NotTo(BeNil())
			Expect(strings.Contains(err.Error(), "field should not be empty string")).To(BeTrue())
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
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Delete deployment, but we don't want to delete other DB entries, hence not calling applicationEventRunner_handleDeploymentModified after deleting deployment.")
			// ----------------------------------------------------------------------------
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status for a deployment which doesn'r exist in given namespace.")
			// ----------------------------------------------------------------------------
			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, string(gitopsDepl.UID), dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Deployment is already been deleted in previous step, now delete related db entries and clean resources.")
			// ----------------------------------------------------------------------------
			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
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
				// When the code asks for a new k8s client, give it our fake client
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}

			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Call applicationEventRunner_handleUpdateDeploymentStatusTick function to update Health/Sync status for deployment which is missing ApplicationState entries.")
			// ----------------------------------------------------------------------------

			err = a.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, string(gitopsDepl.UID), dbQueries)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Retrieve latest version of GitOpsDeployment and check Health/Sync, it should not have any Health/Sync status.")
			// ----------------------------------------------------------------------------

			gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
			gitopsDeploymentKey := client.ObjectKey{Namespace: gitopsDepl.Namespace, Name: gitopsDepl.Name}
			clientErr := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment)

			Expect(clientErr).To(BeNil())

			Expect(gitopsDeployment.Status.Health.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Status).To(BeEmpty())
			Expect(gitopsDeployment.Status.Sync.Revision).To(BeEmpty())
			Expect(gitopsDeployment.Status.Health.Message).To(BeEmpty())

			// ----------------------------------------------------------------------------
			By("Delete GitOpsDepl to clean resources.")
			// ----------------------------------------------------------------------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			_, _, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(err).To(BeNil())
		})

	})

	Context("Check decompressResourceData function.", func() {
		It("Should decompress resource data and return actual Array of ResourceStatus objects.", func() {
			// ----------------------------------------------------------------------------
			By("Creating sample resource data.")
			// ----------------------------------------------------------------------------

			resourceStatus := managedgitopsv1alpha1.ResourceStatus{
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "argoCD",
				Name:      "component-a",
				Status:    "Synced",
				Health: &managedgitopsv1alpha1.HealthStatus{
					Status:  "Healthy",
					Message: "success",
				},
			}

			var resourcesIn []managedgitopsv1alpha1.ResourceStatus
			resourcesIn = append(resourcesIn, resourceStatus)

			// ----------------------------------------------------------------------------
			By("Convert sample ResourceStatus objects into String.")
			// ----------------------------------------------------------------------------
			resourceStr, err := yaml.Marshal(&resourcesIn)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Compress sample data to be passed as input for decompressResourceData function.")
			// ----------------------------------------------------------------------------
			var buffer bytes.Buffer
			gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)

			Expect(err).To(BeNil())

			_, err = gzipWriter.Write([]byte(string(resourceStr)))
			Expect(err).To(BeNil())

			err = gzipWriter.Close()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Decompress data and convert it to String, then convert String into ResourceStatus Array.")
			// ----------------------------------------------------------------------------

			var resourcesOut []managedgitopsv1alpha1.ResourceStatus

			resourcesOut, err = decompressResourceData(buffer.Bytes())

			Expect(err).To(BeNil())

			Expect(resourcesOut).NotTo(BeNil())
			Expect(resourcesOut).NotTo(BeEmpty())

			Expect(resourcesOut[0]).NotTo(BeNil())
			Expect(resourcesOut[0].Group).To(Equal("apps"))
			Expect(resourcesOut[0].Version).To(Equal("v1"))
			Expect(resourcesOut[0].Kind).To(Equal("Deployment"))
			Expect(resourcesOut[0].Namespace).To(Equal("argoCD"))
			Expect(resourcesOut[0].Status).To(Equal(managedgitopsv1alpha1.SyncStatusCodeSynced))
			Expect(resourcesOut[0].Health.Status).To(Equal(managedgitopsv1alpha1.HeathStatusCodeHealthy))
			Expect(resourcesOut[0].Health.Message).To(Equal("success"))
		})

		It("Should decompress empty resource data and return actual Array of ResourceStatus objects.", func() {
			// ----------------------------------------------------------------------------
			By("Creating sample resource data.")
			// ----------------------------------------------------------------------------
			resourceStatus := managedgitopsv1alpha1.ResourceStatus{}

			var resourcesIn []managedgitopsv1alpha1.ResourceStatus
			resourcesIn = append(resourcesIn, resourceStatus)

			// ----------------------------------------------------------------------------
			By("Convert sample ResourceStatus objects into String.")
			// ----------------------------------------------------------------------------
			resourceStr, err := yaml.Marshal(&resourcesIn)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Compress sample data to be passed as input for decompressResourceData function.")
			// ----------------------------------------------------------------------------
			var buffer bytes.Buffer
			gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)

			Expect(err).To(BeNil())

			_, err = gzipWriter.Write([]byte(string(resourceStr)))
			Expect(err).To(BeNil())

			err = gzipWriter.Close()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Decompress data and convert it to String, then convert String into ResourceStatus Array.")
			// ----------------------------------------------------------------------------

			var resourcesOut []managedgitopsv1alpha1.ResourceStatus

			resourcesOut, err = decompressResourceData(buffer.Bytes())

			Expect(err).To(BeNil())

			Expect(resourcesOut).NotTo(BeNil())
			Expect(resourcesOut).NotTo(BeEmpty())

			Expect(resourcesOut[0]).NotTo(BeNil())
			Expect(managedgitopsv1alpha1.ResourceStatus{} == resourcesOut[0]).To(BeTrue())
		})

		It("Should decompress empty resource data and return empty Array of ResourceStatus objects.", func() {
			// ----------------------------------------------------------------------------
			By("Creating sample resource data.")
			// ----------------------------------------------------------------------------

			var resourcesIn []managedgitopsv1alpha1.ResourceStatus

			// ----------------------------------------------------------------------------
			By("Convert sample ResourceStatus objects into String.")
			// ----------------------------------------------------------------------------
			resourceStr, err := yaml.Marshal(&resourcesIn)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Compress sample data to be passed as input for decompressResourceData function.")
			// ----------------------------------------------------------------------------
			var buffer bytes.Buffer
			gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
			Expect(err).To(BeNil())

			_, err = gzipWriter.Write([]byte(string(resourceStr)))
			Expect(err).To(BeNil())

			err = gzipWriter.Close()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Decompress data and convert it to String, then convert String into ResourceStatus Array.")
			// ----------------------------------------------------------------------------

			var resourcesOut []managedgitopsv1alpha1.ResourceStatus

			resourcesOut, err = decompressResourceData(buffer.Bytes())

			Expect(err).To(BeNil())

			Expect(resourcesOut).NotTo(BeNil())
			Expect(resourcesOut).To(BeEmpty())
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
			err           = errors.New("fake reconcile")
			reason        = managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileError")
			conditionType = managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred
		)
		Context("when no conditions defined before and the err is nil", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(false)
			})
			It("It returns nil ", func() {
				errTemp := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(errTemp).To(BeNil())
			})
		})
		Context("when the err comes from reconcileHandler", func() {
			It("should update the CR", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(gomock.Any(), conditionType, managedgitopsv1alpha1.GitOpsConditionStatusTrue, reason, err.Error()).Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, err)
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
