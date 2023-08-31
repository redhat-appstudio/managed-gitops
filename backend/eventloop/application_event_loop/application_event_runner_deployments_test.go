package application_event_loop

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Application Event Runner Deployments", func() {
	Context("createSpecField should generate a valid argocd Application", func() {
		getFakeArgoCDSpecInput := func(automated, unsanitized bool) argoCDSpecInput {
			input := argoCDSpecInput{
				crName:               "sample-depl",
				crNamespace:          "workspace",
				destinationNamespace: "prod",
				destinationName:      "in-cluster",
				sourceRepoURL:        "https://github.com/test/test",
				sourcePath:           "environments/prod",
				automated:            automated,
				project:              "app-project-cluster-user-id",
			}

			if unsanitized {
				input.sourcePath = "environments/&&prod\n"
				input.sourceRepoURL = "https://github.com/```%test/test"
			}
			return input
		}

		getValidApplication := func(automated bool) string {
			input := getFakeArgoCDSpecInput(automated, false)
			application := fauxargocd.FauxApplication{
				FauxTypeMeta: fauxargocd.FauxTypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				FauxObjectMeta: fauxargocd.FauxObjectMeta{
					Name:      input.crName,
					Namespace: input.crNamespace,
				},
				Spec: fauxargocd.FauxApplicationSpec{
					Source: fauxargocd.ApplicationSource{
						RepoURL:        input.sourceRepoURL,
						Path:           input.sourcePath,
						TargetRevision: input.sourceTargetRevision,
					},
					Destination: fauxargocd.ApplicationDestination{
						Name:      input.destinationName,
						Namespace: input.destinationNamespace,
					},
					Project: "app-project-cluster-user-id",
				},
			}
			if automated {
				application.Spec.SyncPolicy = &fauxargocd.SyncPolicy{
					Automated: &fauxargocd.SyncPolicyAutomated{
						Prune:      true,
						AllowEmpty: true,
						SelfHeal:   true,
					},
					SyncOptions: fauxargocd.SyncOptions{
						prunePropagationPolicy,
					},
					Retry: &fauxargocd.RetryStrategy{
						Limit: -1,
						Backoff: &fauxargocd.Backoff{
							Duration:    "5s",
							Factor:      getInt64Pointer(2),
							MaxDuration: "3m",
						},
					},
				}
			}

			appBytes, err := yaml.Marshal(application)
			if err != nil {
				GinkgoT().Fatalf("failed to unmarshall Application: %q", err)
			}

			return string(appBytes)
		}

		It("Input spec is converted to an argocd Application", func() {
			input := getFakeArgoCDSpecInput(false, false)
			application, err := createSpecField(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(application).To(Equal(getValidApplication(false)))
		})

		It("Sanitize illegal characters from input", func() {
			input := getFakeArgoCDSpecInput(false, true)
			application, err := createSpecField(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(application).To(Equal(getValidApplication(false)))
		})

		It("Input spec with automated enabled should set automated sync policy", func() {
			input := getFakeArgoCDSpecInput(true, false)
			application, err := createSpecField(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(application).To(Equal(getValidApplication(true)))
		})
	})
})

var _ = Describe("Application Event Runner Deployments to check SyncPolicy.SyncOption", func() {
	Context("Handle SyncPolicy.SyncOption in GitopsDeployment for CreateNamespace=true", func() {
		var err error
		var workspaceID string
		var ctx context.Context
		var scheme *runtime.Scheme
		var workspace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var kubesystemNamespace *corev1.Namespace
		var gitopsDepl *managedgitopsv1alpha1.GitOpsDeployment
		var appEventLoopRunnerAction applicationEventLoopRunner_Action

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

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
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
					SyncPolicy: &managedgitopsv1alpha1.SyncPolicy{
						SyncOptions: managedgitopsv1alpha1.SyncOptions{
							managedgitopsv1alpha1.SyncOptions_CreateNamespace_true,
						},
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).ToNot(HaveOccurred())

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
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
		})

		It("Checks whether the CreateNamespace=true SyncOption gets inserted/updated into the spec_field of Application table", func() {

			By("Create new deployment.")

			var message deploymentModifiedResult
			_, _, _, message, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			By("Verify that database entries are created.")

			var appMappingsFirst []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsFirst)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsFirst).To(HaveLen(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)

			Expect(err).ToNot(HaveOccurred())

			Expect(strings.Contains(applicationFirst.Spec_field, string(managedgitopsv1alpha1.SyncOptions_CreateNamespace_true))).To(BeTrue())
			//############################################################################

			By("Update existing deployment so that the SyncOption is set to nil/empty")
			var emptySyncOption []managedgitopsv1alpha1.SyncOption
			gitopsDepl.Spec.SyncPolicy.SyncOptions = emptySyncOption

			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("This should update the existing application.")

			_, _, _, message, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Updated))

			By("Verify that the database entries have been updated.")

			var appMappingsSecond []db.DeploymentToApplicationMapping
			err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsSecond)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsSecond).To(HaveLen(1))

			deplToAppMappingSecond := appMappingsSecond[0]
			applicationSecond := db.Application{Application_id: deplToAppMappingSecond.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationSecond)

			Expect(err).ToNot(HaveOccurred())
			Expect(applicationFirst.SeqID).To(Equal(applicationSecond.SeqID))
			Expect(applicationFirst.Spec_field).NotTo(Equal(applicationSecond.Spec_field))
			Expect(strings.Contains(applicationSecond.Spec_field, string(managedgitopsv1alpha1.SyncOptions_CreateNamespace_true))).To(BeFalse())

			//############################################################################
			By("Update existing deployment to a SyncOption that is not empty and is set to CreateNamespace=true")

			gitopsDepl.Spec.SyncPolicy.SyncOptions = managedgitopsv1alpha1.SyncOptions{
				managedgitopsv1alpha1.SyncOptions_CreateNamespace_true,
			}

			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("This should update the existing application.")

			_, _, _, message, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Updated))

			By("Verify that the database entries have been updated.")

			var appMappingsThird []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsThird)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsThird).To(HaveLen(1))

			deplToAppMappingThird := appMappingsThird[0]
			applicationThird := db.Application{Application_id: deplToAppMappingThird.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationThird)

			Expect(err).ToNot(HaveOccurred())
			Expect(applicationThird.SeqID).To(Equal(applicationSecond.SeqID))
			Expect(applicationThird.Spec_field).NotTo(Equal(applicationSecond.Spec_field))
			Expect(strings.Contains(applicationThird.Spec_field, string(managedgitopsv1alpha1.SyncOptions_CreateNamespace_true))).To(BeTrue())

			//############################################################################
			By("Update existing deployment to a SyncOption that is not empty and is set to a false syncOption CreateNamespace=foo ")

			gitopsDepl.Spec.SyncPolicy.SyncOptions = managedgitopsv1alpha1.SyncOptions{
				"CreateNamespace=foo",
			}
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("This should update the existing application.")

			_, _, _, message, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).ToNot(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Failed))

		})
	})
})

