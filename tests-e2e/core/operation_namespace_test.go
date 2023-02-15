package core

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	dboperations "github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Operation CR namespace E2E tests", func() {

	const (
		operationNamespace = "gitops-service-argocd"
		operationName      = "test-operation"
		falseNamespace     = "fake-operation-namespace"
	)

	Context("Operation CR in invalid namespace should be ignored", func() {
		var err error
		var ctx context.Context

		var dbQueries db.AllDatabaseQueries
		var k8sClient client.Client
		var config *rest.Config
		var gitopsEngineInstance *db.GitopsEngineInstance
		var operationDB *db.Operation
		var operationCR *managedgitopsv1alpha1.Operation

		BeforeEach(func() {

			// By("Delete old namespaces, and kube-system resources")
			// Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("deleting the namespace before the test starts, so that the code can create it")
			config, err = fixture.GetSystemKubeConfig()
			if err != nil {
				panic(err)
			}
			err = fixture.DeleteNamespace(operationNamespace, config)
			Expect(err).To(BeNil())
			err = fixture.DeleteNamespace(falseNamespace, config)
			Expect(err).To(BeNil())
			ctx = context.Background()
			config, err := fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())
			k8sClient, err = fixture.GetKubeClient(config)
			Expect(err).To(BeNil())
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())
			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

		})
		AfterEach(func() {
			log := log.FromContext(ctx)
			err = operations.CleanupOperation(ctx, *operationDB, *operationCR, operationCR.Namespace, dbQueries, k8sClient, true, log)
			Expect(err).To(BeNil())
			err = fixture.DeleteNamespace(falseNamespace, config)
			Expect(err).To(BeNil())
		})

		It("should create Operation CR and namespace, the OperationCR.namespace created should match Argocd namespace ", func() {

			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			By("creating Opeartion CR")
			log := log.FromContext(ctx)
			namespaceToCreate := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: operationNamespace,
					UID:  uuid.NewUUID(),
				},
			}
			kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
			err = k8s.Get(kubeSystemNamespace, k8sClient)

			Expect(err).To(Succeed())

			gitopsEngineInstance, _, _, err = dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespaceToCreate, string(kubeSystemNamespace.UID), dbQueries, log)

			operationDB = &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: "test-user",
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, namespaceToCreate)
			Expect(err).To(BeNil())

			operationCR = &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: gitopsEngineInstance.Namespace_name,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: "test-operation",
				},
			}
			err = k8sClient.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			err = wait.PollImmediate(time.Second*1, time.Minute*1, func() (done bool, err error) {

				fmt.Println("Attempting to fetch Operation state: ", operationDB.State)
				isComplete, err := dboperations.IsOperationComplete(ctx, operationDB, dbQueries)
				fmt.Println("- Operation state result achieved : ", isComplete)
				return isComplete, err

			})
			Expect(err).To(BeNil())
			Eventually(operationCR, "60s", "5s").Should(k8s.ExistByName(k8sClient))
			Expect(err).To(BeNil())

		})
		It("should create Operation CR and namespace, the OperationCR.namespace created should not match Argocd namespace ", func() {

			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}
			log := log.FromContext(ctx)

			By("creating Opeartion CR")
			namespaceToCreate := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: operationNamespace,
					UID:  uuid.NewUUID(),
				},
			}

			kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
			err = k8s.Get(kubeSystemNamespace, k8sClient)
			Expect(err).To(Succeed())
			gitopsEngineInstance, _, _, err = dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespaceToCreate, string(kubeSystemNamespace.UID), dbQueries, log)

			operationDB = &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: "test-user",
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())
			falseNamespaceToCreate := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: falseNamespace,
				},
			}

			err = k8sClient.Create(ctx, falseNamespaceToCreate)
			Expect(err).To(BeNil())

			operationCR = &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fake-operation",
					Namespace: falseNamespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: "test-operation",
				},
			}
			err = k8sClient.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			err = wait.PollImmediate(time.Second*3, time.Minute*1, func() (done bool, err error) {

				fmt.Println("Attempting to fetch Operation state: ", operationDB.State)
				isComplete, err := dboperations.IsOperationComplete(ctx, operationDB, dbQueries)
				fmt.Println("- Operation state result achieved : ", isComplete)
				return isComplete, err

			})
			Expect(err).ToNot(BeNil())

			// Even if the operation failed to complete, it will exist in the namespace hence confirming that
			// this operation was successfully ignored by gitops-service
			Eventually(operationCR, "60s", "5s").Should(k8s.ExistByName(k8sClient))

		})
	})
})
