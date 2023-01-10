package shared_resource_loop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1/mocks"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventloop_test_util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("SharedResourceEventLoop Test", func() {

	// This will be used by AfterEach to clean resources

	Context("Shared Resource Event Loop test", func() {

		var mockFactory MockSRLK8sClientFactory

		var k8sClient client.WithWatch
		var dbQueries db.AllDatabaseQueries
		var log logr.Logger
		var ctx context.Context
		var namespace *corev1.Namespace

		// Create a fake k8s client before each test
		BeforeEach(func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log = logf.FromContext(ctx)

			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				innerNamespace, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			namespace = innerNamespace

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			mockFactory = MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

		})

		AfterEach(func() {
			dbQueries.CloseDatabase()
		})

		verifyResult := func(managedEnv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, src SharedResourceManagedEnvContainer) {
			var err error
			apiCR := &db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:       string(managedEnv.UID),
				APIResourceName:      managedEnv.Name,
				APIResourceNamespace: managedEnv.Namespace,
				NamespaceUID:         string(namespace.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        "",
			}
			err = dbQueries.GetDatabaseMappingForAPICR(ctx, apiCR)
			Expect(err).To(BeNil())
			Expect(apiCR.DBRelationKey).ToNot(BeEmpty())

			managedEnvRow := &db.ManagedEnvironment{
				Managedenvironment_id: apiCR.DBRelationKey,
			}
			err = dbQueries.GetManagedEnvironmentById(ctx, managedEnvRow)
			Expect(err).To(BeNil())
			Expect(managedEnvRow.Clustercredentials_id).ToNot(BeEmpty())
			Expect(src.ManagedEnv.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			src.ManagedEnv.Created_on = managedEnvRow.Created_on
			Expect(src.ManagedEnv).To(Equal(managedEnvRow))

			clusterCreds := &db.ClusterCredentials{
				Clustercredentials_cred_id: managedEnvRow.Clustercredentials_id,
			}
			err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
			Expect(err).To(BeNil())
			Expect(clusterCreds.Host).ToNot(BeEmpty())
			Expect(clusterCreds.Serviceaccount_bearer_token).ToNot(BeEmpty())
			Expect(clusterCreds.Serviceaccount_ns).ToNot(BeEmpty())
		}

		It("should test ReconcileSharedManagedEnvironment: verify create, garbage cleanup, update, and delete, of ManagedEnvironment", func() {

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("calling managed environment for the first time, and verifying the database rows are created")

			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))

			verifyResult(managedEnv, src)

			By("calling reconcile on an unchanged resource")

			saList := corev1.ServiceAccountList{}
			err = k8sClient.List(ctx, &saList)
			Expect(err).To(BeNil())

			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			verifyResult(managedEnv, src)

			By("ensuring an old APICRToDatabaseMapping that previously had the same name/namespace, is deleted when the new one is reconciled")

			oldAPICRToDBMapping := &db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:       string(uuid.NewUUID()),
				APIResourceName:      managedEnv.Name,
				APIResourceNamespace: managedEnv.Namespace,
				NamespaceUID:         string(namespace.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        "test-doesnt-exist",
			}
			err = dbQueries.CreateAPICRToDatabaseMapping(ctx, oldAPICRToDBMapping)
			Expect(err).To(BeNil())

			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())

			// Update our copy of the ManagedEnvironment, since the call to reconcile will have added status to it.
			// This prevents an "object was modified" error when we update it.
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())

			By("updating the managed environment, and verifying that the database rows are also updated")

			oldClusterCreds := &db.ClusterCredentials{
				Clustercredentials_cred_id: src.ManagedEnv.Clustercredentials_id,
			}
			err = dbQueries.GetClusterCredentialsById(ctx, oldClusterCreds)
			Expect(err).To(BeNil())

			managedEnv.Spec.APIURL = "https://api2.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"
			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).To(BeNil())
			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())

			By("verifying the old cluster credentials have been deleted, after update")
			err = dbQueries.GetClusterCredentialsById(ctx, oldClusterCreds)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			By("verifying new cluster credentials exist containing the update")
			err = dbQueries.GetManagedEnvironmentById(ctx, src.ManagedEnv)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv.Clustercredentials_id).ToNot(BeEmpty())
			newClusterCreds := &db.ClusterCredentials{
				Clustercredentials_cred_id: src.ManagedEnv.Clustercredentials_id,
			}

			err = dbQueries.GetClusterCredentialsById(ctx, newClusterCreds)
			Expect(err).To(BeNil())
			Expect(newClusterCreds.Host).To(Equal(managedEnv.Spec.APIURL))

			By("deleting the managed environment, and verifying that the database rows are also removed")

			err = k8sClient.Delete(ctx, &managedEnv)
			Expect(err).To(BeNil())

			oldManagedEnv := src.ManagedEnv

			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())

			err = dbQueries.GetManagedEnvironmentById(ctx, oldManagedEnv)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			err = dbQueries.GetClusterCredentialsById(ctx, newClusterCreds)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			mappings := []db.APICRToDatabaseMapping{}
			err = dbQueries.UnsafeListAllAPICRToDatabaseMappings(ctx, &mappings)
			Expect(err).To(BeNil())

			By("Verifying that the API CR to database mapping has been removed.")
			for _, mapping := range mappings {
				Expect(mapping.APIResourceUID).ToNot(Equal(managedEnv.UID))
			}

		})

		It("should test the case where APICRMapping exists, but the managed env doesnt", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			apiCR := &db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:       string(managedEnv.UID),
				APIResourceName:      managedEnv.Name,
				APIResourceNamespace: managedEnv.Namespace,
				NamespaceUID:         string(namespace.UID),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        "does-not-exist",
			}

			err = dbQueries.CreateAPICRToDatabaseMapping(ctx, apiCR)
			Expect(err).To(BeNil())

			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).ToNot(BeNil())

			mappings := []db.APICRToDatabaseMapping{}
			err = dbQueries.UnsafeListAllAPICRToDatabaseMappings(ctx, &mappings)
			Expect(err).To(BeNil())

			for _, mapping := range mappings {
				Expect(mapping.DBRelationKey).To(Not(Equal("does-not-exist")))
			}

		})

		It("should set the condition ConnectionInitializationSucceeded status to True when the connection succeeded", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("calling ReconcileSharedManagedEnv")
			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))

			By("verifying the status condition")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonSucceeded))

			By("ensuring the LastTransitionTime is not updated if nothing has changed")
			lastTransitionTime := managedEnv.Status.Conditions[0].LastTransitionTime
			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].LastTransitionTime).To(Equal(lastTransitionTime))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonSucceeded))
		})

		It("should ensure the condition ConnectionInitializationSucceeded status is True when reconciling and nothing changed", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("calling ReconcileSharedManagedEnv")
			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))

			By("verifying the status condition")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonSucceeded))

			By("removing the status condition and reconciling")
			managedEnv.Status.Conditions = []metav1.Condition{}
			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).To(BeNil())
			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)

			By("ensuring the status condition is recreated")
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonSucceeded))

			By("setting the status condition false and reconciling")
			managedEnv.Status.Conditions[0].Status = metav1.ConditionFalse
			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).To(BeNil())
			src, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)

			By("ensuring the status condition is recreated")
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonSucceeded))
		})

		It("should set the condition ConnectionInitializationSucceeded status to False when the connection fails for new environment", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("simulating a complete failure to connect to the target cluster")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          1,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}

			By("calling reconcile to create  new managed env")
			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).ToNot(BeNil())
			Expect(mockFactory.count).To(Equal(1))

			By("ensuring the .status.condition is set to False")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonUnableToInstallServiceAccount))
		})

		It("should set the condition ConnectionInitializationSucceeded status to False when the connection fails for existing environment", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			By("next simulating a complete failure to connect to the target cluster")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          2,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).ToNot(BeNil())
			Expect(mockFactory.count).To(Equal(2))

			By("ensuring the .status.condition is set to False")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonUnableToInstallServiceAccount))
		})

		It("should test the case where we are unable to connect to a managed env, so new credentials are acquired, and old ones are deleted", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			oldClusterCredentials := firstSrc.ManagedEnv.Clustercredentials_id

			By("next simulating a failure to connect to the target cluster")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          1,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(src.ManagedEnv).ToNot(BeNil())
			Expect(mockFactory.count).To(Equal(1))
			Expect(firstSrc.ManagedEnv.Managedenvironment_id).To(Equal(src.ManagedEnv.Managedenvironment_id))
			Expect(src.ManagedEnv.Clustercredentials_id).ToNot(Equal(oldClusterCredentials))

			By("ensuring the .status.condition is set to True")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).To(BeNil())
			Expect(len(managedEnv.Status.Conditions)).To(Equal(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(ConditionReasonSucceeded))

			By("verifying the old credentials have been deleted, since we simulated them being invalid")
			clusterCreds := &db.ClusterCredentials{Clustercredentials_cred_id: oldClusterCredentials}
			err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))

		})

		It("should clean up Application database entries on managed environment deletion, and create operations for those", func() {

			_, _, _, instance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err = k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("first calling reconcile to create database entries for new managed env")
			createRC, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(createRC.ManagedEnv).ToNot(BeNil())

			applicationRow := &db.Application{
				Application_id:          "test-fake-application-id",
				Spec_field:              "{}",
				Name:                    "app-name",
				Engine_instance_inst_id: instance.Gitopsengineinstance_id,
				Managed_environment_id:  createRC.ManagedEnv.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, applicationRow)
			Expect(err).To(BeNil())

			clusterAccess := &db.ClusterAccess{
				Clusteraccess_user_id:                   createRC.ClusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    createRC.ManagedEnv.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: instance.Gitopsengineinstance_id,
			}
			err = dbQueries.CreateClusterAccess(ctx, clusterAccess)
			Expect(err).To(BeNil())

			By("deleting the managed env, calling reconcile, and verifying that all the database resources are cleaned up, plus operations are created")

			err = k8sClient.Delete(ctx, &managedEnv)
			Expect(err).To(BeNil())

			Expect(len(getAllOperationsForResourceID(ctx, applicationRow.Application_id, dbQueries))).To(Equal(0),
				"There should be no operations for this Application in the database before Reconcile is called.")

			Expect(len(getAllOperationsForResourceID(ctx, createRC.ManagedEnv.Managedenvironment_id, dbQueries))).To(Equal(0),
				"There should be no operations for this ManagedEnvironment in the database before Reconcile is called.")

			By("calling reconcile, after deleting the CR, to ensure the database entries are reconciled")
			deleteRC, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(deleteRC.ManagedEnv).To(BeNil())

			clusterAccesses := []db.ClusterAccess{}
			err = dbQueries.UnsafeListAllClusterAccess(ctx, &clusterAccesses)
			Expect(err).To(BeNil())
			for _, clusterAccess := range clusterAccesses {
				Expect(clusterAccess.Clusteraccess_managed_environment_id).ToNot(Equal(createRC.ManagedEnv.Managedenvironment_id),
					"there should exist no clusteraccess rows that reference the deleted managed environment")
			}
			clusterCred := &db.ClusterCredentials{
				Clustercredentials_cred_id: createRC.ManagedEnv.Clustercredentials_id,
			}
			Expect(db.IsResultNotFoundError(dbQueries.GetClusterCredentialsById(ctx, clusterCred))).To(BeTrue(), "cluster credential should be deleted.")

			err = dbQueries.GetApplicationById(ctx, applicationRow)
			Expect(err).To(BeNil())
			Expect(applicationRow.Managed_environment_id).To(BeEmpty())

			applicationOperations := getAllOperationsForResourceID(ctx, applicationRow.Application_id, dbQueries)
			Expect(len(applicationOperations)).To(Equal(1),
				"after Reconcile is called, there should be an Operation pointing to the Application, because the Application row was updated.")
			err = verifyOperationCRsExist(ctx, applicationOperations, k8sClient)
			Expect(err).To(BeNil())

			managedEnvOperations := getAllOperationsForResourceID(ctx, createRC.ManagedEnv.Managedenvironment_id, dbQueries)
			Expect(len(managedEnvOperations)).To(Equal(1),
				"after Reconcile is called, there should be an Operation pointing to the ManagedEnvironment, because the ManagedEnvironment row is deleted.")
			err = verifyOperationCRsExist(ctx, managedEnvOperations, k8sClient)
			Expect(err).To(BeNil())
		})

		It("should handle the case where a GitOpsDeploymentManagedEnvironment is created without a valid secret", func() {

			_, _, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err = k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			// err = k8sClient.Create(ctx, &secret)
			// Expect(err).To(BeNil())

			By("calling reconcile on the managed env, which is missing a secret")
			createRC, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(BeNil())
			Expect(createRC.ManagedEnv).To(BeNil())

		})

		It("should handle the case where a GitOpsDeploymentManagedEnvironment is created with a valid secret, but that secret is later deleted", func() {

			_, _, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err = k8sClient.Create(ctx, &managedEnv)
			Expect(err).To(BeNil())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("first calling reconcile to create database entries for new managed env")
			createRC, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(createRC.ManagedEnv).ToNot(BeNil())

			err = k8sClient.Delete(ctx, &secret)
			Expect(err).To(BeNil())

			By("call reconcile again, but without the cluster secret existing")
			createRC, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(BeNil())
			Expect(createRC.ManagedEnv).To(BeNil())

		})

		DescribeTable("Tests whether the tlsConfig value from managedEnv gets maped correctly into the database",
			func(tlsVerifyStatus bool) {
				managedEnv, secret := buildManagedEnvironmentForSRL()
				managedEnv.UID = "test-" + uuid.NewUUID()
				managedEnv.Spec.AllowInsecureSkipTLSVerify = tlsVerifyStatus
				secret.UID = "test-" + uuid.NewUUID()
				eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

				err := k8sClient.Create(ctx, &managedEnv)
				Expect(err).To(BeNil())

				err = k8sClient.Create(ctx, &secret)
				Expect(err).To(BeNil())

				By("first calling reconcile to create database entries for new managed env")
				firstSrc, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).To(BeNil())
				Expect(firstSrc.ManagedEnv).ToNot(BeNil())

				getClusterCredentials := firstSrc.ManagedEnv.Clustercredentials_id
				clusterCreds := &db.ClusterCredentials{Clustercredentials_cred_id: getClusterCredentials}
				err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
				Expect(err).To(BeNil())

				Expect(managedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(clusterCreds.AllowInsecureSkipTLSVerify))
			},
			Entry("TLS status set TRUE", bool(managedgitopsv1alpha1.TLSVerifyStatusTrue)),
			Entry("TLS status set FALSE", bool(managedgitopsv1alpha1.TLSVerifyStatusFalse)),
		)
	})

})

