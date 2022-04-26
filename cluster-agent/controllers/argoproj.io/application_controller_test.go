package argoprojio_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	argo "github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/argoproj.io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Application Controller", func() {
	Context("Application Controller Test", func() {
		const (
			name      = "my-application"
			namespace = "argocd"
			dbID      = "databaseID"
		)
		var ctx context.Context
		var managedEnvironment *db.ManagedEnvironment
		var gitopsEngineInstance *db.GitopsEngineInstance
		var dbQueries db.AllDatabaseQueries
		var guestbookApp *appv1.Application
		var reconciler argo.ApplicationReconciler
		var err error

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			// Database Connection.
			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			err = testSetup()
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err = createSampleData(dbQueries)
			Expect(err).To(BeNil())

			guestbookApp = &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-my-application",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
				Status: appv1.ApplicationStatus{
					Health: appv1.HealthStatus{
						Status: "Healthy",
					},
					Sync: appv1.SyncStatus{
						Status: "Synced",
					},
				},
			}

			reconciler = argo.ApplicationReconciler{
				Client:        k8sClient,
				Scheme:        scheme,
				DB:            dbQueries,
				TaskRetryLoop: sharedutil.NewTaskRetryLoop("application-reconciler"),
			}
		})

		It("Create New Application CR in namespace and database and verify that ApplicationState is created", func() {
			// Close database connection.
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			// Add databaseID label to applicationID.
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			// Create a new ArgoCD Application.
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			// Create Application in Database.
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			// Call reconcile function.
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			// Verify Application State is created in database.
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			// Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database.
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

		})

		It("Update an existing Application table in the database, call Reconcile on the Argo CD Application, and verify an existing ApplicationState DB entry is updated", func() {
			// Close database connection.
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			// Add databaseID label to applicationID.
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "OutOfSync",
			}

			// Create Application in Database.
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			// Create Application in Database.
			err = reconciler.DB.CreateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			// Create a new ArgoCD Application.
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			// Call reconcile function.
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			// Verify Application State is created in database.
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			// Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database.
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

		})

		It("Application CR missing corresponding namespace entry", func() {
			// Close database connection.
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			// Add databaseID label to applicationID.
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			// Create Application in Database.
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			// Call reconcile function.
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

		})

		It("Application CR missing corresponding database entry", func() {
			// Close database connection.
			defer dbQueries.CloseDatabase()
			defer testTeardown()
			Expect(err).To(BeNil())

			ctx = context.Background()

			guestbookApp = &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-wrong-input",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
				Status: appv1.ApplicationStatus{
					Health: appv1.HealthStatus{
						Status: "InProgress",
					},
					Sync: appv1.SyncStatus{
						Status: "OutOfSync",
					},
				},
			}

			// Add databaseID label to applicationID.
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			// Verify Application is present in database.
			err = reconciler.DB.GetApplicationById(ctx, applicationDB)
			Expect(err).ToNot(BeNil())

			// Create a new ArgoCD Application.
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			// Call reconcile function.
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

		})

		It("Delete the Argo CD Application resource, and call Reconcile. Assert that no error is returned", func() {
			// Close database connection.
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			// Add databaseID label to applicationID.
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}

			// Create a new ArgoCD Application.
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			// Create Application in Database.
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			// Call reconcile function.
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			// Verify Application State is created in database.
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			// Delete ArgoCD Application.
			err = reconciler.Delete(ctx, guestbookApp)
			Expect(err).To(BeNil())

			// Call reconcile function.
			_, err = reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())

		})

		It("Health and Sync Status of Application CR is empty, sanitize Health and sync status", func() {
			// Close database connection.
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			guestbookApp = &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						dbID: "test-my-application",
					},
				},
				Spec: appv1.ApplicationSpec{
					Source: appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Namespace: "guestbook",
						Server:    "https://kubernetes.default.svc",
					},
					Project: "default",
				},
			}

			// Add databaseID label to applicationID.
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			// Create a new ArgoCD Application.
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			// Create Application in Database.
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			// Call reconcile function.
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

		})
	})
})

