package shared_resource_loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("SharedResourceEventLoop ManagedEnvironment-related Test", func() {

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
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log = logf.FromContext(ctx)

			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				innerNamespace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			namespace = innerNamespace

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			mockFactory = MockSRLK8sClientFactory{
				fakeClient: k8sClient,
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())
			Expect(apiCR.DBRelationKey).ToNot(BeEmpty())

			managedEnvRow := &db.ManagedEnvironment{
				Managedenvironment_id: apiCR.DBRelationKey,
			}
			err = dbQueries.GetManagedEnvironmentById(ctx, managedEnvRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnvRow.Clustercredentials_id).ToNot(BeEmpty())
			Expect(src.ManagedEnv.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			src.ManagedEnv.Created_on = managedEnvRow.Created_on
			Expect(src.ManagedEnv).To(Equal(managedEnvRow))

			clusterCreds := &db.ClusterCredentials{
				Clustercredentials_cred_id: managedEnvRow.Clustercredentials_id,
			}
			err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
			Expect(err).ToNot(HaveOccurred())
			Expect(clusterCreds.Host).ToNot(BeEmpty())
			Expect(clusterCreds.Serviceaccount_bearer_token).ToNot(BeEmpty())
			Expect(clusterCreds.Serviceaccount_ns).ToNot(BeEmpty())
		}

		DescribeTable("should test ReconcileSharedManagedEnvironment: verify create, garbage cleanup, update, and delete, of ManagedEnvironment",

			func(createNewServiceAccount bool) {

				By("creating ManagedEnvironment/Secret, either with or without createNewServiceAccount parameter")

				managedEnv, secret := buildManagedEnvironmentForSRLWithOptionalSA(createNewServiceAccount)
				managedEnv.UID = "test-" + uuid.NewUUID()
				secret.UID = "test-" + uuid.NewUUID()
				eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

				err := k8sClient.Create(ctx, &managedEnv)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Create(ctx, &secret)
				Expect(err).ToNot(HaveOccurred())

				By("calling reconcileSharedManagedEnv for the first time, and verifying the database rows are created")

				src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())
				Expect(src.ManagedEnv).To(Not(BeNil()))

				verifyResult(managedEnv, src)

				By("calling reconcile on an unchanged resource")

				saList := corev1.ServiceAccountList{}
				err = k8sClient.List(ctx, &saList)
				Expect(err).ToNot(HaveOccurred())

				src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())
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
				Expect(err).ToNot(HaveOccurred())

				src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())

				// Update our copy of the ManagedEnvironment, since the call to reconcile will have added status to it.
				// This prevents an "object was modified" error when we update it.
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
				Expect(err).ToNot(HaveOccurred())

				By("updating the managed environment, and verifying that the database rows are also updated")

				oldClusterCreds := &db.ClusterCredentials{
					Clustercredentials_cred_id: src.ManagedEnv.Clustercredentials_id,
				}
				err = dbQueries.GetClusterCredentialsById(ctx, oldClusterCreds)
				Expect(err).ToNot(HaveOccurred())

				managedEnv.Spec.APIURL = "https://api2.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"
				err = k8sClient.Update(ctx, &managedEnv)
				Expect(err).ToNot(HaveOccurred())
				src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())

				By("verifying the old cluster credentials have been deleted, after update")
				err = dbQueries.GetClusterCredentialsById(ctx, oldClusterCreds)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				By("verifying new cluster credentials exist containing the update")
				err = dbQueries.GetManagedEnvironmentById(ctx, src.ManagedEnv)
				Expect(err).ToNot(HaveOccurred())
				Expect(src.ManagedEnv.Clustercredentials_id).ToNot(BeEmpty())
				newClusterCreds := &db.ClusterCredentials{
					Clustercredentials_cred_id: src.ManagedEnv.Clustercredentials_id,
				}

				err = dbQueries.GetClusterCredentialsById(ctx, newClusterCreds)
				Expect(err).ToNot(HaveOccurred())
				Expect(newClusterCreds.Host).To(Equal(managedEnv.Spec.APIURL))

				By("deleting the managed environment, and verifying that the database rows are also removed")

				err = k8sClient.Delete(ctx, &managedEnv)
				Expect(err).ToNot(HaveOccurred())

				oldManagedEnv := src.ManagedEnv

				src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())

				err = dbQueries.GetManagedEnvironmentById(ctx, oldManagedEnv)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				err = dbQueries.GetClusterCredentialsById(ctx, newClusterCreds)
				Expect(db.IsResultNotFoundError(err)).To(BeTrue())

				mappings := []db.APICRToDatabaseMapping{}
				err = dbQueries.UnsafeListAllAPICRToDatabaseMappings(ctx, &mappings)
				Expect(err).ToNot(HaveOccurred())

				By("Verifying that the API CR to database mapping has been removed.")
				for _, mapping := range mappings {
					Expect(mapping.APIResourceUID).ToNot(Equal(string(managedEnv.UID)))
				}

			},
			Entry("createNewServiceAccount = true", true),
			Entry("createNewServiceAccount = false", false))

		It("should test the case where APICRMapping exists, but the managed env doesnt", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).ToNot(BeNil())

			mappings := []db.APICRToDatabaseMapping{}
			err = dbQueries.UnsafeListAllAPICRToDatabaseMappings(ctx, &mappings)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling ReconcileSharedManagedEnv")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).To(Not(BeNil()))

			By("verifying the status condition")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonSucceeded)))

			By("ensuring the LastTransitionTime is not updated if nothing has changed")
			lastTransitionTime := managedEnv.Status.Conditions[0].LastTransitionTime
			src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].LastTransitionTime).To(Equal(lastTransitionTime))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonSucceeded)))
		})

		It("should ensure the condition ConnectionInitializationSucceeded status is True when reconciling and nothing changed", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling ReconcileSharedManagedEnv")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).To(Not(BeNil()))

			By("verifying the status condition")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonSucceeded)))

			By("removing the status condition and reconciling")
			managedEnv.Status.Conditions = []metav1.Condition{}
			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)

			By("ensuring the status condition is recreated")
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonSucceeded)))

			By("setting the status condition false and reconciling")
			managedEnv.Status.Conditions[0].Status = metav1.ConditionFalse
			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)

			By("ensuring the status condition is recreated")
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).To(Not(BeNil()))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonSucceeded)))
		})

		It("should set the condition ConnectionInitializationSucceeded status to False when the connection fails for new environment", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

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

			By("calling reconcile to create new managed env")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(mockFactory.count).To(Equal(1))

			By("ensuring the .status.condition is set to False")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToInstallServiceAccount)))
		})

		It("should set the condition ConnectionInitializationSucceeded status to False when the connection fails for existing environment", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			By("next simulating a complete failure to connect to the target cluster")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          2,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(mockFactory.count).To(Equal(2))

			By("ensuring the .status.condition is set to False")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToInstallServiceAccount)))
		})

		It("should set the condition ConnectionInitializationSucceeded appropriately when the connection fails because of insufficient permissions to list all namespaces", func() {
			managedEnv, secret := buildManagedEnvironmentForSRLWithOptionalSA(false)
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			By("next simulating a 'forbidden' error when attempting to list all namespaces")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			forbidden := k8serrors.NewForbidden(schema.GroupResource{Group: "", Resource: "namespaces"}, "", fmt.Errorf("user can't access namespaces"))
			mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(forbidden)
			mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(forbidden)

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          3,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(mockFactory.count).To(Equal(3))

			By("ensuring the .status.condition is set appropriately")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionUnknown))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToValidateClusterCredentials)))
			Expect(managedEnv.Status.Conditions[0].Message).To(Equal("Provided service account does not have permission to access resources in the cluster. Verify that the service account has the correct Role and RoleBinding."))
		})

		It("should set the condition ConnectionInitializationSucceeded appropriately when the connection fails because the cluster certificate is signed by an unknown authority", func() {
			managedEnv, secret := buildManagedEnvironmentForSRLWithOptionalSA(false)
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			By("next simulating a 'cert signed by unknown authority' error when attempting to list all namespaces")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			certError := fmt.Errorf("Get \"https://mycluster.example.com:6443/api?timeout=32s\": x509: certificate signed by unknown authority")
			mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(certError)
			mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(certError)

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          3,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(mockFactory.count).To(Equal(3))

			By("ensuring the .status.condition is set appropriately")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionUnknown))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToValidateClusterCredentials)))
			Expect(managedEnv.Status.Conditions[0].Message).To(Equal("Certificate signed by unknown authority. Note that the '.spec.allowInsecureSkipTLSVerify' field can be used to ignore this error."))
		})

		It("should set the condition ConnectionInitializationSucceeded appropriately when the connection fails because of insufficient permissions to get a particular namespaces", func() {
			managedEnv, secret := buildManagedEnvironmentForSRLWithOptionalSA(false)
			managedEnv.Spec.Namespaces = []string{namespace.Name}
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			By("next simulating a 'forbidden' error when attempting to get the specific namespace")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			forbidden := k8serrors.NewForbidden(schema.GroupResource{Group: "", Resource: "namespace"}, "", fmt.Errorf("user can't access namespace"))
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(forbidden)
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(forbidden)

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          3,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(mockFactory.count).To(Equal(3))

			By("ensuring the .status.condition is set appropriately")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionUnknown))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonUnableToValidateClusterCredentials)))
			Expect(managedEnv.Status.Conditions[0].Message).To(Equal("Provided service account does not have permission to access resources in the cluster. Verify that the service account has the correct Role and RoleBinding."))
		})

		It("should test the case where we are unable to connect to a managed env, so new credentials are acquired, and old ones are deleted", func() {
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			firstSrc, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(firstSrc.ManagedEnv).ToNot(BeNil())

			oldClusterCredentials := firstSrc.ManagedEnv.Clustercredentials_id

			By("next simulating a failure to connect to the target cluster")
			mockCtrl := gomock.NewController(GinkgoT())
			defer mockCtrl.Finish()
			mockClient := mocks.NewMockClient(mockCtrl)

			mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake unable to connect"))

			mockFactory := &SimulateFailingClientMockSRLK8sClientFactory{
				limit:          1,
				failingClient:  mockClient,
				realFakeClient: k8sClient,
			}
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).ToNot(BeNil())
			Expect(mockFactory.count).To(Equal(1))
			Expect(firstSrc.ManagedEnv.Managedenvironment_id).To(Equal(src.ManagedEnv.Managedenvironment_id))
			Expect(src.ManagedEnv.Clustercredentials_id).ToNot(Equal(oldClusterCredentials))

			By("ensuring the .status.condition is set to True")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(managedEnv.Status.Conditions).To(HaveLen(1))
			Expect(managedEnv.Status.Conditions[0].Type).To(Equal(managedgitopsv1alpha1.ManagedEnvironmentStatusConnectionInitializationSucceeded))
			Expect(managedEnv.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(managedEnv.Status.Conditions[0].Reason).To(Equal(string(managedgitopsv1alpha1.ConditionReasonSucceeded)))

			By("verifying the old credentials have been deleted, since we simulated them being invalid")
			clusterCreds := &db.ClusterCredentials{Clustercredentials_cred_id: oldClusterCredentials}
			err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})

		It("should clean up Application database entries on managed environment deletion, and create operations for those", func() {

			_, _, engineCluster, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())
			instance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-instance-id",
				Namespace_name:          "gitops-service-argocd",
				Namespace_uid:           "test-fake-instance-namespace-914",
				EngineCluster_id:        engineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, instance)
			Expect(err).ToNot(HaveOccurred())

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err = k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			createRC, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(createRC.ManagedEnv).ToNot(BeNil())

			applicationRow := &db.Application{
				Application_id:          "test-fake-application-id",
				Spec_field:              "{}",
				Name:                    "app-name",
				Engine_instance_inst_id: instance.Gitopsengineinstance_id,
				Managed_environment_id:  createRC.ManagedEnv.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, applicationRow)
			Expect(err).ToNot(HaveOccurred())

			clusterAccess := &db.ClusterAccess{
				Clusteraccess_user_id:                   createRC.ClusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    createRC.ManagedEnv.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: instance.Gitopsengineinstance_id,
			}
			err = dbQueries.CreateClusterAccess(ctx, clusterAccess)
			Expect(err).ToNot(HaveOccurred())

			By("deleting the managed env, calling reconcile, and verifying that all the database resources are cleaned up, plus operations are created")

			err = k8sClient.Delete(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			Expect(getAllOperationsForResourceID(ctx, applicationRow.Application_id, dbQueries)).To(BeEmpty(),
				"There should be no operations for this Application in the database before Reconcile is called.")

			Expect(getAllOperationsForResourceID(ctx, createRC.ManagedEnv.Managedenvironment_id, dbQueries)).To(BeEmpty(),
				"There should be no operations for this ManagedEnvironment in the database before Reconcile is called.")

			By("calling reconcile, after deleting the CR, to ensure the database entries are reconciled")
			deleteRC, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(deleteRC.ManagedEnv).To(BeNil())

			clusterAccesses := []db.ClusterAccess{}
			err = dbQueries.UnsafeListAllClusterAccess(ctx, &clusterAccesses)
			Expect(err).ToNot(HaveOccurred())
			for _, clusterAccess := range clusterAccesses {
				Expect(clusterAccess.Clusteraccess_managed_environment_id).ToNot(Equal(createRC.ManagedEnv.Managedenvironment_id),
					"there should exist no clusteraccess rows that reference the deleted managed environment")
			}
			clusterCred := &db.ClusterCredentials{
				Clustercredentials_cred_id: createRC.ManagedEnv.Clustercredentials_id,
			}
			Expect(db.IsResultNotFoundError(dbQueries.GetClusterCredentialsById(ctx, clusterCred))).To(BeTrue(), "cluster credential should be deleted.")

			err = dbQueries.GetApplicationById(ctx, applicationRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationRow.Managed_environment_id).To(BeEmpty())

			applicationOperations := getAllOperationsForResourceID(ctx, applicationRow.Application_id, dbQueries)
			Expect(applicationOperations).To(HaveLen(1),
				"after Reconcile is called, there should be an Operation pointing to the Application, because the Application row was updated.")
			err = verifyOperationCRsExist(ctx, applicationOperations, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			managedEnvOperations := getAllOperationsForResourceID(ctx, createRC.ManagedEnv.Managedenvironment_id, dbQueries)
			Expect(managedEnvOperations).To(HaveLen(1),
				"after Reconcile is called, there should be an Operation pointing to the ManagedEnvironment, because the ManagedEnvironment row is deleted.")
			err = verifyOperationCRsExist(ctx, managedEnvOperations, k8sClient)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle the case where a GitOpsDeploymentManagedEnvironment is created without a valid secret", func() {

			_, _, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err = k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcile on the managed env, which is missing a secret")
			createRC, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(isUserErr).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(createRC.ManagedEnv).To(BeNil())

		})

		It("should handle the case where a GitOpsDeploymentManagedEnvironment is created with a valid secret, but that secret is later deleted", func() {

			_, _, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err = k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("first calling reconcile to create database entries for new managed env")
			createRC, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(createRC.ManagedEnv).ToNot(BeNil())

			err = k8sClient.Delete(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("call reconcile again, but without the cluster secret existing")
			createRC, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(createRC.ManagedEnv).To(BeNil())

		})

		It("should reconcile a ManagedEnvironment containing .spec.namespaces and .spec.clusterResources values", func() {

			managedEnv, secret := buildManagedEnvironmentForSRL()

			managedEnv.Spec.Namespaces = []string{"c", "b", "a"}
			managedEnv.Spec.ClusterResources = true

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcile to create database entries for new managed env")
			createRC, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(createRC.ManagedEnv).ToNot(BeNil())

			By("ensuring cluster credentials should have expected values")
			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: createRC.ManagedEnv.Clustercredentials_id,
			}

			err = dbQueries.GetClusterCredentialsById(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())
			Expect(clusterCredentials.Namespaces).To(Equal("a,b,c"), "should match values from managed env")
			Expect(clusterCredentials.ClusterResources).To(BeTrue(), "should match values from managed env")

			By("updating the namespace/clusterresources values on the managedenv .spec, to ensure the change is applied")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			managedEnv.Spec.ClusterResources = false
			managedEnv.Spec.Namespaces = []string{}

			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("call the reconcile function again")
			createRC, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(createRC.ManagedEnv).ToNot(BeNil())

			By("ensuring cluster credentials should have new expected values")
			clusterCredentials = db.ClusterCredentials{
				Clustercredentials_cred_id: createRC.ManagedEnv.Clustercredentials_id,
			}

			err = dbQueries.GetClusterCredentialsById(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())
			Expect(clusterCredentials.Namespaces).To(Equal(""), "should match values from managed env")
			Expect(clusterCredentials.ClusterResources).To(BeFalse(), "should match values from managed env")

		})

		It("should produce a useful error message if the user in the kubeconfig doesn't have a token", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfigWithoutToken()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
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
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())

			// Find the root error
			for tmp := err; tmp != nil; tmp = errors.Unwrap(tmp) {
				err = tmp
			}
			Expect(err.Error()).To(HavePrefix("kubeconfig must have a service account token for the user in context"))
			Expect(err.Error()).To(HaveSuffix("client-certificate is not supported at this time."))
		})

		It("should produce an error if the secret is missing", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")
			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}
			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                  "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
					CreateNewServiceAccount: false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("no secret specified by managed environment 'test-my-managed-env' in 'gitops-service-argocd'"))
		})

		It("should produce an error if the secret is of the wrong type", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")
			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: corev1.SecretType("wrong"),
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
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
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("invalid secret type: wrong"))
		})

		It("should produce an error if the secret is missing the kubeconfig field", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{},
			}
			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
					ClusterCredentialsSecret: secret.Name,
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("missing kubeconfig field in Secret"))
		})

		It("should produce an error if the kubeconfig field in the secret is invalid", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)("BADBADBAD"),
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
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("unable to parse kubeconfig data: "))
		})

		It("should produce error if it is unable to locate the appropriate context in the kubeconfig", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}
			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://example.com:6443",
					ClusterCredentialsSecret: secret.Name,
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(isUserErr).To(BeTrue())
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			// Find the root error
			for tmp := err; tmp != nil; tmp = errors.Unwrap(tmp) {
				err = tmp
			}
			Expect(err.Error()).To(HavePrefix("the kubeconfig did not have a cluster entry that matched the API URL 'https://example.com:6443"))
		})

		It("should produce an error if the kubeconfig doesn't have auth info", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfigWithoutAuthInfo()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
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
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())

			// Find the root error
			for tmp := err; tmp != nil; tmp = errors.Unwrap(tmp) {
				err = tmp
			}
			Expect(err.Error()).To(Equal("unable to extract remote cluster configuration from kubeconfig, missing auth info for default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin"))
		})

		It("should produce a error if the namespaces field in a new managed environment contains an invalid namespace", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
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
					CreateNewServiceAccount:  false,
					Namespaces:               []string{"BAD"},
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(err.Error()).To(HavePrefix("unable to create managed environment for test-"))
			Expect(err.Error()).To(ContainSubstring("user specified an invalid namespace: ManagedEnvironment contains an invalid namespace in namespaces list: BAD"))
		})

		It("should produce a error if the namespaces field in an existing managed environment is updated to contain an invalid namespace", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
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
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring the managed environment already exists before updating it with an invalid namespace")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(isUserErr).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
			Expect(src.ManagedEnv).ToNot(BeNil())

			By("updating the existing managed environment with an invalid namespace")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: managedEnv.Namespace, Name: managedEnv.Name}, managedEnv)
			Expect(err).ToNot(HaveOccurred())
			managedEnv.Spec.Namespaces = []string{"BAD"}
			err = k8sClient.Update(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")
			src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(err.Error()).To(Equal("user specified an invalid namespace: ManagedEnvironment contains an invalid namespace in namespaces list: BAD"))
		})

		It("should produce a error if unable to create cluster credentials for the managed environment", func() {
			By("creating ManagedEnvironment/Secret")

			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}

			By("creating a managed environment which has enough namespaces to overflow the cluster credential namespace field length")
			namespaces := []string{}
			for i := 0; i < 66; i++ {
				namespaces = append(namespaces, strings.Repeat("a", 63))
			}
			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
					ClusterCredentialsSecret: secret.Name,
					CreateNewServiceAccount:  true,
					Namespaces:               namespaces,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(err.Error()).To(ContainSubstring("unable to create cluster credentials for host 'https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443'"))
			Expect(err.Error()).To(ContainSubstring("Namespaces value exceeds maximum size: max: 4096, actual: 4223"))
		})

		It("should produce an error if the kubeconfig doesn't have a context for the specified cluster", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfigWithoutContext()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}
			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api2.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
					ClusterCredentialsSecret: secret.Name,
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(isUserErr).To(BeTrue())
			Expect(err).To(HaveOccurred())
			// Find the root error
			for tmp := err; tmp != nil; tmp = errors.Unwrap(tmp) {
				err = tmp
			}
			Expect(err.Error()).To(Equal("the kubeconfig did not have a context that matched the cluster specified in the API URL of the GitOpsDeploymentManagedEnvironment. Context was expected to reference cluster 'api2-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443'"))
		})

		It("should produce an error condition if the APIURL contains forbidden characters", func() {
			By("creating ManagedEnvironment/Secret, without creating a new ServiceAccount")

			kubeConfigContents := generateFakeKubeConfig()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					KubeconfigKey: ([]byte)(kubeConfigContents),
				},
			}
			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env",
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443?",
					ClusterCredentialsSecret: secret.Name,
					CreateNewServiceAccount:  false,
				},
			}

			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling reconcileSharedManagedEnv, which should produce the error")

			src, condition, isUserErr, err := internalProcessMessage_internalReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(src.ManagedEnv).To(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeTrue())
			Expect(condition.message).To(Equal("the API URL must not have ? or & values"))
			Expect(condition.reason).To(Equal(managedgitopsv1alpha1.ConditionReasonUnsupportedAPIURL))
		})

		It("should test whether appProjectEnvironment is created and deleted based on managedEnv, and also verifies that a AppProjectManagedEnvironment that is owned by the same user is not deleted.", func() {

			By("create ManagedEnvironment and Secret for it")
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).ToNot(BeNil())

			By("Create DB entry for ClusterCredentials")
			clusterCredentialsDb := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-" + string(uuid.NewUUID()),
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: db.DefaultServiceaccount_bearer_token,
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentialsDb)
			Expect(err).ToNot(HaveOccurred())

			By("Create DB entry for ManagedEnvironment")
			managedEnvironmentDb := db.ManagedEnvironment{
				Managedenvironment_id: "test-" + string(uuid.NewUUID()),
				Clustercredentials_id: clusterCredentialsDb.Clustercredentials_cred_id,
				Name:                  "test-" + string(uuid.NewUUID()),
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
			Expect(err).ToNot(HaveOccurred())

			By("Creating an AppProjectManagedEnvironment to verify the deletion logic, when AppProjectManagedEnvironment is owned by the same user is not deleted.")
			appProjectManagedEnv := db.AppProjectManagedEnvironment{
				AppprojectManagedenvID: "test-app-managedenv-id-1",
				Managed_environment_id: managedEnvironmentDb.Managedenvironment_id,
				Clusteruser_id:         src.ClusterUser.Clusteruser_id,
			}
			err = dbQueries.CreateAppProjectManagedEnvironment(ctx, &appProjectManagedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("Verify whether appProjectManagedEnv is created or not")
			getAppProjectManagedEnvDB := db.AppProjectManagedEnvironment{
				Clusteruser_id:         src.ClusterUser.Clusteruser_id,
				Managed_environment_id: src.ManagedEnv.Managedenvironment_id,
			}

			err = dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &getAppProjectManagedEnvDB)
			Expect(err).ToNot(HaveOccurred())

			By("By deleting the managed env CR, and verifying that the appProjectManagedEnv row is deleted")
			err = k8sClient.Delete(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).To(BeNil())

			err = dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &getAppProjectManagedEnvDB)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no rows in result set"))

			By("Verify that another unrelated AppProjectManagedEnvironment, which is owned by the same user, is not deleted.")
			err = dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnv)
			Expect(err).ToNot(HaveOccurred())

		})

		It("should test whether appProjectEnvironment exists when ManagedEnv is updated", func() {

			By("creating ManagedEnv, and a Secret for it")
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling ReconcileSharedManagedEnvironment")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).ToNot(BeNil())

			err = dbQueries.GetManagedEnvironmentById(ctx, src.ManagedEnv)
			Expect(err).ToNot(HaveOccurred(), "it should still exist")

			By("updating the human readable name of the ManagedEnvironment")
			src.ManagedEnv.Name = "test-updated-name"
			err = dbQueries.UpdateManagedEnvironment(ctx, src.ManagedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("verifying the AppProjectManagedEnvironment still exists")
			appProjectManagedEnvDB := db.AppProjectManagedEnvironment{
				Clusteruser_id:         src.ClusterUser.Clusteruser_id,
				Managed_environment_id: src.ManagedEnv.Managedenvironment_id,
			}

			err = dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvDB)
			Expect(err).ToNot(HaveOccurred())

		})

		It("should test whether appProjectEnvironment does not exist; if it doesn't, then create it while updating managedEnv Cr.", func() {

			By("creating ManagedEnvironment and Secret")
			managedEnv, secret := buildManagedEnvironmentForSRL()
			managedEnv.UID = "test-" + uuid.NewUUID()
			secret.UID = "test-" + uuid.NewUUID()
			eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

			err := k8sClient.Create(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("calling ReconcileSharedManagedEnv")
			src, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())
			Expect(src.ManagedEnv).ToNot(BeNil())

			By("verifying the AppProjectManagedEnvironment exists")
			appProjectManagedEnvDB := db.AppProjectManagedEnvironment{
				Clusteruser_id:         src.ClusterUser.Clusteruser_id,
				Managed_environment_id: src.ManagedEnv.Managedenvironment_id,
			}
			err = dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvDB)
			Expect(err).ToNot(HaveOccurred())

			By("Delete existing AppProjectManagedEnvironment to allow us to test the non existance of AppProjectManagedEnvironment")
			row, err := dbQueries.DeleteAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvDB)
			Expect(err).ToNot(HaveOccurred())
			Expect(row).To(Equal(1))

			By("Update the GitOpsDeploymentManagedEnvironment object with a new URL")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: managedEnv.Namespace, Name: managedEnv.Name}, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			managedEnv.Spec.APIURL = "https://api2.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"
			err = k8sClient.Update(ctx, &managedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("calling ReconcileSharedManagedEnv and verifying that the ManagedEnvironment row was created")
			src, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
				false, *namespace, mockFactory, dbQueries, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(isUserErr).To(BeFalse())

			err = dbQueries.GetManagedEnvironmentById(ctx, src.ManagedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("verifying whether AppProject is created or not")
			appProjectManagedEnvDB = db.AppProjectManagedEnvironment{
				Clusteruser_id:         src.ClusterUser.Clusteruser_id,
				Managed_environment_id: src.ManagedEnv.Managedenvironment_id,
			}

			err = dbQueries.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvDB)
			Expect(err).ToNot(HaveOccurred())

		})

		DescribeTable("Tests whether the tlsConfig value from managedEnv gets mapped correctly into the database",
			func(tlsVerifyStatus bool) {
				managedEnv, secret := buildManagedEnvironmentForSRL()
				managedEnv.UID = "test-" + uuid.NewUUID()
				managedEnv.Spec.AllowInsecureSkipTLSVerify = tlsVerifyStatus
				secret.UID = "test-" + uuid.NewUUID()
				eventloop_test_util.StartServiceAccountListenerOnFakeClient(ctx, string(managedEnv.UID), k8sClient)

				err := k8sClient.Create(ctx, &managedEnv)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Create(ctx, &secret)
				Expect(err).ToNot(HaveOccurred())

				By("first calling reconcile to create database entries for new managed env")
				reconcileRes, isUserErr, err := internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())
				Expect(reconcileRes.ManagedEnv).ToNot(BeNil())

				clusterCreds := &db.ClusterCredentials{Clustercredentials_cred_id: reconcileRes.ManagedEnv.Clustercredentials_id}
				err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
				Expect(err).ToNot(HaveOccurred())

				Expect(managedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(clusterCreds.AllowInsecureSkipTLSVerify))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnv), &managedEnv)
				Expect(err).ToNot(HaveOccurred())

				By("updating allow insecure tls verify, and confirming the database value changes as well")
				managedEnv.Spec.AllowInsecureSkipTLSVerify = !tlsVerifyStatus

				err = k8sClient.Update(ctx, &managedEnv)
				Expect(err).ToNot(HaveOccurred())

				By("calling reconcile again to ensure the managed environment db entry is updated with the new value")
				reconcileRes, isUserErr, err = internalProcessMessage_ReconcileSharedManagedEnv(ctx, k8sClient, managedEnv.Name, managedEnv.Namespace,
					false, *namespace, mockFactory, dbQueries, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(isUserErr).To(BeFalse())
				Expect(reconcileRes.ManagedEnv).ToNot(BeNil())

				clusterCreds = &db.ClusterCredentials{Clustercredentials_cred_id: reconcileRes.ManagedEnv.Clustercredentials_id}
				err = dbQueries.GetClusterCredentialsById(ctx, clusterCreds)
				Expect(err).ToNot(HaveOccurred())

				Expect(managedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(clusterCreds.AllowInsecureSkipTLSVerify))

			},
			Entry("TLS status set TRUE", bool(managedgitopsv1alpha1.TLSVerifyStatusTrue)),
			Entry("TLS status set FALSE", bool(managedgitopsv1alpha1.TLSVerifyStatusFalse)),
		)
	})

	Context("Unit tests for individual pure functions", func() {

		DescribeTable("Verify that isValidNamespaceName conforms to K8s namespace requirements",
			func(namespace string, expectedResult bool) {
				res := isValidNamespaceName(namespace)
				Expect(res).To(Equal(expectedResult))
			},
			Entry("empty namespace", "", false),
			Entry("short non-empty namespace", "a", true),
			Entry("63 character long should be valid", strings.Repeat("a", 63), true),
			Entry("64 character long should be invalid", strings.Repeat("a", 64), false),
			Entry("hyphens are valid in the middle", "a-a", true),
			Entry("hyphens are not valid as a prefix", "-a", false),
			Entry("hyphens are not valid as a suffix", "a-", false),
			Entry("numbers are value", "123", true),
			Entry("must be lowercase", "A", false),
			Entry("other characters are invalid", "invalid_characters", false),
		)

		DescribeTable("Verify that convertManagedEnvNamespacesFieldToCommaSeparatedList correctly converts a string slice to comma-separated list, rejecting invalid namespaces",
			func(namespaceSlice []string, expectedResult string, expectError bool) {
				res, err := convertManagedEnvNamespacesFieldToCommaSeparatedList(namespaceSlice)
				Expect(res).To(Equal(expectedResult))
				Expect(err != nil).To(Equal(expectError))
			},
			Entry("empty namespace", []string{}, "", false),
			Entry("a single valid namespace", []string{"a"}, "a", false),
			Entry("a couple valid namespaces", []string{"a", "b"}, "a,b", false),
			Entry("a couple valid namespaces in reverse order", []string{"b", "a"}, "a,b", false),
			Entry("a valid namespace, one invalid namespace", []string{"B", "a"}, "", true),
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
	Expect(err).ToNot(HaveOccurred())
	for idx, operation := range operationsList {
		if operation.Resource_id == resourceID {
			res = append(res, operationsList[idx])
		}
	}
	return res
}

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

// Build a managed environment object for shared resource loop (SRL) test
func buildManagedEnvironmentForSRL() (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {
	return buildManagedEnvironmentForSRLWithOptionalSA(true)
}

func buildManagedEnvironmentForSRLWithOptionalSA(createNewServiceAccount bool) (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {

	kubeConfigContents := generateFakeKubeConfig()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-managed-env-secret",
			Namespace: "test-k8s-namespace",
		},
		Type: sharedutil.ManagedEnvironmentSecretType,
		Data: map[string][]byte{
			KubeconfigKey: ([]byte)(kubeConfigContents),
		},
	}

	managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-managed-env",
			Namespace: "test-k8s-namespace",
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			APIURL:                   "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443",
			ClusterCredentialsSecret: secret.Name,
			CreateNewServiceAccount:  createNewServiceAccount,
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

func generateFakeKubeConfigWithoutToken() string {
	// This config has been sanitized of any real credentials.
	return `
apiVersion: v1
kind: Config
clusters:
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
contexts:
  - context:
      cluster: api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
current-context: default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
preferences: {}
users:
  - name: kube:admin/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      client-certificate-data: Rk9PCg==
      client-key-data: QkFSCg==
`

}

func generateFakeKubeConfigWithoutAuthInfo() string {
	// This config has been sanitized of any real credentials.
	return `
apiVersion: v1
kind: Config
clusters:
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
contexts:
  - context:
      cluster: api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
current-context: default/api-fake-unit-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
preferences: {}
`
}

func generateFakeKubeConfigWithoutContext() string {
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
