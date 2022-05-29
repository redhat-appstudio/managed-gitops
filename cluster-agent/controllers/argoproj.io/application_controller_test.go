package argoprojio_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	appEventLoop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/util/fauxargocd"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	cache "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	argo "github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/argoproj.io"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
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

var _ = Describe("Namespace Reconciler Tests.", func() {
	var reconciler argo.ApplicationReconciler

	Context("Testing for Namespace Reconciler.", func() {
		var err error
		var ctx context.Context
		var argoCdApp appv1.Application
		var dummyApplicationSpec string
		var applicationput db.Application
		var dbQueries db.AllDatabaseQueries

		BeforeEach(func() {
			ctx = context.Background()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			_, dummyApplicationSpec, argoCdApp, err = createDummyApplicationData()
			Expect(err).To(BeNil())

			applicationput = db.Application{
				Application_id:          "test-my-application",
				Name:                    "test-my-application",
				Spec_field:              dummyApplicationSpec,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, &applicationput)
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			reconciler = argo.ApplicationReconciler{
				Client: k8sClient,
				DB:     dbQueries,
				Cache:  dbutil.NewApplicationInfoCache(),
			}

			err = reconciler.Create(ctx, &argoCdApp)
			Expect(err).To(BeNil())

			var speCialClusterUser db.ClusterUser
			err = dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &speCialClusterUser)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			listOfK8sOperation := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).To(BeNil())

			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[appEventLoop.IdentifierKey] == appEventLoop.IdentifierValue {
					rowsAffected, err := dbQueries.DeleteOperationById(ctx, k8sOperation.Spec.OperationID)
					Expect(err).To(BeNil())
					Expect(rowsAffected).Should((Equal(1)))

					err = reconciler.Delete(ctx, &k8sOperation)
					Expect(err).To(BeNil())
				}
			}
		})

		It("Should do nothing as ArgoCD Application is in sync with DB entry.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			// Call function for workSpace/Namespace reconciler
			argo.RunNamespaceReconcile(ctx, reconciler.DB, reconciler.Client, log)

			// We are using a fake k8s client and because of that we can not check if ArgoCD application has been created/updated.
			// We will just check if k8s Operation created or not.

			By("Verify that K8s Operation and Operation table entry are created.")

			listOfK8sOperation := v1alpha1.OperationList{}

			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).To(BeNil())

			count := 0
			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[appEventLoop.IdentifierKey] == appEventLoop.IdentifierValue {
					// Fetch corresponding DB entry
					dbOperation := db.Operation{
						Operation_id: k8sOperation.Spec.OperationID,
					}
					err = dbQueries.GetOperationById(ctx, &dbOperation)
					Expect(err).To(BeNil())
					if dbOperation.Resource_id == applicationput.Application_id {
						count += 1
					}
				}
			}
			Expect(count).To(Equal(0))
		})

		It("Should update OutofSync ArgoCD Application to keep it in sync with DB entry.", func() {

			// Update ArgoCD Application, so it will be out of Sync with DB entry.
			argoCdApp.Spec.Source.RepoURL = "https://github.com/test/gitops-repository-template"

			err = reconciler.Update(ctx, &argoCdApp)
			Expect(err).To(BeNil())

			ctx := context.Background()
			log := log.FromContext(ctx)

			// Call function for workSpace/Namespace reconciler
			argo.RunNamespaceReconcile(ctx, reconciler.DB, reconciler.Client, log)

			// We are using a fake k8s client and because of that we can not check if ArgoCD application has been created/updated.
			// We will just check if k8s Operation and DB entries are created and assume that in actual environment ArgoCD will pick up this Operation and update/create the application.

			By("Verify that K8s Operation and Operation table entry are created.")

			listOfK8sOperation := v1alpha1.OperationList{}

			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperation.Items)).NotTo(Equal(0))

			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[appEventLoop.IdentifierKey] == appEventLoop.IdentifierValue {
					// Fetch corresponding DB entry
					dbOperation := db.Operation{
						Operation_id: k8sOperation.Spec.OperationID,
					}
					err = dbQueries.GetOperationById(ctx, &dbOperation)
					Expect(err).To(BeNil())
					if dbOperation.Resource_id == applicationput.Application_id {
						Expect(dbOperation.Resource_id).To(Equal(applicationput.Application_id))
						Expect(dbOperation.Instance_id).To(Equal(applicationput.Engine_instance_inst_id))
					}
				}
			}
		})

		It("Should create new ArgoCD Application if application exists in DB, but it is not available in ArgoCD.", func() {

			// Delete the application from ArgoCD, but keep all DB entries.
			err = reconciler.Delete(ctx, &argoCdApp)
			Expect(err).To(BeNil())

			ctx := context.Background()
			log := log.FromContext(ctx)

			// Call function for workSpace/Namespace reconciler
			argo.RunNamespaceReconcile(ctx, reconciler.DB, reconciler.Client, log)

			// We are using a fake k8s client and because of that we can not check if ArgoCD application has been created/updated.
			// We will just check if k8s Operation and DB entries are created and assume that in actual environment ArgoCD will pick up this Operation and update/create the application.

			By("Verify that K8s Operation and Operation table entry are created.")

			listOfK8sOperation := v1alpha1.OperationList{}

			err = reconciler.List(ctx, &listOfK8sOperation)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperation.Items)).NotTo(Equal(0))

			for _, k8sOperation := range listOfK8sOperation.Items {
				// Look for Operation created by Namespace Reconciler.
				if k8sOperation.Annotations[appEventLoop.IdentifierKey] == appEventLoop.IdentifierValue {
					// Fetch corresponding DB entry
					dbOperation := db.Operation{
						Operation_id: k8sOperation.Spec.OperationID,
					}
					err = dbQueries.GetOperationById(ctx, &dbOperation)
					Expect(err).To(BeNil())
					if dbOperation.Resource_id == applicationput.Application_id {
						Expect(dbOperation.Resource_id).To(Equal(applicationput.Application_id))
						Expect(dbOperation.Instance_id).To(Equal(applicationput.Engine_instance_inst_id))
					}
				}
			}
		})
	})

	Context("Testing for CompareApplications function.", func() {
		It("Should compare applications.", func() {

			applicationFromDB, _, applicationFromArgoCD, err := createDummyApplicationData()
			Expect(err).To(BeNil())

			var ctx context.Context
			log := log.FromContext(ctx)

			result := argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeTrue())

			// Set different value in each field then revert them, otherwise next field wont be compared
			applicationFromArgoCD.APIVersion = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.APIVersion = applicationFromDB.APIVersion // Revert the value, to compare next field.

			applicationFromArgoCD.Kind = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Kind = applicationFromDB.Kind

			applicationFromArgoCD.Name = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Name = applicationFromDB.Name

			applicationFromArgoCD.Namespace = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Namespace = applicationFromDB.Namespace

			applicationFromArgoCD.Spec.Source.RepoURL = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Source.RepoURL = applicationFromDB.Spec.Source.RepoURL

			applicationFromArgoCD.Spec.Source.Path = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Source.Path = applicationFromDB.Spec.Source.Path

			applicationFromArgoCD.Spec.Source.TargetRevision = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Source.TargetRevision = applicationFromDB.Spec.Source.TargetRevision

			applicationFromArgoCD.Spec.Destination.Server = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Destination.Server = applicationFromDB.Spec.Destination.Server

			applicationFromArgoCD.Spec.Destination.Namespace = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Destination.Namespace = applicationFromDB.Spec.Destination.Namespace

			applicationFromArgoCD.Spec.Destination.Name = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Destination.Name = applicationFromDB.Spec.Destination.Name

			applicationFromArgoCD.Spec.Project = "test"
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.Project = applicationFromDB.Spec.Project

			applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune = true
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune = applicationFromDB.Spec.SyncPolicy.Automated.Prune

			applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal = true
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal = applicationFromDB.Spec.SyncPolicy.Automated.SelfHeal

			applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty = true
			result = argo.CompareApplications(applicationFromArgoCD, applicationFromDB, log)
			Expect(result).To(BeFalse())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty = applicationFromDB.Spec.SyncPolicy.Automated.AllowEmpty
		})
	})

	Context("Testing CleanK8sOperations function", func() {
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
			Expect(err).To(BeNil())

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			_, dummyApplicationSpec, argoCdApp, err = createDummyApplicationData()
			Expect(err).To(BeNil())

			applicationput = db.Application{
				Application_id:          "test-my-application",
				Name:                    "test-my-application",
				Spec_field:              dummyApplicationSpec,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, &applicationput)
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			// Fake kube client.
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			reconciler = argo.ApplicationReconciler{
				Client: k8sClient,
				DB:     dbQueries,
				Cache:  dbutil.NewApplicationInfoCache(),
			}

			err = reconciler.Create(ctx, &argoCdApp)
			Expect(err).To(BeNil())

			var speCialClusterUser db.ClusterUser
			err = dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &speCialClusterUser)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			for _, operation := range operationList {
				rowsAffected, err := dbQueries.CheckedDeleteOperationById(ctx, operation.Operation_id, operation.Operation_owner_user_id)
				Expect(rowsAffected).Should((Equal(1)))
				Expect(err).To(BeNil())
			}
			// Empty Operation List
			operationList = []db.Operation{}
		})

		It("Should delete Operations from cluster and if operation is completed.", func() {

			ctx := context.Background()
			log := log.FromContext(ctx)

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, dbOperation, err := appEventLoop.CreateOperation(ctx, false, dbOperationInput,
				db.SpecialClusterUserName, cache.GetGitOpsEngineSingleInstanceNamespace(), reconciler.DB, reconciler.Client, log)
			Expect(err).To(BeNil())

			dbOperation.State = "Completed"
			err = dbQueries.UpdateOperation(ctx, dbOperation)
			Expect(err).To(BeNil())

			operationList = append(operationList, *dbOperation)

			// Get list of Operations before cleanup.
			listOfK8sOperationFirst := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationFirst)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationFirst.Items)).NotTo(Equal(0))

			// Clean Operations
			argo.CleanK8sOperations(ctx, dbQueries, reconciler.Client, log)

			// Get list of Operations after cleanup.
			listOfK8sOperationSecond := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationSecond)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationSecond.Items)).To(Equal(0))
		})

		It("Should not delete Operations from cluster and if operation is not completed.", func() {

			ctx := context.Background()
			log := log.FromContext(ctx)

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, dbOperation, err := appEventLoop.CreateOperation(ctx, false, dbOperationInput,
				db.SpecialClusterUserName, cache.GetGitOpsEngineSingleInstanceNamespace(), reconciler.DB, reconciler.Client, log)
			Expect(err).To(BeNil())

			operationList = append(operationList, *dbOperation)

			// Get list of Operations before cleanup.
			listOfK8sOperationFirst := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationFirst)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationFirst.Items)).NotTo(Equal(0))

			// Clean Operations
			argo.CleanK8sOperations(ctx, dbQueries, reconciler.Client, log)

			// Get list of Operations after cleanup.
			listOfK8sOperationSecond := v1alpha1.OperationList{}
			err = reconciler.List(ctx, &listOfK8sOperationSecond)
			Expect(err).To(BeNil())
			Expect(len(listOfK8sOperationSecond.Items)).NotTo(Equal(0))
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

