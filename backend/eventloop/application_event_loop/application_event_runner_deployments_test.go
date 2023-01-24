package application_event_loop

import (
	"context"
	"strings"

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Application Event Runner Deployments", func() {
	Context("createSpecField() should generate a valid argocd Application", func() {
		getfakeArgoCDSpecInput := func(automated, unsanitized bool) argoCDSpecInput {
			input := argoCDSpecInput{
				crName:               "sample-depl",
				crNamespace:          "kcp-workspace",
				destinationNamespace: "prod",
				destinationName:      "in-cluster",
				sourceRepoURL:        "https://github.com/test/test",
				sourcePath:           "environments/prod",
				automated:            automated,
			}

			if unsanitized {
				input.sourcePath = "environments/&&prod\n"
				input.sourceRepoURL = "https://github.com/```%test/test"
			}
			return input
		}

		getValidApplication := func(automated bool) string {
			input := getfakeArgoCDSpecInput(automated, false)
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
					Project: "default",
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
				}
			}

			appBytes, err := yaml.Marshal(application)
			if err != nil {
				GinkgoT().Fatalf("failed to unmarshall Application: %q", err)
			}

			return string(appBytes)
		}

		It("Input spec is converted to an argocd Application", func() {
			input := getfakeArgoCDSpecInput(false, false)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(getValidApplication(false)))
		})

		It("Sanitize illegal characters from input", func() {
			input := getfakeArgoCDSpecInput(false, true)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
			Expect(application).To(Equal(getValidApplication(false)))
		})

		It("Input spec with automated enabled should set automated sync policy", func() {
			input := getfakeArgoCDSpecInput(true, false)
			application, err := createSpecField(input)
			Expect(err).To(BeNil())
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
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
					SyncPolicy: &managedgitopsv1alpha1.SyncPolicy{
						SyncOptions: managedgitopsv1alpha1.SyncOptions{
							"- CreateNamespace=true",
						},
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
				getK8sClientForGitOpsEngineInstance: func(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
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

			Expect(err).To(BeNil())
			Expect(len(appMappingsFirst)).To(Equal(1))

			deplToAppMappingFirst := appMappingsFirst[0]
			applicationFirst := db.Application{Application_id: deplToAppMappingFirst.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationFirst)

			Expect(err).To(BeNil())

			Expect(strings.Contains(applicationFirst.Spec_field, "- CreateNamespace=true")).To(Equal(true))
			//############################################################################

			By("Update existing deployment so that the SyncOption is set to nil/empty")
			var emptySyncOption []string
			gitopsDepl.Spec.SyncPolicy.SyncOptions = emptySyncOption

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
				getK8sClientForGitOpsEngineInstance: func(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
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

			By("This should update the existing application.")

			_, _, _, message, userDevErr = appEventLoopRunnerActionSecond.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Updated))

			By("Verify that the database entries have been updated.")

			var appMappingsSecond []db.DeploymentToApplicationMapping
			err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsSecond)

			Expect(err).To(BeNil())
			Expect(len(appMappingsSecond)).To(Equal(1))

			deplToAppMappingSecond := appMappingsSecond[0]
			applicationSecond := db.Application{Application_id: deplToAppMappingSecond.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationSecond)

			Expect(err).To(BeNil())
			Expect(applicationFirst.SeqID).To(Equal(applicationSecond.SeqID))
			Expect(applicationFirst.Spec_field).NotTo(Equal(applicationSecond.Spec_field))
			Expect(strings.Contains(applicationSecond.Spec_field, "- CreateNamespace=true")).To(Equal(false))

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
			By("Update existing deployment to a SyncOption that is not empty and is set to CreateNamespace=true")

			gitopsDepl.Spec.SyncPolicy.SyncOptions = managedgitopsv1alpha1.SyncOptions{
				"- CreateNamespace=true",
			}

			// Create new client and application runner, but pass existing gitOpsDeployment object.
			k8sClientOuter = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()
			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			appEventLoopRunnerActionThird := applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
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

			By("This should update the existing application.")

			_, _, _, message, userDevErr = appEventLoopRunnerActionThird.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			Expect(userDevErr).To(BeNil())
			Expect(message).To(Equal(deploymentModifiedResult_Updated))

			By("Verify that the database entries have been updated.")

			var appMappingsThird []db.DeploymentToApplicationMapping
			err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappingsThird)

			Expect(err).To(BeNil())
			Expect(len(appMappingsSecond)).To(Equal(1))

			deplToAppMappingThird := appMappingsThird[0]
			applicationThird := db.Application{Application_id: deplToAppMappingThird.Application_id}
			err = dbQueries.GetApplicationById(context.Background(), &applicationThird)

			Expect(err).To(BeNil())
			Expect(applicationThird.SeqID).To(Equal(applicationSecond.SeqID))
			Expect(applicationThird.Spec_field).NotTo(Equal(applicationSecond.Spec_field))
			Expect(strings.Contains(applicationThird.Spec_field, "- CreateNamespace=true")).To(Equal(true))

			clusterUser = db.ClusterUser{User_name: string(workspace.UID)}
			err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
			Expect(err).To(BeNil())

			gitopsEngineInstance = db.GitopsEngineInstance{Gitopsengineinstance_id: applicationThird.Engine_instance_inst_id}
			err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance)
			Expect(err).To(BeNil())

			managedEnvironment = db.ManagedEnvironment{Managedenvironment_id: applicationThird.Managed_environment_id}
			err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).To(BeNil())

		})

	})
})
