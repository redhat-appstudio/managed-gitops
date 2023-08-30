package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("RepoCred Reconcile Function Tests", func() {
	Context("Testing reconcileRepositoryCredentials function for RepositoryCredentials table entries.", func() {

		var log logr.Logger
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var clusterUserDb *db.ClusterUser
		var gitopsEngineInstance *db.GitopsEngineInstance
		var apiCRToDatabaseMappingDb db.APICRToDatabaseMapping
		var gitopsRepositoryCredentialsDb db.RepositoryCredentials

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			By("Create required DB entries.")

			_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			// Create DB entry for ClusterUser
			clusterUserDb = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUserDb)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			// Create DB entry for APICRToDatabaseMapping
			apiCRToDatabaseMappingDb = db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
				APIResourceUID:       "test-" + string(uuid.NewUUID()),
				APIResourceName:      "test-" + string(uuid.NewUUID()),
				APIResourceNamespace: "test-" + string(uuid.NewUUID()),
				NamespaceUID:         "test-" + string(uuid.NewUUID()),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential,
				DBRelationKey:        gitopsRepositoryCredentialsDb.RepositoryCredentialsID,
			}

			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeploymentRepositoryCredentialCR := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      apiCRToDatabaseMappingDb.APIResourceName,
					Namespace: apiCRToDatabaseMappingDb.APIResourceNamespace,
				},
			}
			// Create GitOpsDeployment CR in cluster
			err = k8sClient.Create(context.Background(), gitopsDeploymentRepositoryCredentialCR)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should reconcile status for RepositoryCredentials if the entry is present in RepositoryCredentials DB", func() {

			defer dbq.CloseDatabase()

			By("Call Reconcile function.")

			reconcileRepositoryCredentials(ctx, dbq, k8sClient, log)

			By("Verify that status is updated for GitopsDeploymentRepositoryCredentialCR.")
			objectMeta := metav1.ObjectMeta{
				Name:      apiCRToDatabaseMappingDb.APIResourceName,
				Namespace: apiCRToDatabaseMappingDb.APIResourceNamespace,
			}
			repoCredCR := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{ObjectMeta: objectMeta}

			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(repoCredCR), repoCredCR)
			Expect(err).ToNot(HaveOccurred())
			Expect(repoCredCR).NotTo(BeNil())
			Expect(repoCredCR.Status.Conditions).To(HaveLen(3))

		})
	})
})
