package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("DB Reconciler Test", func() {
	Context("Controller event loop responds to channel events", func() {

		var log logr.Logger
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var application db.Application
		var syncOperation db.SyncOperation
		var applicationState db.ApplicationState
		var managedEnvironment *db.ManagedEnvironment
		var gitopsEngineInstance *db.GitopsEngineInstance
		var gitopsDepl managedgitopsv1alpha1.GitOpsDeployment
		var deploymentToApplicationMapping db.DeploymentToApplicationMapping

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			// Create Application entry
			application = db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &application)
			Expect(err).To(BeNil())

			// Create ApplicationState entry
			applicationState = db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 "Healthy",
			}
			err = dbq.CreateApplicationState(ctx, &applicationState)
			Expect(err).To(BeNil())

			gitopsDepl = managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					UID:       "test-" + types.UID(uuid.New().String()),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			// Create DeploymentToApplicationMapping entry
			deploymentToApplicationMapping = db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: string(gitopsDepl.UID),
				Application_id:                        application.Application_id,
				DeploymentName:                        gitopsDepl.Name,
				DeploymentNamespace:                   gitopsDepl.Namespace,
				NamespaceUID:                          "demo-namespace",
			}
			err = dbq.CreateDeploymentToApplicationMapping(ctx, &deploymentToApplicationMapping)
			Expect(err).To(BeNil())

			// Create GitOpsDeployment CR in cluster
			err = k8sClient.Create(context.Background(), &gitopsDepl)
			Expect(err).To(BeNil())

			// Create SyncOperation entry
			syncOperation = db.SyncOperation{
				SyncOperation_id:    "test-syncOperation",
				Application_id:      application.Application_id,
				Revision:            "master",
				DeploymentNameField: deploymentToApplicationMapping.DeploymentName,
				DesiredState:        "Synced",
			}
			err = dbq.CreateSyncOperation(ctx, &syncOperation)
			Expect(err).To(BeNil())
		})

		var _ = Describe("Tests Database Reconciler utility function.", func() {
			Context("It tests Database Reconciler utility function databaseReconcile().", func() {

				It("Should not delete any of the database entries as long as the GitOpsDeployment CR is present in cluster, and the UID matches the DTAM value", func() {
					defer dbq.CloseDatabase()

					By("Call function for databaseReconcile to check delete DB entries if GitOpsDeployment CR is not present.")
					databaseReconcile(ctx, dbq, k8sClient, log)

					By("Verify that no entry is deleted from DB.")
					err := dbq.GetApplicationStateById(ctx, &applicationState)
					Expect(err).To(BeNil())

					err = dbq.GetSyncOperationById(ctx, &syncOperation)
					Expect(err).To(BeNil())
					Expect(syncOperation.Application_id).NotTo(BeEmpty())

					err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMapping)
					Expect(err).To(BeNil())

					err = dbq.GetApplicationById(ctx, &application)
					Expect(err).To(BeNil())
				})

				It("Should delete related database entries from DB, if the GitOpsDeployment CRs of the DTAM is not present on cluster.", func() {
					defer dbq.CloseDatabase()

					// Create another Application entry
					applicationOne := application
					applicationOne.Application_id = "test-my-application-1"
					applicationOne.Name = "my-application-1"
					err := dbq.CreateApplication(ctx, &applicationOne)
					Expect(err).To(BeNil())

					// Create another DeploymentToApplicationMapping entry
					deploymentToApplicationMappingOne := deploymentToApplicationMapping
					deploymentToApplicationMappingOne.Deploymenttoapplicationmapping_uid_id = "test-" + uuid.New().String()
					deploymentToApplicationMappingOne.Application_id = applicationOne.Application_id
					deploymentToApplicationMappingOne.DeploymentName = "test-deployment-1"
					err = dbq.CreateDeploymentToApplicationMapping(ctx, &deploymentToApplicationMappingOne)
					Expect(err).To(BeNil())

					// Create another ApplicationState entry
					applicationStateOne := applicationState
					applicationStateOne.Applicationstate_application_id = applicationOne.Application_id
					err = dbq.CreateApplicationState(ctx, &applicationStateOne)
					Expect(err).To(BeNil())

					// Create another SyncOperation entry
					syncOperationOne := syncOperation
					syncOperationOne.SyncOperation_id = "test-syncOperation-1"
					syncOperationOne.Application_id = applicationOne.Application_id
					syncOperationOne.DeploymentNameField = deploymentToApplicationMappingOne.DeploymentName
					err = dbq.CreateSyncOperation(ctx, &syncOperationOne)
					Expect(err).To(BeNil())

					By("Call function for databaseReconcile to check/delete DB entries if GitOpsDeployment CR is not present.")
					databaseReconcile(ctx, dbq, k8sClient, log)

					By("Verify that entries for the GitOpsDeployment which is available in cluster, are not deleted from DB.")

					err = dbq.GetApplicationStateById(ctx, &applicationState)
					Expect(err).To(BeNil())

					err = dbq.GetSyncOperationById(ctx, &syncOperation)
					Expect(err).To(BeNil())
					Expect(syncOperation.Application_id).To(Equal(application.Application_id))

					err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMapping)
					Expect(err).To(BeNil())

					err = dbq.GetApplicationById(ctx, &application)
					Expect(err).To(BeNil())

					By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

					err = dbq.GetApplicationStateById(ctx, &applicationStateOne)
					Expect(db.IsResultNotFoundError(err)).To(BeTrue())

					err = dbq.GetSyncOperationById(ctx, &syncOperationOne)
					Expect(err).To(BeNil())
					Expect(syncOperationOne.Application_id).To(BeEmpty())

					err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMappingOne)
					Expect(db.IsResultNotFoundError(err)).To(BeTrue())

					err = dbq.GetApplicationById(ctx, &applicationOne)
					Expect(db.IsResultNotFoundError(err)).To(BeTrue())
				})

				It("should delete the DTAM if the GitOpsDeployment CR if it is present, but the UID doesn't match what is in the DTAM", func() {
					defer dbq.CloseDatabase()

					By("We ensure that the GitOpsDeployment still has the same name, but has a different UID. This simulates a new GitOpsDeployment with the same name/namespace.")
					newUID := "test-" + types.UID(uuid.New().String())
					gitopsDepl.UID = newUID
					err := k8sClient.Update(ctx, &gitopsDepl)
					Expect(err).To(BeNil())

					err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
					Expect(err).To(BeNil())
					Expect(gitopsDepl.UID).To(Equal(newUID))

					By("calling function for databaseReconcile to check delete DB entries if GitOpsDeployment CR is not present.")
					databaseReconcile(ctx, dbq, k8sClient, log)

					By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

					err = dbq.GetApplicationStateById(ctx, &applicationState)
					Expect(db.IsResultNotFoundError(err)).To(BeTrue())

					err = dbq.GetSyncOperationById(ctx, &syncOperation)
					Expect(err).To(BeNil())
					Expect(syncOperation.Application_id).To(BeEmpty())

					err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &deploymentToApplicationMapping)
					Expect(db.IsResultNotFoundError(err)).To(BeTrue())

					err = dbq.GetApplicationById(ctx, &application)
					Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				})
			})
		})
	})
})
