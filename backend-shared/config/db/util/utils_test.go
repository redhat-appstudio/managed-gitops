package util

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Test utility functions.", func() {
	Context("It should execute and test GetOrCreateManagedEnvironmentByNamespaceUID function.", func() {

		It("Should create new managedEnvironment and other resources, if called second time then it should return existing resources.", func() {

			workSpaceUid := uuid.NewUUID()
			ctx := context.Background()

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())
			defer dbQueries.CloseDatabase()

			workspace := v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					UID:       workSpaceUid,
					Namespace: "test-namespace",
				},
				Spec: v1.NamespaceSpec{},
			}

			// ----------------------------------------------------------------------------
			By("It should create new ManagedEnvironment for given namespace.")
			// ----------------------------------------------------------------------------
			log := log.FromContext(context.Background())

			managedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that other database entries are also created.")
			// ----------------------------------------------------------------------------

			// Check KubernetesToDBResourceMapping resource
			kubernetesToDBResourceMappingget := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  string(workSpaceUid),
				DBRelationType:         "ManagedEnvironment",
			}

			err = dbQueries.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
			Expect(err).To(BeNil())

			// Check ClusterCredentials resource
			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: managedEnvironment.Clustercredentials_id,
			}
			err = dbQueries.GetClusterCredentialsById(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("If called 2nd time, it should return existing ManagedEnvironment instead of creating new and flag should be False.")
			// ----------------------------------------------------------------------------
			managedEnvironmentSecond, isNewSecond, errSecond := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(errSecond).To(BeNil())
			Expect(isNewSecond).To(BeFalse())
			Expect(managedEnvironmentSecond).To(Equal(managedEnvironment))

			// ----------------------------------------------------------------------------
			By("Delete resources created by test.")
			// ----------------------------------------------------------------------------

			// Delete managedEnv
			rowsAffected, err := dbQueries.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))

			// Delete clusterCredentials
			rowsAffected, err = dbQueries.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)

			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))

			rowsAffected, err = dbQueries.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingget)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))
		})

		It("Should fail as NameSpace details are invalid.", func() {

			ctx := context.Background()

			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			workspace := v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					UID:       "",
					Namespace: "test-namespace",
				},
				Spec: v1.NamespaceSpec{},
			}

			log := log.FromContext(context.Background())

			managedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(err).NotTo(BeNil())
			Expect(isNew).To(BeFalse())
			Expect(managedEnvironment).To(BeNil())
		})
	})

	Context("It should execute and test GetOrCreateDeploymentToApplicationMapping function.", func() {

		var err error
		var log logr.Logger
		var ctx context.Context
		var application db.Application
		var dbQueries db.AllDatabaseQueries
		var clusterCredentials db.ClusterCredentials
		var managedEnvironment db.ManagedEnvironment
		var gitopsEngineCluster db.GitopsEngineCluster
		var gitopsEngineInstance db.GitopsEngineInstance
		var deploymentToApplicationMapping *db.DeploymentToApplicationMapping

		BeforeEach(func() {
			ctx = context.Background()
			log = logger.FromContext(context.Background())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("First create resources required by deploymentToApplicationMapping.")
			// ----------------------------------------------------------------------------

			// Create ClusterCredentials
			clusterCredentials = db.ClusterCredentials{
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			// ---------------------------------------------------------------------------
			// Create ManagedEnvironment

			managedEnvironment = db.ManagedEnvironment{
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my-managed-environment",
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).To(BeNil())

			// ---------------------------------------------------------------------------
			// Create GitopsEngineCluster

			gitopsEngineCluster = db.GitopsEngineCluster{
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbQueries.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			// ---------------------------------------------------------------------------
			// Create GitopsEngineInstance

			gitopsEngineInstance = db.GitopsEngineInstance{
				Namespace_name:   "my-namespace",
				Namespace_uid:    "test-1",
				EngineCluster_id: gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			// ---------------------------------------------------------------------------
			// Create Application

			application = db.Application{
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, &application)
			Expect(err).To(BeNil())

			// ---------------------------------------------------------------------------
			// Create deploymentToApplicationMapping request data

			deploymentToApplicationMapping = &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: string(uuid.NewUUID()),
				Application_id:                        application.Application_id,
				DeploymentName:                        "my-depl-to-app-mapping",
				DeploymentNamespace:                   "my-namespace",
				NamespaceUID:                          "test-1",
			}
		})

		AfterEach(func() {
			// ----------------------------------------------------------------------------
			By("Delete resources and clean db entries created by test.")
			// ----------------------------------------------------------------------------

			_, err = dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)
			Expect(err).To(BeNil())

			_, err = dbQueries.DeleteApplicationById(ctx, application.Application_id)
			Expect(err).To(BeNil())

			_, err = dbQueries.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
			Expect(err).To(BeNil())

			_, err = dbQueries.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
			Expect(err).To(BeNil())

			_, err = dbQueries.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
			Expect(err).To(BeNil())

			_, err = dbQueries.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
			Expect(err).To(BeNil())

			// Close connection
			dbQueries.CloseDatabase()
		})

		It("Should create new DeploymentToApplicationMapping and if called second time, it should return existing resource instead of creating new.", func() {

			// ----------------------------------------------------------------------------
			By("Create new DeploymentToApplicationMapping resource.")
			// ----------------------------------------------------------------------------
			isNewFirst, errFirst := GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping, dbQueries, log)
			Expect(errFirst).To(BeNil())
			Expect(isNewFirst).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Check if DeploymentToApplicationMapping resource entry is created in DB.")
			// ----------------------------------------------------------------------------

			err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			})
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Try to create same DeploymentToApplicationMapping, it should return existing resource.")
			// ----------------------------------------------------------------------------

			isNewSecond, errSecond := GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping, dbQueries, log)
			Expect(errSecond).To(BeNil())
			Expect(isNewSecond).To(BeFalse())
		})

		It("Should delete exiting DeploymentToApplicationMapping if a resource is passed having same name/namespace, so there is be only one resource per name and namespace.", func() {

			// ----------------------------------------------------------------------------
			By("Create first DeploymentToApplicationMapping resource.")
			// ----------------------------------------------------------------------------

			isNewFirst, errFirst := GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping, dbQueries, log)
			Expect(errFirst).To(BeNil())
			Expect(isNewFirst).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Create second DeploymentToApplicationMapping resource, having same name and namespace as first, but different ID.")
			// ----------------------------------------------------------------------------

			deploymentToApplicationMappingSecond := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: string(uuid.NewUUID()),
				Application_id:                        application.Application_id,
				DeploymentName:                        "my-depl-to-app-mapping",
				DeploymentNamespace:                   "my-namespace",
				NamespaceUID:                          "test-1",
			}

			isNewSecond, errSecond := GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingSecond, dbQueries, log)
			Expect(errSecond).To(BeNil())
			Expect(isNewSecond).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Check that first DeploymentToApplicationMapping is deleted in DB.")
			// ----------------------------------------------------------------------------

			err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			})
			Expect(err).NotTo(BeNil())

			// ----------------------------------------------------------------------------
			By("Check that second DeploymentToApplicationMapping resource is created in DB.")
			// ----------------------------------------------------------------------------

			err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMappingSecond.Deploymenttoapplicationmapping_uid_id,
			})
			Expect(err).To(BeNil())

			// send second resource object to get deleted in AfterEach
			deploymentToApplicationMapping = deploymentToApplicationMappingSecond
		})
	})

})

func TestGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(t *testing.T) {

	_, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.Nil(t, err) {
		return
	}

	// clusterCredentials := db.ClusterCredentials{
	// 	Clustercredentials_cred_id:  "test-creds",
	// 	Host:                        "",
	// 	Kube_config:                 "",
	// 	Kube_config_context:         "",
	// 	Serviceaccount_bearer_token: "",
	// 	Serviceaccount_ns:           "",
	// }

	// gitopsEngineNamespace := v1.Namespace{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name: "fake-namespace",
	// 		UID:  "fake-uid",
	// 	},
	// }

	// kubesystemNamespaceUID := "fake-uid"

	// engineInstance, engineCluster, err := GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(context.Background(), gitopsEngineNamespace,
	// 	kubesystemNamespaceUID, dbq, logr.FromContext(context.Background()))

	// t.Logf("%v", engineInstance)
	// t.Logf("%v", engineCluster)

	// assert.Nil(t, err)
}
