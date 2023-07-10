package shared_resource_loop

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Used to list down resources for deletion which are created while running tests.
type testResources struct {
	clusterAccess           *db.ClusterAccess
	Clusteruser_id          string
	Managedenvironment_id   string
	Gitopsengineinstance_id string
	EngineCluster_id        string
	Clustercredentials_id   []string
	RepositoryCredential_id string
	AppProjectRepository    *db.AppProjectRepository
	OperationID             string
}

var _ = Describe("SharedResourceEventLoop Test", func() {

	// This will be used by AfterEach to clean resources
	var resourcesToBeDeleted testResources

	var ctx context.Context
	var k8sClient *sharedutil.ProxyClient
	var namespace *v1.Namespace

	l := log.FromContext(context.Background())

	Context("Shared Resource Event Loop test", func() {

		// Create a fake k8s client before each test
		BeforeEach(func() {
			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespaceTemp, err := tests.GenericTestSetup()

			Expect(err).To(BeNil())

			namespace = namespaceTemp

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, namespace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			// After each test delete the resources created by it.
			DeferCleanup(func() {
				dbq, err := db.NewUnsafePostgresDBQueries(true, true)
				Expect(err).To(BeNil())

				defer dbq.CloseDatabase()

				// Delete RepositoryCredential
				if resourcesToBeDeleted.RepositoryCredential_id != "" {
					rowsAffected, err := dbq.DeleteRepositoryCredentialsByID(ctx, resourcesToBeDeleted.RepositoryCredential_id)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete Operation
				if resourcesToBeDeleted.OperationID != "" {
					rowsAffected, err := dbq.DeleteOperationById(ctx, resourcesToBeDeleted.OperationID)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete clusterAccess
				if resourcesToBeDeleted.clusterAccess != nil {
					rowsAffected, err := dbq.DeleteClusterAccessById(ctx,
						resourcesToBeDeleted.clusterAccess.Clusteraccess_user_id,
						resourcesToBeDeleted.clusterAccess.Clusteraccess_managed_environment_id,
						resourcesToBeDeleted.clusterAccess.Clusteraccess_gitops_engine_instance_id)

					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete clusterUser
				if resourcesToBeDeleted.Clusteruser_id != "" {
					rowsAffected, err := dbq.DeleteClusterUserById(ctx, resourcesToBeDeleted.Clusteruser_id)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete managedEnv
				if resourcesToBeDeleted.Managedenvironment_id != "" {
					rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, resourcesToBeDeleted.Managedenvironment_id)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete engineInstance
				if resourcesToBeDeleted.Gitopsengineinstance_id != "" {
					rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, resourcesToBeDeleted.Gitopsengineinstance_id)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete engineCluster
				if resourcesToBeDeleted.EngineCluster_id != "" {
					rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, resourcesToBeDeleted.EngineCluster_id)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

				// Delete clusterCredentials
				if len(resourcesToBeDeleted.Clustercredentials_id) != 0 {
					for _, clustercredentials_id := range resourcesToBeDeleted.Clustercredentials_id {
						rowsAffected, err := dbq.DeleteClusterCredentialsById(ctx, clustercredentials_id)
						Expect(rowsAffected).To(Equal(1))
						Expect(err).To(BeNil())
					}
				}

				// Delete AppProjectRepository
				if resourcesToBeDeleted.AppProjectRepository != nil {
					rowsAffected, err := dbq.DeleteAppProjectRepositoryByClusterUserAndRepoURL(ctx, resourcesToBeDeleted.AppProjectRepository)
					Expect(rowsAffected).To(Equal(1))
					Expect(err).To(BeNil())
				}

			})
		})

		It("Should create or fetch a user by Namespace id.", func() {

			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// At first assuming there are no existing users, hence creating new.
			usrOld,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(usrOld).NotTo(BeNil())
			Expect(isNewUser).To(BeTrue())

			// User is created in previous call, then same user should be returned instead of creating new.
			usrNew,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(usrNew).NotTo(BeNil())
			Expect(isNewUser).To(BeFalse())

			By("verify whether the created_on field is within the last 5 minutes")
			Expect(usrNew.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(usrOld.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			// old user should be exactly similar to new user
			Expect(usrOld).To(Equal(usrNew))

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{Clusteruser_id: usrNew.Clusteruser_id}
		})

		It("Should create or fetch resources.", func() {
			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// At first assuming there are no existing resources, hence creating new.
			sharedResourceOld, err := sharedResourceEventLoop.ReconcileSharedManagedEnv(ctx, k8sClient, *namespace, "", "",
				true, MockSRLK8sClientFactory{fakeClient: k8sClient}, l)

			Expect(err).To(BeNil())
			Expect(sharedResourceOld.ClusterUser).NotTo(BeNil())
			Expect(sharedResourceOld.ManagedEnv).NotTo(BeNil())
			Expect(sharedResourceOld.GitopsEngineInstance).NotTo(BeNil())
			Expect(sharedResourceOld.ClusterAccess).NotTo(BeNil())

			Expect(sharedResourceOld.IsNewUser).To(BeTrue())
			Expect(sharedResourceOld.IsNewManagedEnv).To(BeTrue())
			Expect(sharedResourceOld.IsNewInstance).To(BeTrue())
			Expect(sharedResourceOld.IsNewClusterAccess).To(BeTrue())

			// Resources are created in previous call, then same resources should be returned instead of creating new.
			sharedResourceNew, err := sharedResourceEventLoop.ReconcileSharedManagedEnv(ctx, k8sClient, *namespace, "", "",
				true, MockSRLK8sClientFactory{fakeClient: k8sClient}, l)

			Expect(err).To(BeNil())
			Expect(sharedResourceNew.ClusterUser).NotTo(BeNil())
			Expect(sharedResourceNew.ManagedEnv).NotTo(BeNil())
			Expect(sharedResourceNew.GitopsEngineInstance).NotTo(BeNil())
			Expect(sharedResourceNew.ClusterAccess).NotTo(BeNil())

			Expect(sharedResourceNew.IsNewUser).To(BeFalse())
			Expect(sharedResourceNew.IsNewManagedEnv).To(BeFalse())
			Expect(sharedResourceNew.IsNewInstance).To(BeFalse())
			Expect(sharedResourceNew.IsNewClusterAccess).To(BeFalse())

			Expect(sharedResourceNew.ManagedEnv.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(sharedResourceOld.ClusterUser).To(Equal(sharedResourceNew.ClusterUser))
			Expect(sharedResourceNew.ManagedEnv.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(sharedResourceOld.ManagedEnv).To(Equal(sharedResourceNew.ManagedEnv))
			Expect(sharedResourceOld.GitopsEngineInstance).To(Equal(sharedResourceNew.GitopsEngineInstance))
			Expect(sharedResourceNew.ClusterAccess.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(sharedResourceOld.ClusterAccess).To(Equal(sharedResourceNew.ClusterAccess))

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{
				Clusteruser_id:          sharedResourceOld.ClusterUser.Clusteruser_id,
				Managedenvironment_id:   sharedResourceOld.ManagedEnv.Managedenvironment_id,
				Gitopsengineinstance_id: sharedResourceOld.GitopsEngineInstance.Gitopsengineinstance_id,
				clusterAccess:           sharedResourceOld.ClusterAccess,
				EngineCluster_id:        sharedResourceOld.GitopsEngineInstance.EngineCluster_id,
				Clustercredentials_id: []string{
					sharedResourceOld.GitopsEngineCluster.Clustercredentials_id,
					sharedResourceOld.ManagedEnv.Clustercredentials_id,
				},
			}
		})

		It("Should fetch a engine instance by ID.", func() {
			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// Negative test, engineInstance is not present, it should return error
			engineInstanceOld, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, "", k8sClient, *namespace, l)
			Expect(err).NotTo(BeNil())
			Expect(engineInstanceOld.EngineCluster_id).To(BeEmpty())

			// Create new engine instance which will be used by "GetGitopsEngineInstanceById" fucntion
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			defer dbq.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			gitopsEngineCluster := db.GitopsEngineCluster{
				Gitopsenginecluster_id: string(uuid.NewUUID()),
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: string(uuid.NewUUID()),
				Namespace_name:          namespace.Name,
				Namespace_uid:           string(namespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			// Fetch the same engineInstance by ID
			engineInstanceNew, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx,
				gitopsEngineInstance.Gitopsengineinstance_id, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(engineInstanceNew.EngineCluster_id).NotTo(BeNil())

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
				EngineCluster_id:        gitopsEngineInstance.EngineCluster_id,
				Clustercredentials_id: []string{
					clusterCredentials.Clustercredentials_cred_id,
				},
			}
		})

		It("Should fetch a GitOpsDeploymentRepositoryCredential.", func() {
			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// Create new engine instance which will be used by "GetGitopsEngineInstanceById" fucntion
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			defer dbq.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			gitopsEngineCluster := db.GitopsEngineCluster{
				Gitopsenginecluster_id: string(uuid.NewUUID()),
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: string(uuid.NewUUID()),
				Namespace_name:          "gitops-service-argocd",
				Namespace_uid:           string(namespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			// Fetch the same engineInstance by ID
			engineInstanceNew, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx,
				gitopsEngineInstance.Gitopsengineinstance_id, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(engineInstanceNew.EngineCluster_id).NotTo(BeNil())

			// At first assuming there are no existing users, hence creating new.
			usrOld,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(usrOld).NotTo(BeNil())
			Expect(isNewUser).To(BeTrue())

			// User is created in previous call, then same user should be returned instead of creating new.
			usrNew,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(usrNew).NotTo(BeNil())
			Expect(isNewUser).To(BeFalse())
			Expect(usrNew.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(usrOld).To(Equal(usrNew))

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
				EngineCluster_id:        gitopsEngineInstance.EngineCluster_id,
				// Clusteruser_id:          usrNew.Clusteruser_id,
				Clustercredentials_id: []string{
					clusterCredentials.Clustercredentials_cred_id,
				},
			}

			// Create new GitOpsDeploymentRepositoryCredential
			cr := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gitopsdeploymenrepositorycredential",
					Namespace: gitopsEngineInstance.Namespace_name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
					Repository: "https://fakegithub.com/test/test-repository",
					Secret:     "test-secret",
				}}

			err = k8sClient.Create(ctx, cr)
			Expect(err).To(BeNil())

			// Fetch the GitOpsDeploymentRepositoryCredential created
			cred := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cred)
			Expect(err).To(BeNil())

			var repositoryCredentialCRNamespace v1.Namespace
			repositoryCredentialCRNamespace.Name = gitopsEngineInstance.Namespace_name
			repositoryCredentialCRNamespace.UID = types.UID(gitopsEngineInstance.Namespace_uid)

			var k8sClientFactory SRLK8sClientFactory

			dbRepoCred, err := internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)

			// Negative test (there is no Secret)
			Expect(err).NotTo(BeNil())
			Expect(dbRepoCred).To(BeNil())

			// Create new Secret
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: gitopsEngineInstance.Namespace_name,
				},
				Data: map[string][]byte{
					"username": []byte("test-username"),
					"password": []byte("test-password"),
				},
			}
			err = k8sClient.Create(ctx, secret)
			Expect(err).To(BeNil())

			// Create again the CR
			// Expected: Since there's no DB entry for the CR, it will create an operation
			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).NotTo(BeNil())

			By("verify whether appProject is created or not")
			appProjectRepositoryDB := &db.AppProjectRepository{
				Clusteruser_id: usrNew.Clusteruser_id,
				RepoURL:        NormalizeGitURL(dbRepoCred.PrivateURL),
			}

			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, appProjectRepositoryDB)
			Expect(err).To(BeNil())
			Expect(appProjectRepositoryDB).NotTo(BeNil())

			var operationDB db.Operation
			var operations []db.Operation

			// Verify there is one Operation CR created and find its matching DB Entry
			operationList := &managedgitopsv1alpha1.OperationList{}
			err = k8sClient.List(ctx, operationList)
			Expect(err).To(BeNil())

			primaryKey := dbRepoCred.RepositoryCredentialsID
			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, primaryKey, db.OperationResourceType_RepositoryCredentials, &operations, usrNew.Clusteruser_id)
			Expect(err).To(BeNil())

			// Verify the Operation CR and DB Entry are the same
			Expect(len(operations)).To(Equal(1))
			Expect(len(operationList.Items)).To(Equal(1))
			operationDB = operations[0]
			operationCR := operationList.Items[0]
			Expect(operationDB.Operation_id).To(Equal(operationCR.Spec.OperationID))

			fmt.Println("Get Operation State", "operation", operationDB)
			Expect(operationDB.State).Should(Equal(db.OperationState_Waiting))

			// Fetch the RepositoryCredential DB row using information provided by the Operation
			// this is how the cluster-agent will find the RepositoryCredential DB row
			// and verify that this db row is the same with the output of the internalProcessMessage_ReconcileRepositoryCredential()
			fmt.Println("Get the RepositoryCredential DB row using the operationDB.Resource_id", "operation Resource ID", operationDB.Resource_id)
			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, operationDB.Resource_id)
			Expect(err).To(BeNil())
			Expect(fetch.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetch.Created_on = dbRepoCred.Created_on
			Expect(fetch).Should(Equal(*dbRepoCred))

			// Delete the Operation using the CleanRepoCredOperation function
			fmt.Println("TEST: Delete the Operation using the CleanRepoCredOperation function")
			err = CleanRepoCredOperation(ctx, *dbRepoCred, usrNew, cr.Namespace, dbq, k8sClient, operationDB.Operation_id, l)
			Expect(err).To(BeNil()) // No error expected because the Operation is in Waiting state (so it's not deleted, and we don't consider this as an error)

			// Set the Operation DB state to Completed (so it will be deleted the next time)
			operationDB.State = db.OperationState_Completed
			err = dbq.UpdateOperation(ctx, &operationDB)
			Expect(err).To(BeNil())

			// Call the CleanRepoCredOperation function again
			fmt.Println("TEST: Call the CleanRepoCredOperation function again")
			err = CleanRepoCredOperation(ctx, *dbRepoCred, usrNew, cr.Namespace, dbq, k8sClient, operationDB.Operation_id, l)
			Expect(err).To(BeNil())

			// Verify the Operation CR and DB Entry are deleted
			operationList = &managedgitopsv1alpha1.OperationList{}
			err = k8sClient.List(ctx, operationList)
			Expect(err).To(BeNil())

			operations = []db.Operation{}
			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, primaryKey, db.OperationResourceType_RepositoryCredentials, &operations, usrNew.Clusteruser_id)
			Expect(err).To(BeNil())

			// There should be no Operation DB entry
			Expect(len(operations)).To(Equal(0))

			// There should be no Operation CR
			Expect(len(operationList.Items)).To(Equal(0))

			// Re-running should not error
			fmt.Println("Re-running the internalProcessMessage_ReconcileRepositoryCredential()")
			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).NotTo(BeNil())

			By("verify whether appProject is created or not")
			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, appProjectRepositoryDB)
			Expect(err).To(BeNil())
			Expect(appProjectRepositoryDB).NotTo(BeNil())

			// Check if there are any new operations, if they are deleted (previously) there should be none
			primaryKey = dbRepoCred.RepositoryCredentialsID
			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, primaryKey, db.OperationResourceType_RepositoryCredentials, &operations, usrNew.Clusteruser_id)
			Expect(err).To(BeNil())
			Expect(len(operations)).To(Equal(0))
			Expect(len(operationList.Items)).To(Equal(0))

			// Modify the repository credential database, pointing to a wrong secret
			// Expected: The diff should be detected and roll-back to what the GitOpsDeploymentRepositoryCredential CR has (source of truth)
			// also it should fire-up an operation to fix this
			fmt.Println("Modify the repository credential database, pointing to a wrong secret")
			dbRepoCred.SecretObj = "test-secret-2"
			err = dbq.UpdateRepositoryCredentials(ctx, dbRepoCred)
			Expect(err).To(BeNil())

			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).ToNot(BeNil())

			By("verify whether appProject is present or not when repoCred is updated")
			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, appProjectRepositoryDB)
			Expect(err).To(BeNil())
			Expect(appProjectRepositoryDB).NotTo(BeNil())

			// Check if there are any operations
			operationList = &managedgitopsv1alpha1.OperationList{}
			err = k8sClient.List(ctx, operationList)
			Expect(err).To(BeNil())
			Expect(len(operationList.Items)).Should(Equal(1))
			Expect(len(operationList.Items)).To(Equal(1))

			// Fetch the operation db
			operationDB.Operation_id = operationList.Items[0].Spec.OperationID
			err = dbq.GetOperationById(ctx, &operationDB)
			Expect(err).To(BeNil())
			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, primaryKey, db.OperationResourceType_RepositoryCredentials, &operations, usrNew.Clusteruser_id)
			Expect(err).To(BeNil())
			operationDB = operations[0]
			operationCR = operationList.Items[0]
			Expect(operationDB.Operation_id).To(Equal(operationCR.Spec.OperationID))
			Expect(operationDB.State).Should(Equal(db.OperationState_Waiting))

			// Delete the operation db and operation cr
			_, err = dbq.DeleteOperationById(ctx, operationDB.Operation_id)
			Expect(err).To(BeNil())
			err = k8sClient.Delete(ctx, &operationList.Items[0])
			Expect(err).To(BeNil())

			// Set the status to Completed and reconcile
			// Expected: The status should be updated to Completed
			// Also, the operation should be deleted
			operationDB.State = db.OperationState_Completed
			err = dbq.UpdateOperation(ctx, &operationDB)
			// err should not be nil
			Expect(err).ToNot(BeNil()) // err unexpected number of rows affected:
			// Expect(err).To(BeNil())

			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).ToNot(BeNil())

			// Verify the Operation CR and DB Entry are deleted
			operationList = &managedgitopsv1alpha1.OperationList{}
			err = k8sClient.List(ctx, operationList)
			Expect(err).To(BeNil())

			operations = []db.Operation{}
			err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, primaryKey, db.OperationResourceType_RepositoryCredentials, &operations, usrNew.Clusteruser_id)
			Expect(err).To(BeNil())

			// There should be no Operation DB entry
			Expect(len(operations)).To(Equal(0))

			// There should be no Operation CR
			Expect(len(operationList.Items)).To(Equal(0))

			// Delete the Secret
			err = k8sClient.Delete(ctx, secret)
			Expect(err).To(BeNil())

			// Delete the GitOpsDeploymentRepositoryCredential CR and reconcile again
			// Expected: Since there is no GitOpsDeploymentRepositoryCredential CR, it will delete the DB entry
			err = k8sClient.Delete(ctx, cr)
			Expect(err).To(BeNil())
			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).To(BeNil())

			// Negative test: Get the RepositoryCredential from the DB
			// Expected: It should not exist
			_, err = dbq.GetRepositoryCredentialsByID(ctx, cr.Name)
			Expect(err).ToNot(BeNil())

			// A new Operation should be created
			// Check if there are any operations left (should be 1)
			operationList = &managedgitopsv1alpha1.OperationList{}
			err = k8sClient.List(ctx, operationList)
			Expect(err).To(BeNil())
			Expect(len(operationList.Items)).Should(Equal(1))
			// Fetch the operation db
			operationDB.Operation_id = operationList.Items[0].Spec.OperationID
			err = dbq.GetOperationById(ctx, &operationDB)
			Expect(err).To(BeNil())
			Expect(operationDB.State).Should(Equal(db.OperationState_Waiting))

			// Negative test: Try again to reconcile the RepositoryCredential
			// Expected: It should not error (both db row and CR should be deleted). Nothing we can do.
			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).To(BeNil())

		})

		It("Should add display_name to existing clusterUser if display_name is empty", func() {
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			defer dbq.CloseDatabase()

			By("Create cluster user")
			clusterUserDb := &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      string(namespace.UID),
			}
			err = dbq.CreateClusterUser(ctx, clusterUserDb)
			Expect(err).To(BeNil())

			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			user,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(user).NotTo(BeNil())
			Expect(isNewUser).To(BeFalse())
			Expect(user.Display_name).ToNot(BeEmpty())
			Expect(user.Display_name).To(Equal(namespace.Name))

			resourcesToBeDeleted = testResources{Clusteruser_id: clusterUserDb.Clusteruser_id}

		})

		It("Should verify AppProjectRepository is updated to point to the repoCred row in database.", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			By("Create GitopsDeployment")
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitopsdeployment",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL: "http://github.com/jgwest/my-repo",
					},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			err = k8sClient.Create(context.Background(), gitopsDepl)
			Expect(err).To(BeNil())

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			defer dbq.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			gitopsEngineCluster := db.GitopsEngineCluster{
				Gitopsenginecluster_id: string(uuid.NewUUID()),
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: string(uuid.NewUUID()),
				Namespace_name:          "gitops-service-argocd",
				Namespace_uid:           string(namespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			var repositoryCredentialCRNamespace v1.Namespace
			repositoryCredentialCRNamespace.Name = gitopsEngineInstance.Namespace_name
			repositoryCredentialCRNamespace.UID = types.UID(gitopsEngineInstance.Namespace_uid)

			By("Create DB entry for ClusterUser")
			clusterUserDb := &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      string(repositoryCredentialCRNamespace.UID),
			}
			err = dbq.CreateClusterUser(ctx, clusterUserDb)
			Expect(err).To(BeNil())

			By("creating a AppProjectRepository that is based on the contents of the GitOpsDeployment")
			appProjectRepoDB := &db.AppProjectRepository{
				AppProjectRepositoryID:  "test-appProject-ID",
				Clusteruser_id:          clusterUserDb.Clusteruser_id,
				RepositorycredentialsID: "",
				RepoURL:                 NormalizeGitURL(gitopsDepl.Spec.Source.RepoURL),
			}

			err = dbq.CreateAppProjectRepository(ctx, appProjectRepoDB)
			Expect(err).To(BeNil())

			By("Create new GitOpsDeploymentRepositoryCredential")
			cr := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gitopsdeploymentrepositorycredential",
					Namespace: gitopsEngineInstance.Namespace_name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
					Repository: "http://github.com/jgwest/my-repo",
					Secret:     "test-secret",
				}}

			err = k8sClient.Create(ctx, cr)
			Expect(err).To(BeNil())

			var k8sClientFactory SRLK8sClientFactory

			By("Create new Secret")
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: gitopsEngineInstance.Namespace_name,
				},
				Data: map[string][]byte{
					"username": []byte("test-username"),
					"password": []byte("test-password"),
				},
			}
			err = k8sClient.Create(ctx, secret)
			Expect(err).To(BeNil())

			dbRepoCred, err := internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).NotTo(BeNil())

			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, appProjectRepoDB)
			Expect(err).To(BeNil())
			Expect(appProjectRepoDB).NotTo(BeNil())

			By("Verify whether AppProjectRepoCred is updated to point to the repoCred row in the database.")
			Expect(appProjectRepoDB.RepositorycredentialsID).ToNot(BeNil())
			Expect(appProjectRepoDB.RepositorycredentialsID).To(Equal(dbRepoCred.RepositoryCredentialsID))

			By("Deleting resources created by test.")
			resourcesToBeDeleted = testResources{
				Clustercredentials_id: []string{
					clusterCredentials.Clustercredentials_cred_id,
				},
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				AppProjectRepository:    appProjectRepoDB,
			}

		})

		It("Should verify AppProjectRepository is updated/created when RepoCred CR is updated", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			By("Create new engine instance which will be used by `GetGitopsEngineInstanceById` fucntion")
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			defer dbq.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			gitopsEngineCluster := db.GitopsEngineCluster{
				Gitopsenginecluster_id: string(uuid.NewUUID()),
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: string(uuid.NewUUID()),
				Namespace_name:          "gitops-service-argocd",
				Namespace_uid:           string(namespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Fetch the same engineInstance by ID")
			engineInstanceNew, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx,
				gitopsEngineInstance.Gitopsengineinstance_id, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(engineInstanceNew.EngineCluster_id).NotTo(BeNil())

			By("At first assuming there are no existing users, hence creating new.")
			usrOld,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(usrOld).NotTo(BeNil())
			Expect(isNewUser).To(BeTrue())

			By("User is created in previous call, then same user should be returned instead of creating new.")
			usrNew,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace, l)

			Expect(err).To(BeNil())
			Expect(usrNew).NotTo(BeNil())
			Expect(isNewUser).To(BeFalse())
			Expect(usrNew.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(usrOld).To(Equal(usrNew))

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
				EngineCluster_id:        gitopsEngineInstance.EngineCluster_id,
				Clustercredentials_id: []string{
					clusterCredentials.Clustercredentials_cred_id,
				},
			}

			By("Create new GitOpsDeploymentRepositoryCredential")
			cr := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gitopsdeploymenrepositorycredential",
					Namespace: gitopsEngineInstance.Namespace_name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
					Repository: "https://fakegithub.com/test/test-repository",
					Secret:     "test-secret",
				}}

			err = k8sClient.Create(ctx, cr)
			Expect(err).To(BeNil())

			By("Fetch the GitOpsDeploymentRepositoryCredential created")
			cred := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cred)
			Expect(err).To(BeNil())

			var repositoryCredentialCRNamespace v1.Namespace
			repositoryCredentialCRNamespace.Name = gitopsEngineInstance.Namespace_name
			repositoryCredentialCRNamespace.UID = types.UID(gitopsEngineInstance.Namespace_uid)

			var k8sClientFactory SRLK8sClientFactory

			By("Create new Secret")
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: gitopsEngineInstance.Namespace_name,
				},
				Data: map[string][]byte{
					"username": []byte("test-username"),
					"password": []byte("test-password"),
				},
			}
			err = k8sClient.Create(ctx, secret)
			Expect(err).To(BeNil())

			dbRepoCred, err := internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).NotTo(BeNil())

			By("verify whether appProject is created or not")
			appProjectRepositoryDB := &db.AppProjectRepository{
				Clusteruser_id: usrNew.Clusteruser_id,
				RepoURL:        NormalizeGitURL(dbRepoCred.PrivateURL),
			}

			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, appProjectRepositoryDB)
			Expect(err).To(BeNil())
			Expect(appProjectRepositoryDB).NotTo(BeNil())

			By("Fetch the GitOpsDeploymentRepositoryCredential CR")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr)
			Expect(err).To(BeNil())

			By("Update GitopsRepositoryCredential CR  to verify whether it updates RepoCred URL")
			cr.Spec.Repository = "http://github.com/jgwest/my-repo"

			err = k8sClient.Update(ctx, cr)
			Expect(err).To(BeNil())

			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, appProjectRepositoryDB)
			Expect(err).To(BeNil())
			Expect(appProjectRepositoryDB).NotTo(BeNil())

			dbRepoCred, err = internalProcessMessage_ReconcileRepositoryCredential(ctx, cr.Name, repositoryCredentialCRNamespace, k8sClient, k8sClientFactory, dbq, false, l)
			Expect(err).To(BeNil())
			Expect(dbRepoCred).NotTo(BeNil())
			Expect(dbRepoCred.PrivateURL).To(Equal("http://github.com/jgwest/my-repo"))

			By("Verify whether AppProjectRepositoryDB is created with the new Repo URL as Repo URL has been updated in repositoryCredential row")
			getappProjectRepositoryDB := &db.AppProjectRepository{
				Clusteruser_id: usrNew.Clusteruser_id,
				RepoURL:        NormalizeGitURL(dbRepoCred.PrivateURL),
			}

			err = dbq.GetAppProjectRepositoryByClusterUserAndRepoURL(ctx, getappProjectRepositoryDB)
			Expect(err).To(BeNil())
			Expect(getappProjectRepositoryDB).NotTo(BeNil())
			Expect(getappProjectRepositoryDB.RepoURL).To(Equal("http://github.com/jgwest/my-repo"))

			resourcesToBeDeleted = testResources{
				AppProjectRepository: getappProjectRepositoryDB,
			}
		})

	})
})