// newRequest contains the information necessary to reconcile a Kubernetes object.
func newRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// Create sample data for testing.
func createSampleData(dbq db.AllDatabaseQueries) (*db.ClusterCredentials, *db.ManagedEnvironment, *db.GitopsEngineCluster, *db.GitopsEngineInstance, *db.ClusterAccess, error) {

	ctx := context.Background()
	var err error

	clusterCredentials, managedEnvironment, engineCluster, engineInstance, clusterAccess := generateSampleData()

	if err = dbq.CreateClusterCredentials(ctx, &clusterCredentials); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateGitopsEngineCluster(ctx, &engineCluster); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateGitopsEngineInstance(ctx, &engineInstance); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if err = dbq.CreateClusterAccess(ctx, &clusterAccess); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return &clusterCredentials, &managedEnvironment, &engineCluster, &engineInstance, &clusterAccess, nil

}

var testClusterUser = &db.ClusterUser{
	Clusteruser_id: "test-user",
	User_name:      "test-user",
}

// Generate fake cluster for testing.
func generateSampleData() (db.ClusterCredentials, db.ManagedEnvironment, db.GitopsEngineCluster, db.GitopsEngineInstance, db.ClusterAccess) {
	clusterCredentials := db.ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := db.ManagedEnvironment{
		Managedenvironment_id: "test-managed-env",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := db.GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	clusterAccess := db.ClusterAccess{
		Clusteraccess_user_id:                   testClusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
	}

	return clusterCredentials, managedEnvironment, gitopsEngineCluster, gitopsEngineInstance, clusterAccess
}

func testSetup() error {

	ctx := context.Background()

	// 'testSetup' deletes all database rows that start with 'test-' in the primary key of the row.
	// This ensures a clean slate for the test run.

	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	Expect(err).To(BeNil())

	defer dbq.CloseDatabase()

	var deploymentToApplicationMappings []db.DeploymentToApplicationMapping
	var syncOperations []db.SyncOperation

	err = dbq.UnsafeListAllDeploymentToApplicationMapping(ctx, &deploymentToApplicationMappings)
	Expect(err).To(BeNil())

	for _, deploydeploymentToApplicationMapping := range deploymentToApplicationMappings {
		if strings.HasPrefix(deploydeploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id, "test-") {
			rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploydeploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)
			Expect(err).To(BeNil())

			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	err = dbq.UnsafeListAllSyncOperations(ctx, &syncOperations)
	Expect(err).To(BeNil())

	for _, syncOperation := range syncOperations {
		if strings.HasPrefix(syncOperation.SyncOperation_id, "test-") {
			rowsAffected, err := dbq.DeleteSyncOperationById(ctx, syncOperation.SyncOperation_id)
			Expect(err).To(BeNil())

			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	var applicationStates []db.ApplicationState
	err = dbq.UnsafeListAllApplicationStates(ctx, &applicationStates)
	Expect(err).To(BeNil())

	for _, applicationState := range applicationStates {
		if strings.HasPrefix(applicationState.Applicationstate_application_id, "test-") {
			rowsAffected, err := dbq.DeleteApplicationStateById(ctx, applicationState.Applicationstate_application_id)
			Expect(err).To(BeNil())
			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	var operations []db.Operation
	err = dbq.UnsafeListAllOperations(ctx, &operations)
	Expect(err).To(BeNil())

	for _, operation := range operations {

		if strings.HasPrefix(operation.Operation_id, "test-") {
			rowsAffected, err := dbq.CheckedDeleteOperationById(ctx, operation.Operation_id, operation.Operation_owner_user_id)
			Expect(rowsAffected).Should((Equal(1)))
			Expect(err).To(BeNil())

		}
	}

	var applications []db.Application
	err = dbq.UnsafeListAllApplications(ctx, &applications)
	Expect(err).To(BeNil())

	for _, application := range applications {
		if strings.HasPrefix(application.Application_id, "test-") {
			rowsAffected, err := dbq.DeleteApplicationById(ctx, application.Application_id)
			Expect(rowsAffected).Should((Equal(1)))
			Expect(err).To(BeNil())

		}
	}

	var clusterAccess []db.ClusterAccess
	err = dbq.UnsafeListAllClusterAccess(ctx, &clusterAccess)
	Expect(err).To(BeNil())

	for _, clusterAccess := range clusterAccess {
		if strings.HasPrefix(clusterAccess.Clusteraccess_managed_environment_id, "test-") {
			rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.Clusteraccess_user_id,
				clusterAccess.Clusteraccess_managed_environment_id,
				clusterAccess.Clusteraccess_gitops_engine_instance_id)
			Expect(err).To(BeNil())

			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	var engineInstances []db.GitopsEngineInstance
	err = dbq.UnsafeListAllGitopsEngineInstances(ctx, &engineInstances)
	Expect(err).To(BeNil())

	for _, gitopsEngineInstance := range engineInstances {
		if strings.HasPrefix(gitopsEngineInstance.Gitopsengineinstance_id, "test-") {

			rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)

			if !Expect(err).To(BeNil()) {
				return err
			}
			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	var engineClusters []db.GitopsEngineCluster
	err = dbq.UnsafeListAllGitopsEngineClusters(ctx, &engineClusters)
	Expect(err).To(BeNil())

	for _, engineCluster := range engineClusters {
		if strings.HasPrefix(engineCluster.Gitopsenginecluster_id, "test-") {
			rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, engineCluster.Gitopsenginecluster_id)
			Expect(err).To(BeNil())

			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	var managedEnvironments []db.ManagedEnvironment
	err = dbq.UnsafeListAllManagedEnvironments(ctx, &managedEnvironments)
	Expect(err).To(BeNil())

	for _, managedEnvironment := range managedEnvironments {
		if strings.HasPrefix(managedEnvironment.Managedenvironment_id, "test-") {
			rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
			Expect(rowsAffected).Should((Equal(1)))
			Expect(err).To(BeNil())

		}
	}

	var clusterCredentials []db.ClusterCredentials
	err = dbq.UnsafeListAllClusterCredentials(ctx, &clusterCredentials)
	Expect(err).To(BeNil())

	for _, clusterCredential := range clusterCredentials {
		if strings.HasPrefix(clusterCredential.Clustercredentials_cred_id, "test-") {
			rowsAffected, err := dbq.DeleteClusterCredentialsById(ctx, clusterCredential.Clustercredentials_cred_id)
			Expect(err).To(BeNil())

			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}

	var clusterUsers []db.ClusterUser
	if err = dbq.UnsafeListAllClusterUsers(ctx, &clusterUsers); !Expect(err).To(BeNil()) {
		return err
	}

	for _, user := range clusterUsers {
		if strings.HasPrefix(user.Clusteruser_id, "test-") {
			rowsAffected, err := dbq.DeleteClusterUserById(ctx, (user.Clusteruser_id))
			Expect(rowsAffected).Should((Equal(1)))
			Expect(err).To(BeNil())

		}
	}

	err = dbq.CreateClusterUser(ctx, testClusterUser)
	Expect(err).To(BeNil())

	var kubernetesToDBResourceMappings []db.KubernetesToDBResourceMapping
	err = dbq.UnsafeListAllKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappings)
	Expect(err).To(BeNil())

	for _, kubernetesToDBResourceMapping := range kubernetesToDBResourceMappings {
		if strings.HasPrefix(kubernetesToDBResourceMapping.KubernetesResourceUID, "test-") {

			rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)

			if !Expect(err).To(BeNil()) {
				return err
			}
			if err == nil {
				Expect(rowsAffected).Should((Equal(1)))
			}
		}
	}
	return nil
}

func testTeardown() {
	err := testSetup()
	Expect(err).To(BeNil())
}