// verifyOperationCRsExist verifies there exists an Operation resource in the Argo CD namespace, for each row in 'expectedOperationRows' param.
func verifyOperationCRsExist(ctx context.Context, expectedOperationRows []db.Operation, k8sClient client.Client) error {

	operationList := &managedgitopsv1alpha1.OperationList{}
	if err := k8sClient.List(ctx, operationList); err != nil {
		return err
	}

	for _, expectedOperationRow := range expectedOperationRows {

		match := false
		for _, operation := range operationList.Items {
			if operation.Spec.OperationID == expectedOperationRow.Operation_id {
				match = true
				break
			}
		}
		Expect(match).To(BeTrue(), "All operations should have a matching Operation CR. No match for "+expectedOperationRow.Operation_id)
	}

	return nil
}

// getAllOperationsForApplication returns all Operation rows that point to a particular resource
// - resourceID parameter should be the primary key of an Application row, Operation row, etc
func getAllOperationsForResourceID(ctx context.Context, resourceID string, dbQueries db.AllDatabaseQueries) []db.Operation {
	res := []db.Operation{}
	operationsList := []db.Operation{}
	err := dbQueries.UnsafeListAllOperations(ctx, &operationsList)
	Expect(err).To(BeNil())
	for idx, operation := range operationsList {
		if operation.Resource_id == resourceID {
			res = append(res, operationsList[idx])
		}
	}
	return res
}