var _ = Describe("ApplicationEventLoop Handle deployment modified Test", func() {

	// expectFauxArgoCDAppToMatchGitOpsDeployment compares the Spec_field row of an Application with the corresponding GitOpsDeployment, to ensure that Spec_field (the Spec field of the Argo CD Application) accurately reflects what the user defined in the GitOpsDeployment.
	expectFauxArgoCDAppToMatchGitOpsDeployment := func(applicationFirst db.Application, gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) {

		var appArgo fauxargocd.FauxApplication
		err := yaml.Unmarshal([]byte(applicationFirst.Spec_field), &appArgo)
		Expect(err).ToNot(HaveOccurred())

		Expect(managedgitopsv1alpha1.ApplicationSource{
			RepoURL:        appArgo.Spec.Source.RepoURL,
			Path:           appArgo.Spec.Source.Path,
			TargetRevision: appArgo.Spec.Source.TargetRevision,
		}).To(Equal(gitopsDepl.Spec.Source))

		Expect(appArgo.Spec.Destination.Namespace).To(Equal(gitopsDepl.Spec.Destination.Namespace))
		Expect(appArgo.Spec.SyncPolicy.Automated != nil).To(Equal(gitopsDepl.Spec.Type == managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated))

		if sharedutil.AppProjectIsolationEnabled() {
			Expect(appArgo.Spec.Project).To(SatisfyAll(HavePrefix("app-project-"), HaveLen(48)), "must be a valid app project uuid")
		} else {
			Expect(appArgo.Spec.Project).To(Equal("default"))
		}
	}

	Context("Handle deployment modified", func() {
		var err error
		var workspaceID string
		var ctx context.Context
		var scheme *runtime.Scheme
		var workspace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var k8sClientOuter client.WithWatch
		var k8sClient *sharedutil.ProxyClient
		var kubesystemNamespace *corev1.Namespace
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
				err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
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
		})

		It("Should update existing deployment, instead of creating new.", func() {

			// ----------------------------------------------------------------------------
			By("Create new deployment.")
			// ----------------------------------------------------------------------------
			var message deploymentModifiedResult
			_, _, _, message, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			// ----------------------------------------------------------------------------
			By("Verify that database entries are created.")
			// ----------------------------------------------------------------------------

			var appMappingsFirst []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsFirst)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsFirst).To(HaveLen(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)
			Expect(err).ToNot(HaveOccurred())

			expectFauxArgoCDAppToMatchGitOpsDeployment(applicationFirst, *gitopsDepl)

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

			// ----------------------------------------------------------------------------
			By("This should update the existing application.")
			// ----------------------------------------------------------------------------

			_, _, _, message, userDevErr = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Updated))

			// ----------------------------------------------------------------------------
			By("Verify that the database entries have been updated.")
			// ----------------------------------------------------------------------------

			var appMappingsSecond []db.DeploymentToApplicationMapping
			err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsSecond)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsSecond).To(HaveLen(1))

			deplToAppMappingSecond := appMappingsSecond[0]
			applicationSecond := db.Application{Application_id: deplToAppMappingSecond.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationSecond)

			Expect(err).ToNot(HaveOccurred())
			Expect(applicationFirst.SeqID).To(Equal(applicationSecond.SeqID))
			Expect(applicationFirst.Spec_field).NotTo(Equal(applicationSecond.Spec_field))

			clusterUser := db.ClusterUser{User_name: string(workspace.UID)}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).ToNot(HaveOccurred())

			gitopsEngineInstance := db.GitopsEngineInstance{Gitopsengineinstance_id: applicationSecond.Engine_instance_inst_id}
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{Managedenvironment_id: applicationSecond.Managed_environment_id}
			err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			expectFauxArgoCDAppToMatchGitOpsDeployment(applicationSecond, *gitopsDepl)

			//############################################################################

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDepl and verify that the corresponding DB entries are removed.")
			// ----------------------------------------------------------------------------

			gitopsDepl.Finalizers = append(gitopsDepl.Finalizers, managedgitopsv1alpha1.DeletionFinalizer)
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			// verify that the GitOpsDeployment is not deleted due to the presence of finalizer
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDepl.DeletionTimestamp).NotTo(BeNil())

			_, _, _, message, userDevErr = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Deleted))

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &applicationSecond)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).ToNot(HaveOccurred())
			Expect(appMappings).To(BeEmpty())

			// GitopsEngine instance should still be reachable
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			operationCreated := false
			operationDeleted := false
			gitopsDeploymentDeleted := false
			gitopsDeploymentUpdated := false
			var gitopsDeploymentUpdatedAt, operationDeletedAt time.Time

			for _, event := range informer.Events {
				if event.Action == sharedutil.Create && event.ObjectTypeOf() == "Operation" {
					operationCreated = true
				}
				if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "Operation" {
					operationDeleted = true
					operationDeletedAt = event.ExitTime
				}
				if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "GitOpsDeployment" {
					gitopsDeploymentDeleted = true
				}
				if event.Action == sharedutil.Update && event.ObjectTypeOf() == "GitOpsDeployment" {
					gitopsDeploymentUpdated = true
					gitopsDeploymentUpdatedAt = event.ExitTime
				}
			}

			Expect(operationCreated).To(BeTrue())
			Expect(operationDeleted).To(BeTrue())
			Expect(gitopsDeploymentUpdated).To(BeTrue())
			Expect(gitopsDeploymentDeleted).To(BeTrue())

			By("verify whether the finalizer was removed after the successful deletion of operation")
			Expect(gitopsDeploymentUpdatedAt.After(operationDeletedAt)).To(BeTrue())
		})

		It("should handle deletion even if the DB resources are removed in the previous reconciliation", func() {
			// If the controller terminates while handling the deletion of a GitOpsDeployment with a finalizer, it
			// should continue where it left and successfully complete the deletion in the next reconciliation.

			// ----------------------------------------------------------------------------
			By("Create new deployment with the deletion finalizer")
			// ----------------------------------------------------------------------------
			var message deploymentModifiedResult
			_, _, _, message, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			gitopsDepl.Finalizers = append(gitopsDepl.Finalizers, managedgitopsv1alpha1.DeletionFinalizer)
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Verify that database entries are created.")
			// ----------------------------------------------------------------------------

			var appMappingsFirst []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsFirst)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsFirst).To(HaveLen(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)
			expectFauxArgoCDAppToMatchGitOpsDeployment(applicationFirst, *gitopsDepl)

			Expect(err).ToNot(HaveOccurred())

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDeployment and its associated DB resources")
			// ----------------------------------------------------------------------------
			// Here we assume that only DB resources are being deleted in this reconciliation.
			rows, err := dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, deplToAppMappingFirst.Deploymenttoapplicationmapping_uid_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(Equal(1))

			clusterUser := db.ClusterUser{
				User_name: string(workspace.UID),
			}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).ToNot(HaveOccurred())

			rows, err = dbQueries.DeleteApplicationOwner(ctx, deplToAppMappingFirst.Application_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(Equal(1))

			rows, err = dbQueries.DeleteApplicationById(ctx, deplToAppMappingFirst.Application_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(Equal(1))

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			// verify that the GitOpsDeployment is not deleted due to the presence of finalizer
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDepl.DeletionTimestamp).NotTo(BeNil())

			// ----------------------------------------------------------------------------
			By("Verify if the finalizer is removed and the GitOpsDeployment is deleted in the next reconciliation")
			// ----------------------------------------------------------------------------
			_, _, _, message, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Deleted))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("Should not deploy application, as request data is not valid.", func() {

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			// This should fail while creating new application
			_, _, _, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).NotTo(BeNil())

			// ----------------------------------------------------------------------------
			By("Verify that the database entry is not created.")
			// ----------------------------------------------------------------------------

			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappings).To(BeEmpty())
		})

		It("Should not update existing deployment, if no changes were done in fields.", func() {
			// This should create new application
			var message deploymentModifiedResult
			_, _, _, message, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			// ----------------------------------------------------------------------------
			By("Verify that the database entries have been created.")
			// ----------------------------------------------------------------------------

			var appMappingsFirst []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsFirst)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappingsFirst).To(HaveLen(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)
			Expect(err).ToNot(HaveOccurred())
			expectFauxArgoCDAppToMatchGitOpsDeployment(applicationFirst, *gitopsDepl)

			//--------------------------------------------------------------------------------------
			// Now create new client and event loop runner object.
			//--------------------------------------------------------------------------------------

			k8sClientOuter = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			appEventLoopRunnerActionSecond := applicationEventLoopRunner_Action{
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

			//--------------------------------------------------------------------------------------
			// Pass same gitOpsDeployment again, but no changes should be done in the application.
			//--------------------------------------------------------------------------------------
			_, _, _, message, userDevErr = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_NoChange))

			//############################################################################

			// ----------------------------------------------------------------------------
			By("Delete the GitOpsDepl and verify that the corresponding DB entries are removed.")
			// ----------------------------------------------------------------------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, message, userDevErr = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Deleted))

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &applicationFirst)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).ToNot(HaveOccurred())
			Expect(appMappings).To(BeEmpty())

			// GitopsEngine instance should still be reachable
			gitopsEngineInstance := db.GitopsEngineInstance{Gitopsengineinstance_id: applicationFirst.Engine_instance_inst_id}
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

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

		It("creates a deployment and ensure it was processed, then deletes it and ensure that it was processed", func() {
			_, _, _, _, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// Verify that the database entries have been created -----------------------------------------

			var deplToAppMapping db.DeploymentToApplicationMapping
			{
				var appMappings []db.DeploymentToApplicationMapping

				err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
				Expect(err).ToNot(HaveOccurred())

				Expect(appMappings).To(HaveLen(1))

				deplToAppMapping = appMappings[0]
			}

			clusterUser := db.ClusterUser{
				User_name: string(workspace.UID),
			}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).ToNot(HaveOccurred())

			application := db.Application{
				Application_id: deplToAppMapping.Application_id,
			}

			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).ToNot(HaveOccurred())
			expectFauxArgoCDAppToMatchGitOpsDeployment(application, *gitopsDepl)

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: application.Engine_instance_inst_id,
			}

			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: application.Managed_environment_id,
			}
			err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			// Delete the GitOpsDepl and verify that the corresponding DB entries are removed -------------

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())

			// Application should no longer exist
			err = dbQueries.GetApplicationById(ctx, &application)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// DeploymentToApplicationMapping should be removed, too
			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
			Expect(err).ToNot(HaveOccurred())
			Expect(appMappings).To(BeEmpty())

			// GitopsEngine instance should still be reachable
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

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

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("abc", 100),
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						Path: "resources/test-data/sample-gitops-repository/environments/overlays/dev",
					},
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			workspaceID := string(workspace.UID)

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).ToNot(HaveOccurred())

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

			// ------

			_, _, _, _, userDevErr := a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).NotTo(BeNil())

		})

		It("should return an error if the Argo CD Application name field changed between when the GitOpsDeployment was created, and when it was updated", func() {

			By("calling handleDeploymentModified to simulate a new GitOpsDeployment")
			_, applicationDBRow, _, result, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(result).To(Equal(deploymentModifiedResult_Created))

			By("calling handleDeploymentModified again, to simulate an unchanged GitOpsDeployment")
			_, _, _, result, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(result).To(Equal(deploymentModifiedResult_NoChange))

			By("updating the Application name field, simulating the case where a different name was set in the Create logic of handleDeploymentModified")
			applicationDBRow.Name = "a-different-name-than-the-one-set-by-create"
			err := dbQueries.UpdateApplication(ctx, applicationDBRow)
			Expect(err).ToNot(HaveOccurred())

			By("calling handleDeploymentModified again, and expected an error")
			_, _, _, result, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).ToNot(BeNil())
			Expect(result).To(Equal(deploymentModifiedResult_Failed))

		})

		It("should not create a new GitOpsDeployment if the Namespace of the GitOpsDeployment is being deleted", func() {

			By("simulating the start of deletion of the Namespace of the existing GitOpsDeployment")
			workspace.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			workspace.Finalizers = []string{"my-finalizer"}
			err := k8sClient.Update(ctx, workspace)
			Expect(err).ToNot(HaveOccurred())

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
				eventResourceName:           gitopsDepl.Name,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				eventResourceNamespace:      workspace.Name,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			By("calling the function under test, to inform it about the new event")
			_, _, _, res, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			Expect(userDevErr).To(BeNil())
			Expect(res).To(Equal(deploymentModifiedResult_NoChange),
				"since the Namespace is being deleted, the request should not be acted upon")
		})

		It("Should verify whether ApplicationOwner row has been created, retrieved and deleted.", func() {
			By("Create new deployment.")
			var message deploymentModifiedResult
			_, _, _, message, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappings).To(HaveLen(1))

			deplToAppMappingFirst := appMappings[0]
			application := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).ToNot(HaveOccurred())

			clusterUser := db.ClusterUser{
				User_name: string(workspace.UID),
			}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).ToNot(HaveOccurred())

			By("Verify whether ApplicationOwner has been created in database")
			applicationOwner := db.ApplicationOwner{
				ApplicationOwnerApplicationID: application.Application_id,
			}

			err = dbQueries.GetApplicationOwnerByApplicationID(context.Background(), &applicationOwner)
			Expect(err).ToNot(HaveOccurred())

			By("Verify whether ApplicationOwner exists when Application row is updated")
			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).ToNot(HaveOccurred())

			application.Spec_field = "test-update-application"
			err := dbQueries.UpdateApplication(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, result, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(result).To(Equal(deploymentModifiedResult_Updated))

			err = dbQueries.GetApplicationOwnerByApplicationID(context.Background(), &applicationOwner)
			Expect(err).ToNot(HaveOccurred())

			By("Verify whether ApplicationOwner has been deleted")
			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, message, userDevErr = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Deleted))

			err = dbQueries.GetApplicationOwnerByApplicationID(context.Background(), &applicationOwner)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})

		It("Should verify whether ApplicationOwner is recreated while updating application if ApplicationOwner doesn't exists", func() {
			By("Create new deployment.")
			var message deploymentModifiedResult
			_, _, _, message, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Created))

			var appMappings []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)

			Expect(err).ToNot(HaveOccurred())
			Expect(appMappings).To(HaveLen(1))

			deplToAppMappingFirst := appMappings[0]
			application := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).ToNot(HaveOccurred())

			clusterUser := db.ClusterUser{
				User_name: string(workspace.UID),
			}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).ToNot(HaveOccurred())

			By("Verify whether ApplicationOwner has been created in database")
			applicationOwner := db.ApplicationOwner{
				ApplicationOwnerApplicationID: application.Application_id,
			}

			err = dbQueries.GetApplicationOwnerByApplicationID(context.Background(), &applicationOwner)
			Expect(err).ToNot(HaveOccurred())

			By("Verify whether ApplicationOwner exists when Application row is updated")
			err = dbQueries.GetApplicationById(context.Background(), &application)
			Expect(err).ToNot(HaveOccurred())

			application.Spec_field = "test-update-application"
			err := dbQueries.UpdateApplication(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			By("Delete ApplicationOwner")
			rowsAffected, err := dbQueries.DeleteApplicationOwner(ctx, applicationOwner.ApplicationOwnerApplicationID)
			Expect(rowsAffected).To(Equal(1))
			Expect(err).ToNot(HaveOccurred())

			_, _, _, result, userDevErr := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(result).To(Equal(deploymentModifiedResult_Updated))

			By("Verify whether ApplicationOwner is recreated while updating application")
			err = dbQueries.GetApplicationOwnerByApplicationID(context.Background(), &applicationOwner)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("isManagedEnvironmentConnectionUserError function Test", func() {
	Context("Testing isManagedEnvironmentConnectionUserError function.", func() {

		It("should return True if error is related to connection issue.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			rootErr := fmt.Errorf("Get \"https://api.ireland.burr-on-aws.com:6443/api?timeout=32s\": dial tcp: lookup api.ireland.burr-on-aws.com on 172.30.0.10:53: no such host")
			err := fmt.Errorf("%s: %w", shared_resource_loop.UnableToCreateRestConfigError, rootErr)

			result := IsManagedEnvironmentConnectionUserError(err, log)
			Expect(result).To(BeTrue())
		})

		It("should return False if error is not related to connection issue.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			rootErr := fmt.Errorf("some error")
			err := fmt.Errorf("another error: %w", rootErr)

			result := IsManagedEnvironmentConnectionUserError(err, log)
			Expect(result).To(BeFalse())
		})

		It("should return False if error is nil", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			result := IsManagedEnvironmentConnectionUserError(nil, log)
			Expect(result).To(BeFalse())
		})
	})
})