func createDummyApplicationData() (fauxargocd.FauxApplication, string, appv1.Application, error) {
	// Create dummy ArgoCD Application CR.
	dummyArgoCdApplication := appv1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-application",
			Namespace: "gitops-service-argocd",
		},
		Spec: appv1.ApplicationSpec{
			Source: appv1.ApplicationSource{
				RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
				Path:           "environments/overlays/dev",
				TargetRevision: "",
			},
			Destination: appv1.ApplicationDestination{
				Name:      "in-cluster",
				Namespace: "test-fake-namespace",
			},
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{
					Prune: false,
				},
			},
			Project: "default",
		},
	}

	// Create dummy Application Spec to be saved in DB
	dummyApplicationSpec := fauxargocd.FauxApplication{
		FauxTypeMeta: fauxargocd.FauxTypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		FauxObjectMeta: fauxargocd.FauxObjectMeta{
			Name:      "test-my-application",
			Namespace: "gitops-service-argocd",
		},
		Spec: fauxargocd.FauxApplicationSpec{
			Source: fauxargocd.ApplicationSource{
				RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
				Path:           "environments/overlays/dev",
				TargetRevision: "",
			},
			Destination: fauxargocd.ApplicationDestination{
				Name:      "in-cluster",
				Namespace: "test-fake-namespace",
			},
			SyncPolicy: &fauxargocd.SyncPolicy{
				Automated: &fauxargocd.SyncPolicyAutomated{
					Prune: false,
				},
			},
			Project: "default",
		},
	}

	dummyApplicationSpecBytes, err := yaml.Marshal(dummyApplicationSpec)

	if err != nil {
		return fauxargocd.FauxApplication{}, "", appv1.Application{}, err
	}

	return dummyApplicationSpec, string(dummyApplicationSpecBytes), dummyArgoCdApplication, nil
}
