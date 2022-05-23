package shared_resource_loop

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
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
}

var _ = Describe("SharedResourceEventLoop Test", func() {

	// This will be used by AfterEach to clean resources
	var resourcesToBeDeleted testResources

	var ctx context.Context
	var k8sClient *sharedutil.ProxyClient
	var namespace *v1.Namespace

	Context("Shared Resource Event Loop test", func() {

		// Create a fake k8s client before each test
		BeforeEach(func() {
			ctx = context.Background()
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespaceTemp, err := eventlooptypes.GenericTestSetup()

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

			})
		})

		It("Should create or fetch a user by Namespace id.", func() {
			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// At first assuming there are no existing users, hence creating new.
			usrOld,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace)

			Expect(err).To(BeNil())
			Expect(usrOld).NotTo(BeNil())
			Expect(isNewUser).To(BeTrue())

			// User is created in previous call, then same user should be returned instead of creating new.
			usrNew,
				isNewUser,
				err := sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, k8sClient, *namespace)

			Expect(err).To(BeNil())
			Expect(usrNew).NotTo(BeNil())
			Expect(isNewUser).To(BeFalse())
			Expect(usrOld).To(Equal(usrNew))

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{Clusteruser_id: usrNew.Clusteruser_id}
		})

		It("Should create or fetch resources.", func() {
			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// At first assuming there are no existing resources, hence creating new.
			clusterUserOld,
				isNewUser,
				managedEnvOld,
				isNewManagedEnv,
				gitopsEngineInstanceOld,
				isNewInstance,
				clusterAccessOld,
				isNewClusterAccess,
				gitopsEngineClusterOld,
				err := sharedResourceEventLoop.GetOrCreateSharedResources(ctx, k8sClient, *namespace)

			Expect(err).To(BeNil())
			Expect(clusterUserOld).NotTo(BeNil())
			Expect(managedEnvOld).NotTo(BeNil())
			Expect(gitopsEngineInstanceOld).NotTo(BeNil())
			Expect(clusterAccessOld).NotTo(BeNil())

			Expect(isNewUser).To(BeTrue())
			Expect(isNewManagedEnv).To(BeTrue())
			Expect(isNewInstance).To(BeTrue())
			Expect(isNewClusterAccess).To(BeTrue())

			// Resources are created in previous call, then same resources should be returned instead of creating new.
			clusterUserNew,
				isNewUser,
				managedEnvNew,
				isNewManagedEnv,
				gitopsEngineInstanceNew,
				isNewInstance,
				clusterAccessNew,
				isNewClusterAccess,
				_, err := sharedResourceEventLoop.GetOrCreateSharedResources(ctx, k8sClient, *namespace)

			Expect(err).To(BeNil())
			Expect(clusterUserNew).NotTo(BeNil())
			Expect(managedEnvNew).NotTo(BeNil())
			Expect(gitopsEngineInstanceNew).NotTo(BeNil())
			Expect(clusterAccessNew).NotTo(BeNil())

			Expect(isNewUser).To(BeFalse())
			Expect(isNewManagedEnv).To(BeFalse())
			Expect(isNewInstance).To(BeFalse())
			Expect(isNewClusterAccess).To(BeFalse())

			Expect(clusterUserOld).To(Equal(clusterUserNew))
			Expect(managedEnvOld).To(Equal(managedEnvNew))
			Expect(gitopsEngineInstanceOld).To(Equal(gitopsEngineInstanceNew))
			Expect(clusterAccessOld).To(Equal(clusterAccessNew))

			// To be used by AfterEach to clean up the resources created by test
			resourcesToBeDeleted = testResources{
				Clusteruser_id:          clusterUserOld.Clusteruser_id,
				Managedenvironment_id:   managedEnvOld.Managedenvironment_id,
				Gitopsengineinstance_id: gitopsEngineInstanceOld.Gitopsengineinstance_id,
				clusterAccess:           clusterAccessOld,
				EngineCluster_id:        gitopsEngineInstanceOld.EngineCluster_id,
				Clustercredentials_id: []string{
					gitopsEngineClusterOld.Clustercredentials_id,
					managedEnvOld.Clustercredentials_id,
				},
			}
		})

		It("Should fetch a engine instance by ID.", func() {
			sharedResourceEventLoop := &SharedResourceEventLoop{inputChannel: make(chan sharedResourceLoopMessage)}

			go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

			// Negative test, engineInstance is not present, it should return error
			engineInstanceOld, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, "", k8sClient, *namespace)
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
				Namespace_name:          namespace.Namespace,
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
			engineInstanceNew, err := sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id, k8sClient, *namespace)

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
	})
})
