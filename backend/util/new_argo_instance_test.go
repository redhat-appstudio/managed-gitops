package util

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Test for creating opeartion with resource-type as Gitopsengineinstance ", func() {

	Context("New Argo Instance test", func() {

		var k8sClient client.Client
		var dbQueries db.AllDatabaseQueries
		var log logr.Logger
		var ctx context.Context

		var clusterUserID string

		// Create a fake k8s client before each test
		BeforeEach(func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log = logf.FromContext(ctx)

			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, _, clusterAccess, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			clusterUserID = clusterAccess.Clusteraccess_user_id

		})

		AfterEach(func() {
			dbQueries.CloseDatabase()
		})

		It("tests whether the operation pointing to gitopsengineinstance resource type gets created successfully", func() {

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-new-gitopsengineinstance",
					UID:  "test-new-gitopsengineinstance-uuid",
				},
			}

			err := k8sClient.Create(context.Background(), ns)
			Expect(err).ToNot(HaveOccurred())

			clusterUser := db.ClusterUser{Clusteruser_id: clusterUserID}
			err = dbQueries.GetClusterUserById(ctx, &clusterUser)
			Expect(err).ToNot(HaveOccurred())

			err = CreateNewArgoCDInstance(ctx, ns, clusterUser, k8sClient, log, dbQueries)
			Expect(err).ToNot(HaveOccurred())

			operationList := managedgitopsv1alpha1.OperationList{}
			err = k8sClient.List(context.Background(), &operationList)
			Expect(err).ToNot(HaveOccurred())

			By("looking for an Operation that points to the new GitOpsEngineInstance we created")
			matchFound := false

			for _, operation := range operationList.Items {

				operationDB := &db.Operation{
					Operation_id: operation.Spec.OperationID,
				}

				err = dbQueries.GetOperationById(context.Background(), operationDB)
				Expect(err).ToNot(HaveOccurred())

				if operationDB.Resource_type == db.OperationResourceType_GitOpsEngineInstance {

					gitopsEngineInstanceDB := db.GitopsEngineInstance{
						Gitopsengineinstance_id: operationDB.Resource_id,
					}

					err := dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstanceDB)
					Expect(err).ToNot(HaveOccurred())

					if gitopsEngineInstanceDB.Namespace_name == ns.Name {
						matchFound = true
					}

				}
			}

			Expect(matchFound).To(BeTrue(), "an operation pointing to a gitopsengineinstance should exist, and the gitopsengineinstance should matches the test namespace")

		})
	})
})
