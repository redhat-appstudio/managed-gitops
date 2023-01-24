package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedoperations "github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("DB Reconciler Test", func() {
	Context("Testing Reconcile for DeplToAppMappingDbReconcile table entries.", func() {

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
					UID:       "test-" + uuid.NewUUID(),
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

		It("Should not delete any of the database entries as long as the GitOpsDeployment CR is present in cluster, and the UID matches the DTAM value", func() {
			defer dbq.CloseDatabase()

			By("Call function for deplToAppMappingDbReconcile to check delete DB entries if GitOpsDeployment CR is not present.")
			deplToAppMappingDbReconcile(ctx, dbq, k8sClient, log)

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
			deploymentToApplicationMappingOne.Deploymenttoapplicationmapping_uid_id = "test-" + string(uuid.NewUUID())
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

			By("Call function for deplToAppMappingDbReconcile to check/delete DB entries if GitOpsDeployment CR is not present.")
			deplToAppMappingDbReconcile(ctx, dbq, k8sClient, log)

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

		It("Should delete the DTAM if the GitOpsDeployment CR if it is present, but the UID doesn't match what is in the DTAM", func() {
			defer dbq.CloseDatabase()

			By("We ensure that the GitOpsDeployment still has the same name, but has a different UID. This simulates a new GitOpsDeployment with the same name/namespace.")
			newUID := "test-" + uuid.NewUUID()
			gitopsDepl.UID = newUID
			err := k8sClient.Update(ctx, &gitopsDepl)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
			Expect(err).To(BeNil())
			Expect(gitopsDepl.UID).To(Equal(newUID))

			By("calling function for deplToAppMappingDbReconcile to check delete DB entries if GitOpsDeployment CR is not present.")
			deplToAppMappingDbReconcile(ctx, dbq, k8sClient, log)

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

	Context("Testing Reconcile for APICRToDBMapping table entries.", func() {
		Context("Testing Reconcile for APICRToDBMapping table entries of ManagedEnvironment CR.", func() {
			var log logr.Logger
			var ctx context.Context
			var dbq db.AllDatabaseQueries
			var k8sClient client.WithWatch
			var clusterCredentialsDb db.ClusterCredentials
			var managedEnvironmentDb db.ManagedEnvironment
			var apiCRToDatabaseMappingDb db.APICRToDatabaseMapping
			var managedEnvCr *managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment

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

				By("Create required CRs in Cluster.")

				// Create Secret in Cluster
				secretCr := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-managed-env-secret",
						Namespace: "test-k8s-namespace",
					},
					Type:       "managed-gitops.redhat.com/managed-environment",
					StringData: map[string]string{"kubeconfig": "abc"},
				}
				err = k8sClient.Create(context.Background(), secretCr)
				Expect(err).To(BeNil())

				// Create GitOpsDeploymentManagedEnvironment CR in cluster
				managedEnvCr = &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-" + string(uuid.NewUUID()),
						Namespace: "test-k8s-namespace",
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
						APIURL:                     "",
						ClusterCredentialsSecret:   secretCr.Name,
						AllowInsecureSkipTLSVerify: true,
					},
				}
				err = k8sClient.Create(context.Background(), managedEnvCr)
				Expect(err).To(BeNil())

				By("Create required DB entries.")

				// Create DB entry for ClusterCredentials
				clusterCredentialsDb = db.ClusterCredentials{
					Clustercredentials_cred_id:  "test-" + string(uuid.NewUUID()),
					Host:                        "host",
					Kube_config:                 "kube-config",
					Kube_config_context:         "kube-config-context",
					Serviceaccount_bearer_token: "serviceaccount_bearer_token",
					Serviceaccount_ns:           "Serviceaccount_ns",
				}
				err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsDb)
				Expect(err).To(BeNil())

				// Create DB entry for ManagedEnvironment
				managedEnvironmentDb = db.ManagedEnvironment{
					Managedenvironment_id: "test-env-" + string(managedEnvCr.UID),
					Clustercredentials_id: clusterCredentialsDb.Clustercredentials_cred_id,
					Name:                  managedEnvCr.Name,
				}
				err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
				Expect(err).To(BeNil())

				// Create DB entry for APICRToDatabaseMapping
				apiCRToDatabaseMappingDb = db.APICRToDatabaseMapping{
					APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
					APIResourceUID:       string(managedEnvCr.UID),
					APIResourceName:      managedEnvCr.Name,
					APIResourceNamespace: managedEnvCr.Namespace,
					NamespaceUID:         "test-" + string(uuid.NewUUID()),
					DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
					DBRelationKey:        managedEnvironmentDb.Managedenvironment_id,
				}
			})

			It("Should not delete any of the database entries as long as the Managed Environment CR is present in cluster, and the UID matches the APICRToDatabaseMapping value", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that no entry is deleted from DB.")
				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDb)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the Managed Environment CR of the APICRToDatabaseMapping is not present on cluster.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				// Create DB entry for ClusterCredentials
				clusterCredentialsDb.Clustercredentials_cred_id = "test-" + string(uuid.NewUUID())
				err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsDb)
				Expect(err).To(BeNil())

				// Create another ManagedEnvironment entry
				managedEnvironmentDbTemp := managedEnvironmentDb
				managedEnvironmentDb.Name = "test-env-" + string(uuid.NewUUID())
				managedEnvironmentDb.Managedenvironment_id = "test-" + string(uuid.NewUUID())
				managedEnvironmentDb.Clustercredentials_id = clusterCredentialsDb.Clustercredentials_cred_id
				err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
				Expect(err).To(BeNil())

				// Create another APICRToDatabaseMapping entry
				apiCRToDatabaseMappingDbTemp := apiCRToDatabaseMappingDb
				apiCRToDatabaseMappingDb.DBRelationKey = managedEnvironmentDb.Managedenvironment_id
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMappingDb.APIResourceName = managedEnvironmentDb.Name
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that entries for the ManagedEnvironment which is not available in cluster, are deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				By("Verify that entries for the ManagedEnvironment which is available in cluster, are not deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDbTemp)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDbTemp)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the Managed Environment CR is present in cluster, but the UID doesn't match what is in the APICRToDatabaseMapping", func() {
				defer dbq.CloseDatabase()

				// Create another ACTDB entry
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that entries for the ManagedEnvironment which is not available in cluster, are deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})

			It("Should delete related database entries from DB, if the Managed Environment CR of the APICRToDatabaseMapping is not present on cluster and it should create Operation to inform.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				// Create DB entry for ClusterCredentials
				clusterCredentialsDb.Clustercredentials_cred_id = "test-" + string(uuid.NewUUID())
				err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsDb)
				Expect(err).To(BeNil())

				// Create another ManagedEnvironment entry
				managedEnvironmentDbTemp := managedEnvironmentDb
				managedEnvironmentDb.Name = "test-env-" + string(uuid.NewUUID())
				managedEnvironmentDb.Managedenvironment_id = "test-" + string(uuid.NewUUID())
				managedEnvironmentDb.Clustercredentials_id = clusterCredentialsDb.Clustercredentials_cred_id
				err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
				Expect(err).To(BeNil())

				// Create another APICRToDatabaseMapping entry
				apiCRToDatabaseMappingDbTemp := apiCRToDatabaseMappingDb
				apiCRToDatabaseMappingDb.DBRelationKey = managedEnvironmentDb.Managedenvironment_id
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMappingDb.APIResourceName = managedEnvironmentDb.Name
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
				Expect(err).To(BeNil())

				// Create DB entry for Application
				applicationDb := &db.Application{
					Application_id:          "test-app-" + string(uuid.NewUUID()),
					Name:                    "test-app",
					Spec_field:              "{}",
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironmentDb.Managedenvironment_id,
				}
				err = dbq.CreateApplication(ctx, applicationDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, MockSRLK8sClientFactory{fakeClient: k8sClient}, log)

				By("Verify that entries for the ManagedEnvironment which is not available in cluster, are deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				By("Verify that entries for the ManagedEnvironment which is available in cluster, are not deleted from DB.")

				err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDbTemp)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDbTemp)
				Expect(err).To(BeNil())

				By("Verify that Operation for the ManagedEnvironment is created.")

				var operationlist []db.Operation
				err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, managedEnvironmentDb.Managedenvironment_id, db.OperationResourceType_ManagedEnvironment, &operationlist, "cluster-agent-application-sync-user")
				Expect(err).To(BeNil())
				Expect(len(operationlist)).ShouldNot(Equal(0))
				Expect(operationlist[0].Resource_id).To(Equal(managedEnvironmentDb.Managedenvironment_id))
			})
		})

		Context("Testing Reconcile for APICRToDBMapping table entries of RepositoryCredential CR.", func() {
			var log logr.Logger
			var ctx context.Context
			var dbq db.AllDatabaseQueries
			var k8sClient client.WithWatch
			var clusterUserDb *db.ClusterUser
			var apiCRToDatabaseMappingDb db.APICRToDatabaseMapping
			var gitopsRepositoryCredentialsDb db.RepositoryCredentials
			var repoCredentialCr managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential

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

				By("Create required CRs in Cluster.")

				// Create Secret in Cluster
				secretCr := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-k8s-namespace",
					},
					Type: "managed-gitops.redhat.com/managed-environment",
					StringData: map[string]string{
						"username": "test-user",
						"password": "test@123",
					},
				}
				err = k8sClient.Create(context.Background(), secretCr)
				Expect(err).To(BeNil())

				// Create GitOpsDeploymentRepositoryCredential in Cluster
				repoCredentialCr = managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-repo-" + string(uuid.NewUUID()),
						Namespace: "test-k8s-namespace",
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
						Repository: "https://test-private-url",
						Secret:     "test-secret",
					},
				}
				err = k8sClient.Create(context.Background(), &repoCredentialCr)
				Expect(err).To(BeNil())

				By("Create required DB entries.")

				_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
				Expect(err).To(BeNil())

				// Create DB entry for ClusterUser
				clusterUserDb = &db.ClusterUser{
					Clusteruser_id: "test-repocred-user-id",
					User_name:      "test-repocred-user",
				}
				err = dbq.CreateClusterUser(ctx, clusterUserDb)
				Expect(err).To(BeNil())

				// Create DB entry for RepositoryCredentials
				gitopsRepositoryCredentialsDb = db.RepositoryCredentials{
					RepositoryCredentialsID: "test-repo-" + string(uuid.NewUUID()),
					UserID:                  clusterUserDb.Clusteruser_id,
					PrivateURL:              "https://test-private-url",
					AuthUsername:            "test-auth-username",
					AuthPassword:            "test-auth-password",
					AuthSSHKey:              "test-auth-ssh-key",
					SecretObj:               "test-secret-obj",
					EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
				}
				err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsDb)
				Expect(err).To(BeNil())

				// Create DB entry for APICRToDatabaseMapping
				apiCRToDatabaseMappingDb = db.APICRToDatabaseMapping{
					APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
					APIResourceUID:       string(repoCredentialCr.UID),
					APIResourceName:      repoCredentialCr.Name,
					APIResourceNamespace: repoCredentialCr.Namespace,
					NamespaceUID:         "test-" + string(uuid.NewUUID()),
					DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
					DBRelationKey:        gitopsRepositoryCredentialsDb.RepositoryCredentialsID,
				}
			})

			It("Should not delete any of the database entries as long as the RepositoryCredentials CR is present in cluster, and the UID matches the APICRToDatabaseMapping value", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that no entry is deleted from DB.")
				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsDb.RepositoryCredentialsID)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the RepositoryCredentials CR of the APICRToDatabaseMapping is not present on cluster.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				// Create another GitopsRepositoryCredentials entry in Db
				gitopsRepositoryCredentialsDbTemp := gitopsRepositoryCredentialsDb
				gitopsRepositoryCredentialsDb.RepositoryCredentialsID = "test-repo-" + string(uuid.NewUUID())
				err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsDb)
				Expect(err).To(BeNil())

				// Create another APICRToDatabaseMapping entry in Db
				apiCRToDatabaseMappingTemp := apiCRToDatabaseMappingDb
				apiCRToDatabaseMappingDb.DBRelationKey = gitopsRepositoryCredentialsDb.RepositoryCredentialsID
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMappingDb.APIResourceName = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that entries for the GitOpsDeployment which is not available in cluster, are deleted from DB.")

				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsDb.RepositoryCredentialsID)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				By("Verify that entries for the RepositoryCredentials which is available in cluster, are not deleted from DB.")

				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsDbTemp.RepositoryCredentialsID)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingTemp)
				Expect(err).To(BeNil())

				By("Verify that Operation for the RepositoryCredentials is created in cluster and DB.")

				var specialClusterUser db.ClusterUser
				err = dbq.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser)
				Expect(err).To(BeNil())

				var operationlist []db.Operation
				err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, gitopsRepositoryCredentialsDb.RepositoryCredentialsID, db.OperationResourceType_RepositoryCredentials, &operationlist, specialClusterUser.Clusteruser_id)
				Expect(err).To(BeNil())
				Expect(len(operationlist)).ShouldNot(Equal(0))

				objectMeta := metav1.ObjectMeta{
					Name:      sharedoperations.GenerateOperationCRName(operationlist[0]),
					Namespace: repoCredentialCr.Namespace,
				}
				k8sOperation := managedgitopsv1alpha1.Operation{ObjectMeta: objectMeta}

				err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: objectMeta.Namespace, Name: objectMeta.Name}, &k8sOperation)
				Expect(err).To(BeNil())
			})

			It("should delete related database entries from DB, if the RepositoryCredentials CR is present in cluster, but the UID doesn't match what is in the APICRToDatabaseMapping", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				// Create another RepositoryCredentials entry in Db
				gitopsRepositoryCredentialsDb.RepositoryCredentialsID = "test-repo-" + string(uuid.NewUUID())
				err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsDb)
				Expect(err).To(BeNil())

				// Create another APICRToDatabaseMapping entry in Db
				apiCRToDatabaseMappingDb.DBRelationKey = gitopsRepositoryCredentialsDb.RepositoryCredentialsID
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that entries for the RepositoryCredentials which is not available in cluster, are deleted from DB.")

				_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsDb.RepositoryCredentialsID)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})
		})

		Context("Testing Reconcile for APICRToDBMapping table entries of GitOpsDeploymentSyncRun CR.", func() {
			var log logr.Logger
			var ctx context.Context
			var dbq db.AllDatabaseQueries
			var k8sClient client.WithWatch
			var syncOperationDb db.SyncOperation
			var apiCRToDatabaseMappingDb db.APICRToDatabaseMapping
			var gitopsDeplSyncRunCr managedgitopsv1alpha1.GitOpsDeploymentSyncRun

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

				By("Create required CRs in Cluster.")

				// Create GitOpsDeploymentSyncRun in Cluster
				gitopsDeplSyncRunCr = managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-gitopsdeployment-syncrun",
						Namespace: "test-k8s-namespace",
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
						GitopsDeploymentName: "test-app",
						RevisionID:           "HEAD",
					},
				}

				err = k8sClient.Create(context.Background(), &gitopsDeplSyncRunCr)
				Expect(err).To(BeNil())

				By("Create required DB entries.")

				_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
				Expect(err).To(BeNil())

				// Create DB entry for Application
				applicationDb := &db.Application{
					Application_id:          "test-app-" + string(uuid.NewUUID()),
					Name:                    gitopsDeplSyncRunCr.Spec.GitopsDeploymentName,
					Spec_field:              "{}",
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}
				err = dbq.CreateApplication(ctx, applicationDb)
				Expect(err).To(BeNil())

				// Create DB entry for SyncOperation
				syncOperationDb = db.SyncOperation{
					SyncOperation_id:    "test-op-" + string(uuid.NewUUID()),
					Application_id:      applicationDb.Application_id,
					DeploymentNameField: "test-depl-" + string(uuid.NewUUID()),
					Revision:            "Head",
					DesiredState:        "Terminated",
				}
				err = dbq.CreateSyncOperation(ctx, &syncOperationDb)
				Expect(err).To(BeNil())

				// Create DB entry for APICRToDatabaseMappingapiCRToDatabaseMapping
				apiCRToDatabaseMappingDb = db.APICRToDatabaseMapping{
					APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
					APIResourceUID:       string(gitopsDeplSyncRunCr.UID),
					APIResourceName:      gitopsDeplSyncRunCr.Name,
					APIResourceNamespace: gitopsDeplSyncRunCr.Namespace,
					NamespaceUID:         "test-" + string(uuid.NewUUID()),
					DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
					DBRelationKey:        syncOperationDb.SyncOperation_id,
				}
			})

			It("Should not delete any of the database entries as long as the GitOpsDeploymentSyncRun CR is present in cluster, and the UID matches the APICRToDatabaseMapping value", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that no entry is deleted from DB.")
				err = dbq.GetSyncOperationById(ctx, &syncOperationDb)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())
			})

			It("Should delete related database entries from DB, if the GitOpsDeploymentSyncRun CR of the APICRToDatabaseMapping is not present on cluster.", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				// Create another entry for SyncOperation
				syncOperationDbTemp := syncOperationDb
				syncOperationDb.SyncOperation_id = "test-sync-" + string(uuid.NewUUID())
				err = dbq.CreateSyncOperation(ctx, &syncOperationDb)
				Expect(err).To(BeNil())

				// Create another entry for APICRToDatabaseMapping
				apiCRToDatabaseMappingDbTemp := apiCRToDatabaseMappingDb
				apiCRToDatabaseMappingDb.DBRelationKey = syncOperationDb.SyncOperation_id
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				apiCRToDatabaseMappingDb.APIResourceName = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that entries for the GitOpsDeploymentSyncRun which is not available in cluster, are deleted from DB.")

				err = dbq.GetSyncOperationById(ctx, &syncOperationDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				By("Verify that entries for the GitOpsDeploymentSyncRun which is available in cluster, are not deleted from DB.")

				err = dbq.GetSyncOperationById(ctx, &syncOperationDbTemp)
				Expect(err).To(BeNil())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDbTemp)
				Expect(err).To(BeNil())

				By("Verify that Operation for the GitOpsDeploymentSyncRun is created in cluster and DB.")

				var specialClusterUser db.ClusterUser
				err = dbq.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser)
				Expect(err).To(BeNil())

				var operationlist []db.Operation
				err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, syncOperationDbTemp.Application_id, db.OperationResourceType_SyncOperation, &operationlist, specialClusterUser.Clusteruser_id)
				Expect(err).To(BeNil())
				Expect(len(operationlist)).ShouldNot(Equal(0))

				objectMeta := metav1.ObjectMeta{
					Name:      sharedoperations.GenerateOperationCRName(operationlist[0]),
					Namespace: gitopsDeplSyncRunCr.Namespace,
				}
				k8sOperation := managedgitopsv1alpha1.Operation{ObjectMeta: objectMeta}

				err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: objectMeta.Namespace, Name: objectMeta.Name}, &k8sOperation)
				Expect(err).To(BeNil())
			})

			It("should delete related database entries from DB, if the GitOpsDeploymentSyncRun CR is present in cluster, but the UID doesn't match what is in the APICRToDatabaseMapping", func() {
				defer dbq.CloseDatabase()

				err := dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				// Create another SyncOperation DB entry
				syncOperationDb.SyncOperation_id = "test-sync-" + string(uuid.NewUUID())
				err = dbq.CreateSyncOperation(ctx, &syncOperationDb)
				Expect(err).To(BeNil())

				// Create another APICRToDatabaseMapping DB entry
				apiCRToDatabaseMappingDb.DBRelationKey = syncOperationDb.SyncOperation_id
				apiCRToDatabaseMappingDb.APIResourceUID = "test-" + string(uuid.NewUUID())
				err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
				Expect(err).To(BeNil())

				By("Call function for apiCrToDbMappingDbReconcile.")
				apiCRToDbMappingDbReconcile(ctx, dbq, k8sClient, nil, log)

				By("Verify that entries for the GitOpsDeploymentSyncRun which is not available in cluster, are deleted from DB.")

				err = dbq.GetSyncOperationById(ctx, &syncOperationDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())
			})
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
