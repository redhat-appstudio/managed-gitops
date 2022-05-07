package argoprojio_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
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

			err = db.SetupForTestingDBGinkgo()
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
				Cache:         dbutil.NewApplicationInfoCache(),
			}
		})

		AfterEach(func() {
			// Prevent race conditions between tests, by shutting down the cache after each test
			reconciler.Cache.DebugOnly_Shutdown(context.Background())
		})

		It("Create New Application CR in namespace and database and verify that ApplicationState is created", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
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

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			By("Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database")
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

		})

		It("Update an existing Application table in the database, call Reconcile on the Argo CD Application, and verify an existing ApplicationState DB entry is updated", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
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

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Create ApplicationState in Database")
			err = reconciler.DB.CreateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			By("Verify that Health and Status of ArgoCD Application is equal to the Health and Status of Application in database")
			Expect(applicationState.Health).To(Equal(string(guestbookApp.Status.Health.Status)))
			Expect(applicationState.Sync_Status).To(Equal(string(guestbookApp.Status.Sync.Status)))

		})

		It("Calls Reconcile on an Argo CD Application resource that doesn't exist", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

		})

		It("Has an Argo CD Application that points to a database entry that doesn't exist", func() {
			By("Close database connection")
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

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			By("Verify Application is not present in database")
			err = reconciler.DB.GetApplicationById(ctx, applicationDB)
			Expect(err).ToNot(BeNil())

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

		})

		It("Delete the Argo CD Application resource, and call Reconcile. Assert that no error is returned", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			ctx = context.Background()

			By("Add databaseID label to applicationID")
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

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

			By("Verify Application State is created in database")
			err = reconciler.DB.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			By("Delete ArgoCD Application")
			err = reconciler.Delete(ctx, guestbookApp)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			_, err = reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())

		})

		It("Health and Sync Status of Application CR is empty, sanitize Health and sync status", func() {
			By("Close database connection")
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

			By("Add databaseID label to applicationID")
			databaseID := guestbookApp.Labels[dbID]
			applicationDB := &db.Application{
				Application_id:          databaseID,
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			By("Create a new ArgoCD Application")
			err = reconciler.Create(ctx, guestbookApp)
			Expect(err).To(BeNil())

			By("Create Application in Database")
			err = reconciler.DB.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Call reconcile function")
			result, err := reconciler.Reconcile(ctx, newRequest(namespace, name))
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())

		})
	})

	Context("Test compressResourceData function", func() {
		It("Should compress resource data into byte array", func() {
			resourceStatus := appv1.ResourceStatus{
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "argoCD",
				Name:      "component-a",
				Status:    "Synced",
				Health: &appv1.HealthStatus{
					Status:  "Healthy",
					Message: "success",
				},
			}

			var resources []appv1.ResourceStatus
			resources = append(resources, resourceStatus)

			byteArr, err := argo.CompressResourceData(resources)

			Expect(err).To(BeNil())
			Expect(byteArr).NotTo(BeEmpty())
		})

		It("Should work for empty ResourceStatus", func() {
			var resourceStatus appv1.ResourceStatus
			var resources []appv1.ResourceStatus
			resources = append(resources, resourceStatus)

			byteArr, err := argo.CompressResourceData(resources)

			Expect(err).To(BeNil())
			Expect(byteArr).NotTo(BeEmpty())
		})

		It("Should work for empty Resource Array", func() {
			var resources []appv1.ResourceStatus

			byteArr, err := argo.CompressResourceData(resources)

			Expect(err).To(BeNil())
			Expect(byteArr).NotTo(BeEmpty())
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

func testTeardown() {
	err := db.SetupForTestingDBGinkgo()
	Expect(err).To(BeNil())
}