// func startServiceAccountListenerOnFakeClient(ctx context.Context, k8sClient client.Client) {
// 	go func() {

// 		var sa *corev1.ServiceAccount

// 		err := wait.Poll(time.Second*1, time.Hour*1, func() (bool, error) {

// 			saList := corev1.ServiceAccountList{}

// 			err := k8sClient.List(ctx, &saList, &client.ListOptions{Namespace: "kube-system"})
// 			Expect(err).To(BeNil())

// 			for idx := range saList.Items {

// 				item := saList.Items[idx]

// 				sa = &item

// 				if strings.HasPrefix(item.Name, sharedutil.ArgoCDManagerServiceAccountPrefix) {
// 					return true, nil
// 				}

// 			}

// 			return false, nil
// 		})
// 		Expect(err).To(BeNil())

// 		tokenSecret := &corev1.Secret{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      "token-secret",
// 				Namespace: "kube-system",
// 				Annotations: map[string]string{
// 					corev1.ServiceAccountNameKey: sa.Name,
// 				},
// 			},
// 			Data: map[string][]byte{"token": ([]byte)("token")},
// 			Type: corev1.SecretTypeServiceAccountToken,
// 		}
// 		err = k8sClient.Create(ctx, tokenSecret)
// 		Expect(err).To(BeNil())

