package argoprojio

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/argoproj.io/application_info_cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var _ = Describe("Namespace Reconciler Tests.", func() {
	var reconciler ApplicationReconciler

	Context("Testing for Namespace Reconciler.", func() {
		It("Should consider ArgoCD Application as an orphaned and delete it, if application entry doesnt exists in DB.", func() {
			ctx := context.Background()
			log := logger.FromContext(ctx)

			scheme, _, _, _, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := ApplicationReconciler{Client: k8sClient}

			argoApplications := []appv1.Application{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-2"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-3"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-4"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"databaseID": "test-my-application-5"}}},
			}

			processedApplicationIds := map[string]any{"test-my-application-3": false, "test-my-application-5": false}

			deletedArgoApplications := cleanOrphanedCRsfromCluster_Applications(argoApplications, processedApplicationIds, ctx, reconciler.Client, log)

			Expect(deletedArgoApplications).To(HaveLen(3))

			deletedApplicationIds := map[string]string{"test-my-application-1": "", "test-my-application-2": "", "test-my-application-4": ""}
			for _, app := range deletedArgoApplications {
				_, ok := deletedApplicationIds[app.Labels["databaseID"]]
				Expect(ok).To(BeTrue())
			}
		})
	})

	Context("Testing syncCRsWithDB_Applications_Delete_Operations function", func() {
		var err error
		var dbQueries db.AllDatabaseQueries
		var ctx context.Context
		var operationList []db.Operation
		var argoCdApp appv1.Application
		var dummyApplicationSpec string
		var applicationput db.Application

		BeforeEach(func() {
			ctx = context.Background()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			_, dummyApplicationSpec, argoCdApp, err = createDummyApplicationData()
			Expect(err).ToNot(HaveOccurred())

			applicationput = db.Application{
				Application_id:          "test-my-application",
				Name:                    "test-my-application",
				Spec_field:              dummyApplicationSpec,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, &applicationput)
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			reconciler = ApplicationReconciler{
				Client: k8sClient,
				DB:     dbQueries,
				Cache:  application_info_cache.NewApplicationInfoCache(),
			}

			err = reconciler.Create(ctx, &argoCdApp)
			Expect(err).ToNot(HaveOccurred())

			var speCialClusterUser db.ClusterUser
			err = dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &speCialClusterUser)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			for _, operation := range operationList {
				rowsAffected, err := dbQueries.CheckedDeleteOperationById(ctx, operation.Operation_id, operation.Operation_owner_user_id)
				Expect(rowsAffected).Should((Equal(1)))
				Expect(err).ToNot(HaveOccurred())
			}
			// Empty Operation List
			operationList = []db.Operation{}
		})

		It("Should delete Operations from cluster and if operation is completed.", func() {

			ctx := context.Background()
			log := logger.FromContext(ctx)

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, dbOperation, err := operations.CreateOperation(ctx, false, dbOperationInput,
				db.SpecialClusterUserName, "argocd", reconciler.DB, reconciler.Client, log)
			Expect(err).ToNot(HaveOccurred())

			dbOperation.State = "Completed"
			err = dbQueries.UpdateOperation(ctx, dbOperation)
			Expect(err).ToNot(HaveOccurred())

			operationList = append(operationList, *dbOperation)

			// Get list of Operations before cleanup.
			listOfK8sOperationFirst := managedgitopsv1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfK8sOperationFirst.Items).NotTo(BeEmpty())

			// Clean Operations
			syncCRsWithDB_Applications_Delete_Operations(ctx, dbQueries, reconciler.Client, log)

			// Get list of Operations after cleanup.
			listOfK8sOperationSecond := managedgitopsv1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationSecond)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfK8sOperationSecond.Items).To(BeEmpty())
		})

		It("Should not delete Operations from cluster and if operation is not completed.", func() {

			ctx := context.Background()
			log := logger.FromContext(ctx)

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, dbOperation, err := operations.CreateOperation(ctx, false, dbOperationInput,
				db.SpecialClusterUserName, "argocd", reconciler.DB, reconciler.Client, log)
			Expect(err).ToNot(HaveOccurred())

			operationList = append(operationList, *dbOperation)

			// Get list of Operations before cleanup.
			listOfK8sOperationFirst := managedgitopsv1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfK8sOperationFirst.Items).NotTo(BeEmpty())

			// Clean Operations
			syncCRsWithDB_Applications_Delete_Operations(ctx, dbQueries, reconciler.Client, log)

			// Get list of Operations after cleanup.
			listOfK8sOperationSecond := managedgitopsv1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationSecond)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfK8sOperationSecond.Items).NotTo(BeEmpty())
		})
	})

	Context("Testing for cleanOrphanedCRsfromCluster_Secret function.", func() {
		var log logr.Logger
		var ctx context.Context
		var secret corev1.Secret
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var clusterCredentials db.ClusterCredentials
		var gitopsEngineInstance db.GitopsEngineInstance

		BeforeEach(func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			scheme, _, _, _, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			// Fake kube client.
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			kubeSystemNamepace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
					UID:  "test-kube-system",
				},
			}
			err = k8sClient.Create(ctx, &kubeSystemNamepace)
			Expect(err).ToNot(HaveOccurred())

			gitopsEngineCluster, created, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubeSystemNamepace.UID), dbq, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(created).To(BeTrue())

			By("Create required db entries.")
			clusterCredentials = db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: db.DefaultServiceaccount_bearer_token,
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			gitopsEngineInstance = db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			By("Create Secret CR.")
			secret = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-secret",
					Namespace: gitopsEngineInstance.Namespace_name,
				},
			}
		})

		It("Should not delete repository secret CR from cluster, if entry pointed by secret is present in RepositoryCredentials table.", func() {

			defer dbq.CloseDatabase()

			clusterUser := &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err := dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())

			By("Create a RepositoryCredentials DB entry.")

			repoCredentials := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-cred-id" + string(uuid.NewUUID()),
				UserID:                  clusterUser.Clusteruser_id,
				PrivateURL:              "https://test-private-url",
				AuthUsername:            "test-auth-username",
				AuthPassword:            "test-auth-password",
				AuthSSHKey:              "test-auth-ssh-key",
				SecretObj:               "test-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbq.CreateRepositoryCredentials(ctx, &repoCredentials)
			Expect(err).ToNot(HaveOccurred())

			By("Create secret in cluster with label pointing to DB entry that exists.")

			secret.Labels = map[string]string{
				SecretDbIdentifierKey:                    repoCredentials.RepositoryCredentialsID,
				sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretRepoTypeValue,
			}

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify RepositoryCredentials DB entry still exists.")

			_, err = dbq.GetRepositoryCredentialsByID(ctx, repoCredentials.RepositoryCredentialsID)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should delete repository secret from cluster, if entry pointed by secret is not present in RepositoryCredentials table.", func() {

			defer dbq.CloseDatabase()

			By("Create secret in cluster with label pointing to DB entry that does not exist.")

			secret.Labels = map[string]string{
				SecretDbIdentifierKey:                    "test-cred-id" + string(uuid.NewUUID()),
				sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretRepoTypeValue,
			}

			err := k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify repository secret from cluster is deleted.")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("Should not delete cluster secret from cluster, if entry pointed by secret is present in ManagedEnvironment table.", func() {

			defer dbq.CloseDatabase()

			By("Create a ManagedEnvironment DB entry.")

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-env-id" + string(uuid.NewUUID()),
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "test-env-" + string(uuid.NewUUID()),
			}

			err := dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			By("Create secret in cluster with label pointing to DB entry that exists.")

			secret.Labels = map[string]string{
				SecretDbIdentifierKey:                    managedEnvironment.Managedenvironment_id,
				sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretClusterTypeValue,
			}

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify ManagedEnvironment DB entry still exists.")

			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should delete cluster secret from cluster, if entry pointed by secret is not present in ManagedEnvironment table.", func() {

			defer dbq.CloseDatabase()

			By("Create secret in cluster with label pointing to DB entry that does not exist.")

			secret.Labels = map[string]string{
				SecretDbIdentifierKey:                    "test-env-id" + string(uuid.NewUUID()),
				sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretClusterTypeValue,
			}

			err := k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify cluster secret from cluster is deleted.")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("Should not delete cluster secret from cluster, if it does have required label.", func() {

			defer dbq.CloseDatabase()

			By("Create secret in cluster with one label.")

			err := k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify cluster secret from cluster is deleted.")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should not delete cluster secret from cluster, if it does have secret-type label.", func() {

			defer dbq.CloseDatabase()

			By("Create secret in cluster with one label.")

			secret.Labels = map[string]string{SecretDbIdentifierKey: "test-env-id" + string(uuid.NewUUID())}

			err := k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify cluster secret from cluster is deleted.")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should not delete cluster secret from cluster, if it does have databaseID label.", func() {

			defer dbq.CloseDatabase()

			By("Create secret in cluster with one label.")

			secret.Labels = map[string]string{sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretClusterTypeValue}

			err := k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Call cleanOrphanedCRsfromCluster_Secret function.")

			cleanOrphanedCRsfromCluster_Secret(ctx, dbq, k8sClient, log)

			By("Verify cluster secret from cluster is deleted.")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Testing for cleanOrphanedCRsfromCluster_Operation function.", func() {
		var log logr.Logger
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var k8sOperationFirst managedgitopsv1alpha1.Operation
		var gitopsEngineInstance *db.GitopsEngineInstance

		BeforeEach(func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			scheme, _, _, _, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			// Fake kube client.
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			By("Create Secret CR.")

			// Create K8s operation CR
			k8sOperationFirst = managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-op-" + string(uuid.NewUUID()),
					Namespace: "test-ns-1",
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: "test-id-" + string(uuid.NewUUID()),
				},
			}
		})

		It("Should delete Operation if it doesn't point to a DB entry.", func() {
			k8sOperationSecond := k8sOperationFirst

			// Create 1st CR in cluster
			err := k8sClient.Create(ctx, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			// Create 2nd CR in cluster
			k8sOperationSecond.Name = "test-op-" + string(uuid.NewUUID())
			k8sOperationSecond.Spec.OperationID = "test-id-" + string(uuid.NewUUID())
			err = k8sClient.Create(ctx, &k8sOperationSecond)
			Expect(err).ToNot(HaveOccurred())

			By("Calling cleanOrphanedCRsfromCluster_Operation function to delete orphaned Operation CR, if corresponding DB entry is not present.")

			cleanOrphanedCRsfromCluster_Operation(ctx, dbq, k8sClient, log)

			By("Verify that orphaned Operation CRs without a DB entry are deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationFirst.Name, Namespace: k8sOperationFirst.Namespace}, &k8sOperationFirst)
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationSecond.Name, Namespace: k8sOperationSecond.Namespace}, &k8sOperationSecond)
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("Should not delete an Operation if it points to a valid DB entry but operation is in 'Waiting' state.", func() {
			k8sOperationSecond := k8sOperationFirst

			// Create 1st CR in cluster
			err := k8sClient.Create(ctx, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			// Create 2nd CR in cluster
			k8sOperationSecond.Name = "test-op-" + string(uuid.NewUUID())
			k8sOperationSecond.Spec.OperationID = "test-id-" + string(uuid.NewUUID())
			err = k8sClient.Create(ctx, &k8sOperationSecond)
			Expect(err).ToNot(HaveOccurred())

			// Create DB entry for 1st CR.
			dbOperation := db.Operation{
				Operation_id:            k8sOperationFirst.Spec.OperationID,
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-" + string(uuid.NewUUID()),
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateOperation(ctx, &dbOperation, dbOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			// Create DB entry for 2nd CR.
			operationSecond := dbOperation
			operationSecond.Operation_id = k8sOperationSecond.Spec.OperationID
			operationSecond.Resource_id = "test-" + string(uuid.NewUUID())
			err = dbq.CreateOperation(ctx, &operationSecond, operationSecond.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			By("Calling cleanOrphanedCRsfromCluster_Operation function to delete orphaned Operation CR, if corresponding DB entry is not present.")

			cleanOrphanedCRsfromCluster_Operation(ctx, dbq, k8sClient, log)

			By("Verify that Operation CRs with a valid DB entry are not deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationFirst.Name, Namespace: k8sOperationFirst.Namespace}, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationSecond.Name, Namespace: k8sOperationSecond.Namespace}, &k8sOperationSecond)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should delete only those Operation which don't point to a valid DB entry.", func() {
			k8sOperationSecond := k8sOperationFirst

			// Create 1st CR in cluster
			err := k8sClient.Create(ctx, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			// Create 2nd CR in cluster
			k8sOperationSecond.Name = "test-op-" + string(uuid.NewUUID())
			k8sOperationSecond.Spec.OperationID = "test-id-" + string(uuid.NewUUID())
			err = k8sClient.Create(ctx, &k8sOperationSecond)
			Expect(err).ToNot(HaveOccurred())

			// Create DB entry for 1st CR
			dbOperation := db.Operation{
				Operation_id:            k8sOperationFirst.Spec.OperationID,
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-" + string(uuid.NewUUID()),
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateOperation(ctx, &dbOperation, dbOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			By("Calling cleanOrphanedCRsfromCluster_Operation function to delete orphaned Operation CR, if corresponding DB entry is not present.")

			cleanOrphanedCRsfromCluster_Operation(ctx, dbq, k8sClient, log)

			By("Verify that Operation CR with valid DB entry is not deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationFirst.Name, Namespace: k8sOperationFirst.Namespace}, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			By("Verify that Operation CR without valid DB entry is deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationSecond.Name, Namespace: k8sOperationSecond.Namespace}, &k8sOperationSecond)
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("Verify that DB entry for Operation CR is not deleted.")

			err = dbq.GetOperationById(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should delete an Operation if it points to a valid DB entry, but 'created_on' is more than waitTimeForK8sResourceDelete and 'State' is set to 'Completed'.", func() {
			// Create CR in cluster
			err := k8sClient.Create(ctx, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			// Create DB entry for CR
			dbOperation := db.Operation{
				Operation_id:            k8sOperationFirst.Spec.OperationID,
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-" + string(uuid.NewUUID()),
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateOperation(ctx, &dbOperation, dbOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetOperationById(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())

			// Change "Created_on" & "State" field using UpdateOperation function since CreateOperation does not allow to insert custom "Created_on" and "State" field.

			// Set "State" to "Completed"
			dbOperation.State = db.OperationState_Completed

			// Set "Created_on" field to > waitTimeForK8sResourceDelete
			dbOperation.Created_on = time.Now().Add(-1 * (waitTimeForK8sResourceDelete + 1*time.Second))

			err = dbq.UpdateOperation(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())

			By("Calling cleanOrphanedCRsfromCluster_Operation function to delete orphaned Operation CR, if corresponding DB entry is not present.")

			cleanOrphanedCRsfromCluster_Operation(ctx, dbq, k8sClient, log)

			By("Verify that Operation CRs with a valid DB entry but marked as Completed is deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationFirst.Name, Namespace: k8sOperationFirst.Namespace}, &k8sOperationFirst)
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("Should not delete an Operation if it points to a valid DB entry, but 'created_on' is more than waitTimeForK8sResourceDelete and 'State' is not set to 'Completed'.", func() {
			err := k8sClient.Create(ctx, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			dbOperation := db.Operation{
				Operation_id:            k8sOperationFirst.Spec.OperationID,
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-" + string(uuid.NewUUID()),
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateOperation(ctx, &dbOperation, dbOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			// Change "Created_on" field using UpdateOperation function since CreateOperation does not allow to insert custom "Created_on" field.

			err = dbq.GetOperationById(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())

			// Set "Created_on" field to > waitTimeForK8sResourceDelete
			dbOperation.Created_on = time.Now().Add(-1 * (waitTimeForK8sResourceDelete + 1*time.Second))
			err = dbq.UpdateOperation(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())

			By("Calling cleanOrphanedCRsfromCluster_Operation function to delete orphaned Operation CR, if corresponding DB entry is not present.")

			cleanOrphanedCRsfromCluster_Operation(ctx, dbq, k8sClient, log)

			By("Verify that Operation CRs is not deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationFirst.Name, Namespace: k8sOperationFirst.Namespace}, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should not delete an Operation if it points to a valid DB entry, but 'created_on' is less than waitTimeForK8sResourceDelete and 'State' is set to 'Completed'.", func() {
			err := k8sClient.Create(ctx, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())

			dbOperation := db.Operation{
				Operation_id:            k8sOperationFirst.Spec.OperationID,
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-" + string(uuid.NewUUID()),
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbq.CreateOperation(ctx, &dbOperation, dbOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			// Change "State" field using UpdateOperation function since CreateOperation does not allow to insert custom "State" field.

			err = dbq.GetOperationById(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())

			// Set "State" to "Completed"
			dbOperation.State = db.OperationState_Completed
			err = dbq.UpdateOperation(ctx, &dbOperation)
			Expect(err).ToNot(HaveOccurred())

			By("Calling cleanOrphanedCRsfromCluster_Operation function to delete orphaned Operation CR, if corresponding DB entry is not present.")

			cleanOrphanedCRsfromCluster_Operation(ctx, dbq, k8sClient, log)

			By("Verify that Operation CRs is not deleted.")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: k8sOperationFirst.Name, Namespace: k8sOperationFirst.Namespace}, &k8sOperationFirst)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Testing for recreateClusterSecrets function.", func() {

		var log logr.Logger
		var ctx context.Context
		var secret corev1.Secret
		var application db.Application
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var kubeSystemNamepace corev1.Namespace
		var managedEnvironment db.ManagedEnvironment

		var clusterAccess db.ClusterAccess

		BeforeEach(func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log = logger.FromContext(ctx)

			dbq, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			scheme, _, _, _, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			By("Create fake kube client.")

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			By("Create kube-system namespace.")

			kubeSystemNamepace = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
					UID:  "test-" + uuid.NewUUID(),
				},
			}
			err = k8sClient.Create(ctx, &kubeSystemNamepace)
			Expect(err).ToNot(HaveOccurred())

			By("Create GitopsEngineCluster.")

			gitopsEngineCluster, created, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubeSystemNamepace.UID), dbq, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(created).To(BeTrue())

			By("Create ClusterCredentials entry in DB.")

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-creds-" + string(uuid.NewUUID()),
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "fake service account bearer token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			By("Create ManagedEnvironment entry in DB.")

			managedEnvironment = db.ManagedEnvironment{
				Managedenvironment_id: "test-env-" + string(uuid.NewUUID()),
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my env",
			}
			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			By("Create GitopsEngineInstance entry in DB.")

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-id" + string(uuid.NewUUID()),
				Namespace_name:          "test-ns-" + string(uuid.NewUUID()),
				Namespace_uid:           "test-ns-" + string(uuid.NewUUID()),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application entry in DB.")

			application = db.Application{
				Application_id:          "test-id" + string(uuid.NewUUID()),
				Name:                    "test-name" + string(uuid.NewUUID()),
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			By("Create ClusterAccess entry in DB.")

			clusterAccess = db.ClusterAccess{
				Clusteraccess_user_id:                   "test-user",
				Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbq.CreateClusterAccess(ctx, &clusterAccess)
			Expect(err).ToNot(HaveOccurred())

			By("Create Secret CR definition.")

			secret = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argosharedutil.GenerateArgoCDClusterSecretName(managedEnvironment),
					Namespace: gitopsEngineInstance.Namespace_name,
				},
			}
		})

		It("Should not create Operation for Secret, even if it is missing in Cluster, since ManagedEnvironment is created recently.", func() {

			defer dbq.CloseDatabase()

			By("Get list of Operations before calling function.")

			var operation []db.Operation

			err := dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)
			Expect(err).ToNot(HaveOccurred())
			Expect(operation).To(BeEmpty())

			By("Call function to recreate Secret if missing from cluster.")

			recreateClusterSecrets(ctx, dbq, k8sClient, log)

			By("Get list of Operations after calling function.")

			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)
			Expect(err).ToNot(HaveOccurred())
			Expect(operation).To(BeEmpty())
		})

		It("Should not create Operation for Secret, since it is already present in Cluster.", func() {

			defer dbq.CloseDatabase()

			By("Set 'Created_on' field more than 30 Minutes.")

			managedEnvironment.Created_on = time.Now().Add(time.Duration(-(31 * time.Minute)))
			err := dbq.UpdateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			By("Create Secret CR in cluster.")

			err = k8sClient.Create(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			By("Get list of Operations before calling function.")

			var operation []db.Operation

			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)
			Expect(err).ToNot(HaveOccurred())
			Expect(operation).To(BeEmpty())

			By("Call function to recreate Secret if missing from cluster.")

			recreateClusterSecrets(ctx, dbq, k8sClient, log)

			By("Get list of Operations after calling function.")

			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)
			Expect(err).ToNot(HaveOccurred())
			Expect(operation).To(BeEmpty())
		})

		It("Should create Operation to recreate Secret, since it is not present in Cluster.", func() {

			defer dbq.CloseDatabase()

			By("Set 'Created_on' field more than 30 Minutes.")

			managedEnvironment.Created_on = time.Now().Add(time.Duration(-(31 * time.Minute)))
			Expect(dbq.UpdateManagedEnvironment(ctx, &managedEnvironment)).To(Succeed())

			By("Get list of Operations before calling function.")

			var operation []db.Operation

			Expect(dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)).To(Succeed())
			Expect(operation).To(BeEmpty())

			By("Call function to recreate Secret if missing from cluster.")

			recreateClusterSecrets(ctx, dbq, k8sClient, log)

			By("Get list of Operations after calling function.")

			Expect(dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)).To(Succeed())
			Expect(operation).To(HaveLen(1))
		})

		It("Should not create Operation to recreate Secret, if the ManagedEnvironment is deploying to the same cluster as Argo CD", func() {

			rowsDeleted, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.Clusteraccess_user_id, clusterAccess.Clusteraccess_managed_environment_id, clusterAccess.Clusteraccess_gitops_engine_instance_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsDeleted).To(Equal(1))

			By("Create new ClusterCredentials entry in DB, with default 'Serviceaccount_bearer_token' field")

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-creds-" + string(uuid.NewUUID()),
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: db.DefaultServiceaccount_bearer_token,
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			Expect(dbq.CreateClusterCredentials(ctx, &clusterCredentials)).To(Succeed())

			By("Set 'Created_on' field more than 30 Minutes.")
			By("updating the existing ManagedEnvironment to point to the new credentials")
			managedEnvironment.Clustercredentials_id = clusterCredentials.Clustercredentials_cred_id
			managedEnvironment.Created_on = time.Now().Add(time.Duration(-(31 * time.Minute)))
			Expect(dbq.UpdateManagedEnvironment(ctx, &managedEnvironment)).To(Succeed())

			By("Get list of Operations before calling function.")

			var operation []db.Operation

			Expect(dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)).To(Succeed())
			Expect(operation).To(BeEmpty())

			By("Call function to recreate Secret if missing from cluster.")

			recreateClusterSecrets(ctx, dbq, k8sClient, log)

			By("Get list of Operations after calling function.")

			Expect(dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, application.Application_id, db.OperationResourceType_Application, &operation, db.SpecialClusterUserName)).To(Succeed())
			Expect(operation).To(BeEmpty(), "no operations should have been created, because the managed environment's cluster credential has a default value in the Serviceaccount_bearer_token field. In this case, an Argo CD Cluster Secret does not need to exist, because Argo CD always has the ability to deploy to the local cluster.")
		})
	})
})
