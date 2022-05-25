package util

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

// Used to list down resources for deletion which are created while running tests.
type testResources struct {
	Application_id                        string
	Managedenvironment_id                 string
	Gitopsenginecluster_id                string
	Gitopsengineinstance_id               string
	Clustercredentials_cred_id            string
	Deploymenttoapplicationmapping_uid_id string
	kubernetesToDBResourceMapping         db.KubernetesToDBResourceMapping
}

// Delete resources from table
func deleteTestResources(ctx context.Context, dbQueries db.AllDatabaseQueries, resourcesToBeDeleted testResources) {
	var rowsAffected int
	var err error

	// Delete kubernetesToDBResourceMapping
	if resourcesToBeDeleted.kubernetesToDBResourceMapping.KubernetesResourceUID != "" {
		rowsAffected, err = dbQueries.DeleteKubernetesResourceToDBResourceMapping(ctx, &resourcesToBeDeleted.kubernetesToDBResourceMapping)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete DeploymentToApplicationMapping
	if resourcesToBeDeleted.Deploymenttoapplicationmapping_uid_id != "" {
		rowsAffected, err := dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, resourcesToBeDeleted.Deploymenttoapplicationmapping_uid_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete Application
	if resourcesToBeDeleted.Application_id != "" {
		rowsAffected, err = dbQueries.DeleteApplicationById(ctx, resourcesToBeDeleted.Application_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete GitopsEngineInstance
	if resourcesToBeDeleted.Gitopsengineinstance_id != "" {
		rowsAffected, err = dbQueries.DeleteGitopsEngineInstanceById(ctx, resourcesToBeDeleted.Gitopsengineinstance_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete GitopsEngineCluster
	if resourcesToBeDeleted.Gitopsenginecluster_id != "" {
		rowsAffected, err = dbQueries.DeleteGitopsEngineClusterById(ctx, resourcesToBeDeleted.Gitopsenginecluster_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete ManagedEnvironment
	if resourcesToBeDeleted.Managedenvironment_id != "" {
		rowsAffected, err = dbQueries.DeleteManagedEnvironmentById(ctx, resourcesToBeDeleted.Managedenvironment_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete ClusterCredentials
	if resourcesToBeDeleted.Clustercredentials_cred_id != "" {
		rowsAffected, err = dbQueries.DeleteClusterCredentialsById(ctx, resourcesToBeDeleted.Clustercredentials_cred_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}
}

// Check required KubernetesToDBResourceMapping is present in Db.
// Since we don't know "kubernetes_resource_uid" to fetch object using GetDBResourceMappingForKubernetesResource function
// we will have to fetch all entries from table and check one by one if expected entry is present in table.
func findKubernetesToDBResourceMappingInTable(ctx context.Context, dbQueries db.AllDatabaseQueries, dBRelationKey string, dBRelationType string) (db.KubernetesToDBResourceMapping, bool, error) {

	foundMapping := false
	var mappingResult db.KubernetesToDBResourceMapping
	var kubernetesToDBResourceMappings []db.KubernetesToDBResourceMapping

	err := dbQueries.UnsafeListAllKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappings)
	if err != nil {
		return mappingResult, foundMapping, err
	}

	for i := range kubernetesToDBResourceMappings {
		item := kubernetesToDBResourceMappings[i]
		if item.DBRelationType == dBRelationType &&
			item.DBRelationKey == dBRelationKey {

			// The entry we are looking for is present in table
			foundMapping = true
			mappingResult = item
			break
		}
	}
	return mappingResult, foundMapping, nil
}

// initialize object required prior to tests
func initialSetUp() (context.Context, db.AllDatabaseQueries, logr.Logger, types.UID, error) {
	dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
	if err != nil {
		return nil, nil, nil, "", err
	}

	ctx := context.Background()
	log := logger.FromContext(ctx)

	return ctx, dbQueries, log, uuid.NewUUID(), err
}

var _ = Describe("Test utility functions.", func() {

	Context("Testing for GetOrCreateManagedEnvironmentByNamespaceUID function.", func() {

		It("Should create new managedEnvironment and other resources, if called second time then it should return existing resources.", func() {
			ctx, dbQueries, log, workSpaceUid, err := initialSetUp()
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

			managedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that other database entries are also created.")
			// ----------------------------------------------------------------------------

			// Check KubernetesToDBResourceMapping resource
			kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: db.K8sToDBMapping_Namespace,
				KubernetesResourceUID:  string(workSpaceUid),
				DBRelationType:         db.K8sToDBMapping_ManagedEnvironment,
			}

			err = dbQueries.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())

			// Check ClusterCredentials resource
			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: managedEnvironment.Clustercredentials_id,
			}
			err = dbQueries.GetClusterCredentialsById(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("If called 2nd time, it should return existing ManagedEnvironment, instead of creating new and flag should be False.")
			// ----------------------------------------------------------------------------

			retriveManagedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeFalse())
			Expect(retriveManagedEnvironment).To(Equal(managedEnvironment))

			// ----------------------------------------------------------------------------
			By("Delete resources created by test.")
			// ----------------------------------------------------------------------------

			resourcesToBeDeleted := testResources{
				Managedenvironment_id:         managedEnvironment.Managedenvironment_id,
				Clustercredentials_cred_id:    clusterCredentials.Clustercredentials_cred_id,
				kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)
		})

		It("Should fail as NameSpace is invalid.", func() {
			ctx, dbQueries, log, _, err := initialSetUp()
			Expect(err).To(BeNil())

			workspace := v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					UID:       "",
					Namespace: "test-namespace",
				},
				Spec: v1.NamespaceSpec{},
			}

			managedEnvironment, isNew, err := GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspace, dbQueries, log)

			Expect(err).NotTo(BeNil())
			Expect(isNew).To(BeFalse())
			Expect(managedEnvironment).To(BeNil())
		})
	})

	Context("Testing for GetOrCreateDeploymentToApplicationMapping function.", func() {

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
			ctx, dbQueries, log, _, err = initialSetUp()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("First create resources required by DeploymentToApplicationMapping resource.")
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
			resourcesToBeDeleted := testResources{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
				Application_id:                        application.Application_id,
				Gitopsengineinstance_id:               gitopsEngineInstance.Gitopsengineinstance_id,
				Gitopsenginecluster_id:                gitopsEngineCluster.Gitopsenginecluster_id,
				Managedenvironment_id:                 managedEnvironment.Managedenvironment_id,
				Clustercredentials_cred_id:            clusterCredentials.Clustercredentials_cred_id,
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			// Close connection
			defer dbQueries.CloseDatabase()
		})

		It("Should create new DeploymentToApplicationMapping if called first time and for second time, it should return existing resource instead of creating new.", func() {

			// ----------------------------------------------------------------------------
			By("Create new DeploymentToApplicationMapping resource.")
			// ----------------------------------------------------------------------------
			isNew, err := GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify DeploymentToApplicationMapping resource entry is created in DB.")
			// ----------------------------------------------------------------------------

			err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			})
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Try to create same DeploymentToApplicationMapping, it should return existing resource instead of creating new.")
			// ----------------------------------------------------------------------------

			isNew, err = GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeFalse())
		})

		It("Should delete existing DeploymentToApplicationMapping, if a resource is passed having same name/namespace, so there is be only one resource per name and namespace.", func() {

			// ----------------------------------------------------------------------------
			By("Create first DeploymentToApplicationMapping resource.")
			// ----------------------------------------------------------------------------

			isNew, err := GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

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

			isNew, err = GetOrCreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMappingSecond, dbQueries, log)
			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Check first DeploymentToApplicationMapping is deleted in DB.")
			// ----------------------------------------------------------------------------

			err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			})
			Expect(err).NotTo(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Check second DeploymentToApplicationMapping resource is created in DB.")
			// ----------------------------------------------------------------------------

			err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMappingSecond.Deploymenttoapplicationmapping_uid_id,
			})
			Expect(err).To(BeNil())

			// send second resource object to get deleted in AfterEach
			deploymentToApplicationMapping = deploymentToApplicationMappingSecond
		})
	})

	Context("Testing for GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID function.", func() {

		var err error
		var isNew bool
		var log logr.Logger
		var ctx context.Context
		var workSpaceUid types.UID
		var dbQueries db.AllDatabaseQueries
		var gitopsEngineCluster *db.GitopsEngineCluster
		var kubernetesToDBResourceMapping db.KubernetesToDBResourceMapping

		AfterEach(func() {
			var resourcesToBeDeleted testResources
			// Objects could be nil, in that case it would throw error. To avoid that check object first
			if gitopsEngineCluster != nil {
				resourcesToBeDeleted.Gitopsenginecluster_id = gitopsEngineCluster.Gitopsenginecluster_id
			}

			if gitopsEngineCluster != nil {
				resourcesToBeDeleted.Clustercredentials_cred_id = gitopsEngineCluster.Clustercredentials_id
			}

			if kubernetesToDBResourceMapping.KubernetesResourceUID != "" {
				resourcesToBeDeleted.kubernetesToDBResourceMapping = kubernetesToDBResourceMapping
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			defer dbQueries.CloseDatabase()
		})

		It("Should create new GitopsEngineCluster and KubernetesToDBResourceMapping.", func() {
			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("It should create new KubernetesToDBResourceMapping and GitopsEngineCluster.")
			// ----------------------------------------------------------------------------

			gitopsEngineCluster, isNew, err = GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that GitopsEngineCluster and KubernetesToDBResourceMapping entries are created in DB.")
			// ----------------------------------------------------------------------------

			// Check GitopsEngineCluster is created.
			retrieveGitopsEngineCluster := db.GitopsEngineCluster{
				Gitopsenginecluster_id: gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbQueries.GetGitopsEngineClusterById(ctx, &retrieveGitopsEngineCluster)

			Expect(err).To(BeNil())
			Expect(db.GitopsEngineCluster{} == retrieveGitopsEngineCluster).To(BeFalse())

			//----------------------------------------------------------
			// Check KubernetesToDBResourceMapping is created.
			var foundMapping bool
			kubernetesToDBResourceMapping, foundMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)

			Expect(err).To(BeNil())
			Expect(foundMapping).To(BeTrue())
		})

		It("Should delete old KubernetesToDBResourceMapping, if corresponding GitopsEngineCluster doesnt exists and then create new GitopsEngineCluster and KubernetesToDBResourceMapping .", func() {

			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			kubernetesToDBResourceMappingFirst := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: db.K8sToDBMapping_Namespace,
				KubernetesResourceUID:  string(workSpaceUid),
				DBRelationType:         db.K8sToDBMapping_GitopsEngineCluster,
				DBRelationKey:          "dummy-relation-key",
			}

			err = dbQueries.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingFirst)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("It should delete old KubernetesToDBResourceMapping and create new KubernetesToDBResourceMapping and GitopsEngineCluster.")
			// ----------------------------------------------------------------------------

			gitopsEngineCluster, isNew, err = GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that old KubernetesToDBResourceMapping is deleted and GitopsEngineCluster and KubernetesToDBResourceMapping entries are created in DB.")
			// ----------------------------------------------------------------------------
			// Check new KubernetesToDBResourceMapping is created and old entry is deleted.
			var foundOldMapping, foundNewMapping bool

			_, foundOldMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, kubernetesToDBResourceMappingFirst.DBRelationKey, kubernetesToDBResourceMappingFirst.DBRelationType)
			Expect(err).To(BeNil())
			Expect(foundOldMapping).To(BeFalse())

			kubernetesToDBResourceMapping, foundNewMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
			Expect(foundNewMapping).To(BeTrue())
		})

		It("Should return existing GitopsEngineCluster and KubernetesToDBResourceMapping, instead of creating new.", func() {

			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("First create new KubernetesToDBResourceMapping and GitopsEngineCluster.")
			// ----------------------------------------------------------------------------

			gitopsEngineCluster, isNew, err = GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that GitopsEngineCluster and KubernetesToDBResourceMapping entries are created in DB.")
			// ----------------------------------------------------------------------------

			// Fetch KubernetesToDBResourceMapping to be used later.
			var foundMapping bool
			kubernetesToDBResourceMapping, foundMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
			Expect(foundMapping).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Call function again and It should return existing KubernetesToDBResourceMapping and GitopsEngineCluster, instead of creating new.")
			// ----------------------------------------------------------------------------

			retrieveGitopsEngineCluster, retrieveIsNewSecond, err := GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(retrieveIsNewSecond).To(BeFalse())

			// ----------------------------------------------------------------------------
			By("Verify exsting GitopsEngineCluster and KubernetesToDBResourceMapping entries are returned.")
			// ----------------------------------------------------------------------------

			Expect(gitopsEngineCluster).To(Equal(retrieveGitopsEngineCluster))

			kubernetesToDBResourceMappingSecond, _, err := findKubernetesToDBResourceMappingInTable(ctx, dbQueries, retrieveGitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
			Expect(kubernetesToDBResourceMappingSecond).To(Equal(kubernetesToDBResourceMapping))
		})

		It("Should return error as kubesystemNamespaceUID is empty.", func() {
			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			// To erase value of local variable set by previous test, otherwise condition to check not-nil in AfterEach will satisfy
			// and it would try to delete entry which is already been deleted in previous test and fail in check for number of rows affected.
			kubernetesToDBResourceMapping = db.KubernetesToDBResourceMapping{}

			gitopsEngineCluster, isNew, err = GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, "", dbQueries, log)

			Expect(err).NotTo(BeNil())
			Expect(strings.Contains(err.Error(), "field should not be empty string")).To(BeTrue())
			Expect(isNew).To(BeFalse())
		})

	})

	Context("Testing for GetGitopsEngineClusterByKubeSystemNamespaceUID function.", func() {

		var err error
		var log logr.Logger
		var ctx context.Context
		var workSpaceUid types.UID
		var dbQueries db.AllDatabaseQueries
		var gitopsEngineCluster *db.GitopsEngineCluster
		var kubernetesToDBResourceMapping db.KubernetesToDBResourceMapping

		AfterEach(func() {
			var resourcesToBeDeleted testResources

			// Objects could be nil, in that case it would throw error. To avoid that check object first
			if gitopsEngineCluster != nil {
				resourcesToBeDeleted.Gitopsenginecluster_id = gitopsEngineCluster.Gitopsenginecluster_id
				resourcesToBeDeleted.Clustercredentials_cred_id = gitopsEngineCluster.Clustercredentials_id
			}

			if kubernetesToDBResourceMapping.KubernetesResourceType != "" {
				resourcesToBeDeleted.kubernetesToDBResourceMapping = kubernetesToDBResourceMapping
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			defer dbQueries.CloseDatabase()
		})

		It("Should return nil, if KubernetesToDBResourceMapping and GitopsEngineCluster don't exist", func() {
			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("No GitopsEngineCluster instance should be returned.")
			// ----------------------------------------------------------------------------

			gitopsEngineCluster, err = GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(gitopsEngineCluster).To(BeNil())
		})

		It("Should return nil, if KubernetesToDBResourceMapping exists, but GitopsEngineCluster doesn't.", func() {
			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Create DeploymentToApplicationMapping.")
			// ----------------------------------------------------------------------------

			kubernetesToDBResourceMapping = db.KubernetesToDBResourceMapping{
				KubernetesResourceType: db.K8sToDBMapping_Namespace,
				KubernetesResourceUID:  string(workSpaceUid),
				DBRelationType:         db.K8sToDBMapping_GitopsEngineCluster,
				DBRelationKey:          "dummy-relation-key",
			}

			err = dbQueries.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("No GitopsEngineCluster instance should be returned.")
			// ----------------------------------------------------------------------------

			gitopsEngineCluster, err = GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(gitopsEngineCluster).To(BeNil())
		})

		It("Should return GitopsEngineCluster, if KubernetesToDBResourceMapping and GitopsEngineCluster exist in DB.", func() {

			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("First create new DeploymentToApplicationMapping and GitopsEngineCluster to be used later.")
			// ----------------------------------------------------------------------------

			gitopsEngineCluster, _, err = GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("Existing GitopsEngineCluster instance should be returned.")
			// ----------------------------------------------------------------------------

			retriveGitopsEngineCluster, err := GetGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(gitopsEngineCluster).To(Equal(retriveGitopsEngineCluster))

			// ----------------------------------------------------------------------------
			By("To cleanup the resource created by test")
			// ----------------------------------------------------------------------------

			// Fetch KubernetesToDBResourceMapping to be cleaned in AfterEach.
			kubernetesToDBResourceMapping, _, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
		})
	})

	Context("Testing for GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID function.", func() {
		var err error
		var isNew bool
		var log logr.Logger
		var ctx context.Context
		var workSpaceUid types.UID
		var workspace v1.Namespace
		var dbQueries db.AllDatabaseQueries
		var gitopsEngineCluster *db.GitopsEngineCluster
		var gitopsEngineInstance *db.GitopsEngineInstance
		var kubernetesToDBResourceMappingForEngineInstance db.KubernetesToDBResourceMapping
		var kubernetesToDBResourceMappingForEngineCluster db.KubernetesToDBResourceMapping

		BeforeEach(func() {
			ctx, dbQueries, log, workSpaceUid, err = initialSetUp()
			Expect(err).To(BeNil())

			workspace = v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					UID:       workSpaceUid,
					Namespace: "test-namespace",
				},
				Spec: v1.NamespaceSpec{},
			}
		})

		AfterEach(func() {
			var resourcesToBeDeleted testResources

			// Objects could be nil, in that case it would throw error. To avoid that check object first

			if gitopsEngineInstance != nil {
				resourcesToBeDeleted.Gitopsengineinstance_id = gitopsEngineInstance.Gitopsengineinstance_id
			}

			if gitopsEngineCluster != nil {
				resourcesToBeDeleted.Gitopsenginecluster_id = gitopsEngineCluster.Gitopsenginecluster_id
				resourcesToBeDeleted.Clustercredentials_cred_id = gitopsEngineCluster.Clustercredentials_id
			}

			if kubernetesToDBResourceMappingForEngineInstance.KubernetesResourceUID != "" {
				resourcesToBeDeleted.kubernetesToDBResourceMapping = kubernetesToDBResourceMappingForEngineInstance
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			// There are two kubernetesToDBResourceMapping objects are created in one of the test,
			// to clean both calling deleteTestResources twice, instead of making testResources.kubernetesToDBResourceMapping an Array just for one test.
			if kubernetesToDBResourceMappingForEngineCluster.KubernetesResourceUID != "" {
				resourcesToBeDeleted.kubernetesToDBResourceMapping = kubernetesToDBResourceMappingForEngineCluster
			}

			deleteTestResources(ctx, dbQueries, testResources{kubernetesToDBResourceMapping: kubernetesToDBResourceMappingForEngineCluster})

			defer dbQueries.CloseDatabase()
		})

		It("Should create new gitopsEngineInstance.", func() {

			// ----------------------------------------------------------------------------
			By("Create new GitopsEngineInstance and KubernetesToDBResourceMapping.")
			// ----------------------------------------------------------------------------

			gitopsEngineInstance, isNew, gitopsEngineCluster, err = GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, workspace, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that GitopsEngineInstance and KubernetesToDBResourceMapping entries are created in DB and clean them in AfterEach.")
			// ----------------------------------------------------------------------------

			// Check GitopsEngineInstance is created.
			retriveGitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbQueries.GetGitopsEngineInstanceById(ctx, &retriveGitopsEngineInstance)
			Expect(err).To(BeNil())
			Expect(&retriveGitopsEngineInstance).To(Equal(gitopsEngineInstance))

			// Check KubernetesToDBResourceMapping is created for GitopsEngineInstance and GitopsEngineCluster
			var foundGitopsEngineInstanceMapping, foundGitopsEngineClusterMapping bool

			kubernetesToDBResourceMappingForEngineInstance, foundGitopsEngineInstanceMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineInstance.Gitopsengineinstance_id, db.K8sToDBMapping_GitopsEngineInstance)
			Expect(err).To(BeNil())
			Expect(foundGitopsEngineInstanceMapping).To(BeTrue())

			kubernetesToDBResourceMappingForEngineCluster, foundGitopsEngineClusterMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
			Expect(foundGitopsEngineClusterMapping).To(BeTrue())
		})

		It("Should delete existing KubernetesToDBResourceMapping and create new KubernetesToDBResourceMapping and GitopsEngineInstance.", func() {

			// ----------------------------------------------------------------------------
			By("First create KubernetesToDBResourceMapping.")
			// ----------------------------------------------------------------------------

			kubernetesToDBResourceMappingFirst := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: db.K8sToDBMapping_Namespace,
				KubernetesResourceUID:  string(workSpaceUid),
				DBRelationType:         db.K8sToDBMapping_GitopsEngineInstance,
				DBRelationKey:          "dummy-relation-key",
			}

			err = dbQueries.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingFirst)
			Expect(err).To(BeNil())

			// ----------------------------------------------------------------------------
			By("It should delete existing KubernetesToDBResourceMapping and rerurn new gitopsEngineInstance and KubernetesToDBResourceMapping.")
			// ----------------------------------------------------------------------------

			gitopsEngineInstance, isNew, gitopsEngineCluster, err = GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, workspace, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("Verify that existing KubernetesToDBResourceMapping is deleted and new gitopsEngineInstance and KubernetesToDBResourceMapping entries are created in DB and clean them in AfterEach.")
			// ----------------------------------------------------------------------------

			// Check GitopsEngineInstance is created.
			retriveGitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbQueries.GetGitopsEngineInstanceById(ctx, &retriveGitopsEngineInstance)
			Expect(err).To(BeNil())
			Expect(&retriveGitopsEngineInstance).To(Equal(gitopsEngineInstance))

			var foundOldGitopsEngineInstanceMapping, foundNewGitopsEngineInstanceMapping, foundGitopsEngineClusterMapping bool

			// Check existing mapping for GitOpsEngineInstance is deleted.
			_, foundOldGitopsEngineInstanceMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, kubernetesToDBResourceMappingFirst.DBRelationKey, kubernetesToDBResourceMappingFirst.DBRelationType)
			Expect(err).To(BeNil())
			Expect(foundOldGitopsEngineInstanceMapping).To(BeFalse())

			// Check new mapping for GitOpsEngineInstance is created.
			kubernetesToDBResourceMappingForEngineInstance, foundNewGitopsEngineInstanceMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineInstance.Gitopsengineinstance_id, db.K8sToDBMapping_GitopsEngineInstance)
			Expect(err).To(BeNil())
			Expect(foundNewGitopsEngineInstanceMapping).To(BeTrue())

			// Check mapping for GitOpsEngineCluster is present.
			kubernetesToDBResourceMappingForEngineCluster, foundGitopsEngineClusterMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
			Expect(foundGitopsEngineClusterMapping).To(BeTrue())
		})

		It("Should return existing KubernetesToDBResourceMapping and GitopsEngineInstance, instead of creating new.", func() {

			// ----------------------------------------------------------------------------
			By("It should create new gitopsEngineInstance and KubernetesToDBResourceMapping.")
			// ----------------------------------------------------------------------------

			gitopsEngineInstanceFirst, isNew, gitopsEngineClusterFirst, err := GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, workspace, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeTrue())

			// ----------------------------------------------------------------------------
			By("It should return existing gitopsEngineInstance")
			// ----------------------------------------------------------------------------

			gitopsEngineInstance, isNew, gitopsEngineCluster, err = GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, workspace, string(workSpaceUid), dbQueries, log)

			Expect(err).To(BeNil())
			Expect(isNew).To(BeFalse())

			Expect(gitopsEngineInstance).To(Equal(gitopsEngineInstanceFirst))
			Expect(gitopsEngineCluster).To(Equal(gitopsEngineClusterFirst))

			// ----------------------------------------------------------------------------
			By("Fetch GitopsEngineInstance and KubernetesToDBResourceMapping entries to be cleaned by AfterEach.")
			// ----------------------------------------------------------------------------

			// Check GitopsEngineInstance is created.
			retriveGitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbQueries.GetGitopsEngineInstanceById(ctx, &retriveGitopsEngineInstance)
			Expect(err).To(BeNil())
			Expect(&retriveGitopsEngineInstance).To(Equal(gitopsEngineInstance))

			// Check KubernetesToDBResourceMapping is created for GitopsEngineInstance and GitopsEngineCluster
			var foundGitopsEngineInstanceMapping, foundGitopsEngineClusterMapping bool

			// Check mapping for GitOpsEngineInstance is present.
			kubernetesToDBResourceMappingForEngineInstance, foundGitopsEngineInstanceMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineInstance.Gitopsengineinstance_id, db.K8sToDBMapping_GitopsEngineInstance)
			Expect(err).To(BeNil())
			Expect(foundGitopsEngineInstanceMapping).To(BeTrue())

			// Check mapping for GitOpsEngineCluster is present.
			kubernetesToDBResourceMappingForEngineCluster, foundGitopsEngineClusterMapping, err = findKubernetesToDBResourceMappingInTable(ctx, dbQueries, gitopsEngineCluster.Gitopsenginecluster_id, db.K8sToDBMapping_GitopsEngineCluster)
			Expect(err).To(BeNil())
			Expect(foundGitopsEngineClusterMapping).To(BeTrue())

		})

		It("Should fail if workSpaceUid is empty.", func() {
			kubernetesToDBResourceMappingForEngineInstance = db.KubernetesToDBResourceMapping{}
			kubernetesToDBResourceMappingForEngineCluster = db.KubernetesToDBResourceMapping{}

			gitopsEngineInstance, isNew, gitopsEngineCluster, err = GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, workspace, "", dbQueries, log)

			Expect(err).NotTo(BeNil())
			Expect(isNew).To(BeFalse())
		})
	})
})

/*
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
*/