// 		sa.Secrets = append(sa.Secrets, corev1.ObjectReference{
// 			Name:      tokenSecret.Name,
// 			Namespace: tokenSecret.Namespace,
// 		})

// 		err = k8sClient.Update(ctx, sa)
// 		Expect(err).To(BeNil())

// 	}()
// }

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

type SimulateFailingClientMockSRLK8sClientFactory struct {
	limit          int
	count          int
	failingClient  client.Client
	realFakeClient client.Client
}

func (f *SimulateFailingClientMockSRLK8sClientFactory) BuildK8sClient(restConfig *rest.Config) (client.Client, error) {
	GinkgoWriter.Println("SimulateFailingClientMockSRLK8sClientFactory call count:", f.count)
	if f.count < f.limit {
		f.count++
		return f.failingClient, nil
	}
	return f.realFakeClient, nil
}

func (f *SimulateFailingClientMockSRLK8sClientFactory) GetK8sClientForGitOpsEngineInstance(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
	return f.realFakeClient, nil
}

func (f *SimulateFailingClientMockSRLK8sClientFactory) GetK8sClientForServiceWorkspace() (client.Client, error) {
	return f.realFakeClient, nil
}

func buildManagedEnvironmentForSRL() (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {

	kubeConfigContents := generateFakeKubeConfig()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-managed-env-secret",
			Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
		},
		Type: sharedutil.ManagedEnvironmentSecretType,
		Data: map[string][]byte{
			"kubeconfig": ([]byte)(kubeConfigContents),
		},
	}

	managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-managed-env",
			Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
			ClusterCredentialsSecret: secret.Name,
		},
	}

	return *managedEnv, *secret
}

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
