package eventloop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	sharedoperations "github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
)

var _ = Describe("Operation Controller", func() {
	const (
		name      = "operation"
		namespace = "argocd"
		dbID      = "databaseID"
	)
	Context("Operation Controller Test", func() {

		var ctx context.Context
		var dbQueries db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var task processOperationEventTask
		var logger logr.Logger
		var kubesystemNamespace *v1.Namespace
		var argocdNamespace *v1.Namespace
		var workspace *v1.Namespace
		var scheme *runtime.Scheme
		var testClusterUser *db.ClusterUser
		var err error

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			testClusterUser = &db.ClusterUser{
				ClusterUserID: "test-user",
				UserName:      "test-user",
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
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

			By("Initialize fake kube client")
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			task = processOperationEventTask{
				log: logger,
				event: operationEventLoopEvent{
					request: newRequest(namespace, name),
					client:  k8sClient,
				},
			}

		})
		It("Ensure that calling perform task on an operation CR for Application that doesn't exist, it doesn't return an error, and retry is false", func() {
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:         "test-operation",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				ResourceType:         db.OperationResourceType_Application,
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

		})

		It("ensures that if the operation row doesn't exist, an error is not returned, and retry is false", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			operationDB := &db.Operation{
				Operation_id:         "test-operation",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				ResourceType:         db.OperationResourceType_Application,
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
			Expect(err).To(BeNil())

			By("Operation row(test-wrong-operation) doesn't exists")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: "test-wrong-operation",
				},
			}

			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

		})

		It("ensures that if the kube-system namespace does not having a matching namespace uid, an error is not returned, but retry it true", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			By("'kube-system' namespace has a UID that is not found in a corresponding row in GitOpsEngineCluster database")
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())
			Expect(kubesystemNamespace.UID).ToNot(Equal(gitopsEngineInstance.NamespaceUID))

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:         "test-operation",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				ResourceType:         db.OperationResourceType_Application,
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
			Expect(err).To(BeNil())

			By("Operation CR exists")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}

			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeTrue())

		})

		It("Ensures that if the GitopsEngineInstance's namespace_name field doesn't exist, an error is returned, and retry is true", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
			gitopsEngineInstance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance",
				NamespaceName:           "doesn't-exist",
				NamespaceUID:            string("doesnt-exist-uid"),
				EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:         "test-operation",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				ResourceType:         db.OperationResourceType_Application,
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
			Expect(err).To(BeNil())

			By("creating Operation CR")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}

			By("creating Operation CR")
			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(BeNil())
			Expect(retry).To(BeTrue())

			kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  string(kubesystemNamespace.UID),
				DBRelationType:         "GitopsEngineCluster",
				DBRelationKey:          gitopsEngineCluster.PrimaryKeyID,
			}

			By("deleting resources and cleaning up db entries created by test.")
			resourcesToBeDeleted := testResources{
				Operation_id:                  []string{operationDB.Operation_id},
				PrimaryKeyID:                  gitopsEngineCluster.PrimaryKeyID,
				Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
				ClusterCredentials_id:         gitopsEngineCluster.ClusterCredentialsID,
				kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

		})

		Context("Process Application Operation Test", func() {
			It("Verify that When an Operation row points to an Application row that doesn't exist, any Argo Application CR that relates to that Application row should be removed.", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()

				applicationCR := &appv1.Application{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Application",
						APIVersion: "argoproj.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "my-user",
						Labels: map[string]string{
							dbID: "doesnt-exist",
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
					},
				}

				err = task.event.client.Create(ctx, applicationCR)
				Expect(err).To(BeNil())

				// The Argo CD Applications used by GitOps Service use finalizers, so the Applicaitonwill not be deleted until the finalizer is removed.
				// Normally it us Argo CD's job to do this, but since this is a unit test, there is no Argo CD. Instead we wait for the deletiontimestamp
				// to be set (by the delete call of PerformTask, and then just remove the finalize and update, simulating what Argo CD does)
				go func() {
					err = wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
						if applicationCR.DeletionTimestamp != nil {
							err = k8sClient.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
							Expect(err).To(BeNil())

							applicationCR.Finalizers = nil

							err = k8sClient.Update(ctx, applicationCR)
							Expect(err).To(BeNil())

							err = task.event.client.Delete(ctx, applicationCR)
							return true, nil
						}
						return false, nil
					})
				}()

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).To(BeNil())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					NamespaceName:           workspace.Name,
					NamespaceUID:            string(workspace.UID),
					EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
				}

				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).To(BeNil())

				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:         "test-operation",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           "doesnt-exist",
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Creating Operation CR")
				operationCR := &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB.Operation_id,
					},
				}

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				By("Verifying whether Application CR is deleted")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, applicationCR)
					return apierr.IsNotFound(err)
				}).Should(BeTrue())

				kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
					KubernetesResourceType: "Namespace",
					KubernetesResourceUID:  string(kubesystemNamespace.UID),
					DBRelationType:         "GitopsEngineCluster",
					DBRelationKey:          gitopsEngineCluster.PrimaryKeyID,
				}

				By("deleting resources and cleaning up db entries created by test.")
				resourcesToBeDeleted := testResources{
					Operation_id:                  []string{operationDB.Operation_id},
					PrimaryKeyID:                  gitopsEngineCluster.PrimaryKeyID,
					Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:         gitopsEngineCluster.ClusterCredentialsID,
					kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify that when an Operation row points to an Application row that exists in the database, but doesn't exist in the Argo CD namespace, it should be created.", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).To(BeNil())

				dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).To(BeNil())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).To(BeNil())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					NamespaceName:           workspace.Name,
					NamespaceUID:            string(workspace.UID),
					EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).To(BeNil())

				By("Create Application in Database")
				applicationDB := &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   name,
					SpecField:              dummyApplicationSpecString,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
				}

				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).To(BeNil())

				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:         "test-operation",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           applicationDB.ApplicationID,
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Creating Operation CR")
				operationCR := &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB.Operation_id,
					},
				}

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				By("Verifying whether Application CR is created")
				applicationCR := appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: "my-user",
					},
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: name}, &applicationCR)
				Expect(err).To(BeNil())
				Expect(dummyApplicationSpec.Spec).To(Equal(applicationCR.Spec))

				kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
					KubernetesResourceType: "Namespace",
					KubernetesResourceUID:  string(kubesystemNamespace.UID),
					DBRelationType:         "GitopsEngineCluster",
					DBRelationKey:          gitopsEngineCluster.PrimaryKeyID,
				}

				By("deleting resources and cleaning up db entries created by test.")
				resourcesToBeDeleted := testResources{
					ApplicationID:                 applicationDB.ApplicationID,
					Operation_id:                  []string{operationDB.Operation_id},
					PrimaryKeyID:                  gitopsEngineCluster.PrimaryKeyID,
					Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:         gitopsEngineCluster.ClusterCredentialsID,
					kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify that Application CR should be updated to be consistent with the Application row", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).To(BeNil())

				_, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).To(BeNil())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).To(BeNil())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					NamespaceName:           workspace.Name,
					NamespaceUID:            string(workspace.UID),
					EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).To(BeNil())

				applicationDB := &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   name,
					SpecField:              dummyApplicationSpecString,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
				}

				By("Create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).To(BeNil())

				By("Creating new operation row in database")
				operationDB := &db.Operation{
					Operation_id:         "test-operation",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           applicationDB.ApplicationID,
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Creating Operation CR")
				operationCR := &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB.Operation_id,
					},
				}

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				By("creating a new spec and putting it into the Application in the database, so that operation wlil update it")
				newSpecApp, newSpecString, err := createCustomizedDummyApplicationData("different-path")
				Expect(err).To(BeNil())

				By("Update Application in Database")
				applicationUpdate := &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   applicationDB.Name,
					SpecField:              newSpecString,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
					SeqID:                  101,
					CreatedOn:              applicationDB.CreatedOn,
				}

				err = dbQueries.UpdateApplication(ctx, applicationUpdate)
				Expect(err).To(BeNil())

				By("Creating new operation row in database")
				operationDB2 := &db.Operation{
					Operation_id:         "test-operation-2",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           applicationDB.ApplicationID,
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB2, operationDB2.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Create new operation CR")
				operationCR = &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      sharedoperations.GenerateOperationCRName(*operationDB2),
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB2.Operation_id,
					},
				}

				By("updating the task that we are calling PerformTask with to point to the new operation")
				task.event.request.Name = operationCR.Name
				task.event.request.Namespace = operationCR.Namespace

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				By("Verifying whether Application CR is created")
				applicationCR := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "my-user",
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).To(BeNil())

				By("Call Perform task again and verify that update works: the Application CR should now have the updated spec from the database.")
				retry, err = task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())
				Expect(newSpecApp.Spec).To(Equal(applicationCR.Spec), "PerformTask should have updated the Application CR to be consistent with the new spec in the database")

				kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
					KubernetesResourceType: "Namespace",
					KubernetesResourceUID:  string(kubesystemNamespace.UID),
					DBRelationType:         "GitopsEngineCluster",
					DBRelationKey:          gitopsEngineCluster.PrimaryKeyID,
				}

				By("deleting resources and cleaning up db entries created by test.")

				resourcesToBeDeleted := testResources{
					ApplicationID:                 applicationDB.ApplicationID,
					Operation_id:                  []string{operationDB.Operation_id, operationDB2.Operation_id},
					PrimaryKeyID:                  gitopsEngineCluster.PrimaryKeyID,
					Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:         gitopsEngineCluster.ClusterCredentialsID,
					kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify that SyncOption is picked up by Perform Task to be in sync for CreateNamespace=true", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()
				err = db.SetupForTestingDBGinkgo()
				Expect(err).To(BeNil())

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).To(BeNil())

				dummyApplication, dummyApplicationSpecString, err := createApplicationWithSyncOption("CreateNamespace=true")
				Expect(err).To(BeNil())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).To(BeNil())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					NamespaceName:           workspace.Name,
					NamespaceUID:            string(workspace.UID),
					EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).To(BeNil())

				applicationDB := &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   name,
					SpecField:              dummyApplicationSpecString,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
				}

				By("Create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).To(BeNil())

				By("Creating new operation row in database")
				operationDB := &db.Operation{
					Operation_id:         "test-operation",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           applicationDB.ApplicationID,
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Creating Operation CR")
				operationCR := &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB.Operation_id,
					},
				}

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By("Verifying whether Application CR is created")
				applicationCR := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "my-user",
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).To(BeNil())

				By("Verify that the SyncOption in the Application has Option - CreateNamespace=true")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))
				Expect(dummyApplication.Spec.SyncPolicy.SyncOptions).To(Equal(applicationCR.Spec.SyncPolicy.SyncOptions))
				Expect(applicationCR.Spec.SyncPolicy.SyncOptions.HasOption("CreateNamespace=true")).To(Equal(true))

				//############################################################################

				By("Update the SyncOption to not have option CreateNamespace=true")
				newSpecApp, newSpecString, err := createApplicationWithSyncOption("")
				Expect(err).To(BeNil())

				By("Update Application in Database")
				applicationUpdate := &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   applicationDB.Name,
					SpecField:              newSpecString,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
					SeqID:                  101,
					CreatedOn:              applicationDB.CreatedOn,
				}

				err = dbQueries.UpdateApplication(ctx, applicationUpdate)
				Expect(err).To(BeNil())

				By("Creating new operation row in database")
				operationDB2 := &db.Operation{
					Operation_id:         "test-operation-2",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           applicationDB.ApplicationID,
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB2, operationDB2.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Create new operation CR")
				operationCR = &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      sharedoperations.GenerateOperationCRName(*operationDB2),
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB2.Operation_id,
					},
				}

				By("updating the task that we are calling PerformTask with to point to the new operation")
				task.event.request.Name = operationCR.Name
				task.event.request.Namespace = operationCR.Namespace

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				By("Verifying whether Application CR is created")
				applicationCR = &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "my-user",
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).To(BeNil())

				By("Call Perform task again and verify that update works: the Application CR should now have the updated spec from the database.")
				retry, err = task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).To(BeNil())
				Expect(newSpecApp.Spec.SyncPolicy.SyncOptions).To(Equal(applicationCR.Spec.SyncPolicy.SyncOptions), "PerformTask should have updated the Application CR to be consistent with the new spec(SyncOption) in the database")
				Expect(applicationCR.Spec.SyncPolicy.SyncOptions.HasOption("CreateNamespace=true")).To(Equal(false))

				By("Verify that the SyncOption in the Application has Option - CreateNamespace=true and the operation is completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				//############################################################################

				By("Update the SyncOption to have option CreateNamespace=true")
				newSpecApp2, newSpecString2, err := createApplicationWithSyncOption("CreateNamespace=true")
				Expect(err).To(BeNil())

				By("Update Application in Database")
				applicationUpdate2 := &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   applicationDB.Name,
					SpecField:              newSpecString2,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
					SeqID:                  101,
					CreatedOn:              applicationDB.CreatedOn,
				}

				err = dbQueries.UpdateApplication(ctx, applicationUpdate2)
				Expect(err).To(BeNil())

				By("Creating new operation row in database")
				operationDBUpdate2 := &db.Operation{
					Operation_id:         "test-operation-3",
					InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
					ResourceID:           applicationDB.ApplicationID,
					ResourceType:         "Application",
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDBUpdate2, operationDBUpdate2.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("Create new operation CR")
				operationCR = &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      sharedoperations.GenerateOperationCRName(*operationDBUpdate2),
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDBUpdate2.Operation_id,
					},
				}

				By("updating the task that we are calling PerformTask with to point to the new operation")
				task.event.request.Name = operationCR.Name
				task.event.request.Namespace = operationCR.Namespace

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())

				By("Verifying whether Application CR is created")
				applicationCR2 := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "my-user",
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR2), applicationCR2)
				Expect(err).To(BeNil())

				By("Call Perform task again and verify that update works: the Application CR should now have the updated spec from the database.")
				retry, err = task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR2), applicationCR2)
				Expect(err).To(BeNil())
				Expect(newSpecApp2.Spec.SyncPolicy.SyncOptions).To(Equal(applicationCR2.Spec.SyncPolicy.SyncOptions), "PerformTask should have updated the Application CR to be consistent with the new spec(SyncOption) in the database")
				Expect(applicationCR2.Spec.SyncPolicy.SyncOptions.HasOption("CreateNamespace=true")).To(Equal(true))

			})

			DescribeTable("Checks whether the user updated tls-certificate verification maps correctly from database to cluster secret",
				func(tlsVerifyStatus bool) {
					By("Close database connection")
					defer dbQueries.CloseDatabase()
					defer testTeardown()

					_, dummyApplicationSpecString, err := createDummyApplicationData()
					Expect(err).To(BeNil())

					gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
					Expect(gitopsEngineCluster).ToNot(BeNil())
					Expect(err).To(BeNil())

					By("creating a cluster credential with tls-cert-verification set to true")

					clusterCredentials := &db.ClusterCredentials{
						ClustercredentialsCredID:   "test-clustercredentials_cred_id-1",
						Host:                       "test-host",
						KubeConfig:                 "test-kube_config",
						KubeConfig_context:         "test-kube_config_context",
						ServiceAccountBearerToken:  "test-serviceaccount_bearer_token",
						ServiceAccountNs:           "test-serviceaccount_ns",
						AllowInsecureSkipTLSVerify: tlsVerifyStatus,
					}

					managedEnvironment := &db.ManagedEnvironment{
						Managedenvironment_id: "test-managed-env-1",
						ClusterCredentialsID:  clusterCredentials.ClustercredentialsCredID,
						Name:                  "test-my-env101",
					}
					gitopsEngineInstance := &db.GitopsEngineInstance{
						Gitopsengineinstance_id: "test-fake-engine-instance",
						NamespaceName:           workspace.Name,
						NamespaceUID:            string(workspace.UID),
						EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
					}

					err = dbQueries.CreateClusterCredentials(ctx, clusterCredentials)
					Expect(err).To(BeNil())
					err = dbQueries.CreateManagedEnvironment(ctx, managedEnvironment)
					Expect(err).To(BeNil())
					err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
					Expect(err).To(BeNil())

					applicationDB := &db.Application{
						ApplicationID:          "test-my-application-new-1",
						Name:                   name,
						SpecField:              dummyApplicationSpecString,
						EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
						Managed_environment_id: managedEnvironment.Managedenvironment_id,
					}

					By("Create Application in Database")
					err = dbQueries.CreateApplication(ctx, applicationDB)
					Expect(err).To(BeNil())

					By("Creating new operation row in database")
					operationDB := &db.Operation{
						Operation_id:         "test-operation",
						InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
						ResourceID:           applicationDB.ApplicationID,
						ResourceType:         "Application",
						State:                db.OperationState_Waiting,
						OperationOwnerUserID: testClusterUser.ClusterUserID,
					}

					err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
					Expect(err).To(BeNil())

					By("Creating Operation CR")
					operationCR := &managedgitopsv1alpha1.Operation{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
						},
						Spec: managedgitopsv1alpha1.OperationSpec{
							OperationID: operationDB.Operation_id,
						},
					}

					err = task.event.client.Create(ctx, operationCR)
					Expect(err).To(BeNil())

					By("Verifying whether Cluster secret is created and unmarshalling the tls-config byte value")
					secretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: applicationDB.Managed_environment_id})
					clusterSecret := &v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: gitopsEngineInstance.NamespaceName,
						},
						Data: map[string][]byte{},
					}

					retry, err := task.PerformTask(ctx)
					Expect(err).To(BeNil())
					Expect(retry).To(BeFalse())

					err = task.event.client.Get(ctx, client.ObjectKeyFromObject(clusterSecret), clusterSecret)
					Expect(err).To(BeNil())

					getClusterSecretData := clusterSecret.Data
					tlsconfigbyte := getClusterSecretData["config"]
					var tlsunmarshalled ClusterSecretConfigJSON
					err = json.Unmarshal(tlsconfigbyte, &tlsunmarshalled)
					Expect(err).To(BeNil())

					By("The tlsConfig value from cluster-secret should be equal to the tlsVerify value from the database")
					Expect(tlsunmarshalled.TLSClientConfig.Insecure).To(Equal(clusterCredentials.AllowInsecureSkipTLSVerify))

					By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
					err = dbQueries.GetOperationById(ctx, operationDB)
					Expect(err).To(BeNil())
					Expect(operationDB.State).To(Equal(db.OperationState_Completed))

					By("deleting resources and cleaning up db entries created by test.")

					resourcesToBeDeleted := testResources{
						ApplicationID:           applicationDB.ApplicationID,
						Operation_id:            []string{operationDB.Operation_id},
						PrimaryKeyID:            gitopsEngineCluster.PrimaryKeyID,
						Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
						ClusterCredentials_id:   gitopsEngineCluster.ClusterCredentialsID,
					}

					deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)
				},
				Entry("TLS status set TRUE", bool(managedgitopsv1alpha1.TLSVerifyStatusTrue)),
				Entry("TLS status set FALSE", bool(managedgitopsv1alpha1.TLSVerifyStatusFalse)),
			)

		})

		Context("Process SyncOperation", func() {

			var (
				applicationDB          *db.Application
				applicationCR          *appv1.Application
				gitopsEngineInstanceID string
			)

			createOperationDBAndCR := func(resourceID, gitopsEngineInstanceID string) {
				By("creating new operation row of type SyncOperation in the database")
				operationDB := &db.Operation{
					Operation_id:         "test-operation",
					InstanceID:           gitopsEngineInstanceID,
					ResourceID:           resourceID,
					ResourceType:         db.OperationResourceType_SyncOperation,
					State:                db.OperationState_Waiting,
					OperationOwnerUserID: testClusterUser.ClusterUserID,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.OperationOwnerUserID)
				Expect(err).To(BeNil())

				By("creating Operation CR")
				operationCR := &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: operationDB.Operation_id,
					},
				}

				err = task.event.client.Create(ctx, operationCR)
				Expect(err).To(BeNil())
			}

			updateApplicationOperationState := func(applicationCR *appv1.Application) {
				operation := &appv1.Operation{
					Sync: &appv1.SyncOperation{
						Revision: "123",
					},
				}

				applicationCR.Operation = operation
				applicationCR.Status.OperationState = &appv1.OperationState{
					Operation: *operation,
				}

				err = k8sClient.Update(ctx, applicationCR)
				Expect(err).To(BeNil())
			}

			BeforeEach(func() {
				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).To(BeNil())

				_, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).To(BeNil())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).To(BeNil())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					NamespaceName:           workspace.Name,
					NamespaceUID:            string(workspace.UID),
					EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).To(BeNil())

				gitopsEngineInstanceID = gitopsEngineInstance.Gitopsengineinstance_id

				applicationDB = &db.Application{
					ApplicationID:          "test-my-application",
					Name:                   name,
					SpecField:              dummyApplicationSpecString,
					EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
				}

				By("create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).To(BeNil())

				By("create Application CR")
				applicationCR = &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: workspace.Name,
					},
				}
				err = k8sClient.Create(ctx, applicationCR)
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				dbQueries.CloseDatabase()
				testTeardown()
			})

			It("should handle a valid SyncOperation", func() {

				By("create a SyncOperation in the database")
				syncOperation := db.SyncOperation{
					SyncOperationID:     "test-syncoperation",
					ApplicationID:       applicationDB.ApplicationID,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Running,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperationID, gitopsEngineInstanceID)

				By("verify there is no retry for a successful sync")
				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return nil
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).Should(BeNil())
				Expect(retry).To(BeFalse())
			})

			It("should return an error and retry if the sync fails", func() {

				By("create a SyncOperation in the database")
				syncOperation := db.SyncOperation{
					SyncOperationID:     "test-syncoperation",
					ApplicationID:       applicationDB.ApplicationID,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Running,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperationID, gitopsEngineInstanceID)

				By("check if the sync failed error is returned with retry")
				expectedErr := "sync failed due to xyz reason"
				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return fmt.Errorf(expectedErr)
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err.Error()).Should(Equal(expectedErr))
				Expect(retry).To(BeTrue())
			})

			It("return an error and don't retry if the SyncOperation DB row is not found", func() {

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR("uknown", gitopsEngineInstanceID)

				By("check if SyncOperation not found error is handled")
				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return nil
					},
				}
				expectedErr := "no results found for GetSyncOperationById: no rows in result set"

				retry, err := task.PerformTask(ctx)
				Expect(err.Error()).Should(Equal(expectedErr))
				Expect(retry).To(BeFalse())
			})

			It("don't retry if the SyncOperation is neither in Running nor in Terminated state", func() {

				By("create a SyncOperation of unknown state in the database")
				syncOperation := db.SyncOperation{
					SyncOperationID:     "test-syncoperation",
					ApplicationID:       applicationDB.ApplicationID,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        "uknown",
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperationID, gitopsEngineInstanceID)

				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return nil
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).Should(BeNil())
				Expect(retry).To(BeFalse())
			})

			It("don't retry if we can successfully terminate a sync operation", func() {
				By("update the Application operation state so it can be terminated later")
				updateApplicationOperationState(applicationCR)

				By("create a SyncOperation with desired state 'Terminated' in the database")
				syncOperation := db.SyncOperation{
					SyncOperationID:     "test-syncoperation",
					ApplicationID:       applicationDB.ApplicationID,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Terminated,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperationID, gitopsEngineInstanceID)

				By("verify that there is no retry and error for a successful termination")
				task.syncFuncs = &syncFuncs{
					terminateOperation: func(ctx context.Context, s string, n v1.Namespace, cs *utils.CredentialService, c client.Client, d time.Duration, l logr.Logger) error {
						return nil
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).Should(BeNil())
				Expect(retry).To(BeFalse())
			})

			It("should return an error and retry if we can't terminate a sync operation", func() {
				By("update the Application operation state so it can be terminated later")
				updateApplicationOperationState(applicationCR)

				By("create a SyncOperation with desired state 'Terminated' in the database")
				syncOperation := db.SyncOperation{
					SyncOperationID:     "test-syncoperation",
					ApplicationID:       applicationDB.ApplicationID,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Terminated,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperationID, gitopsEngineInstanceID)

				By("check if an error is returned for the failed termination")
				expectedErr := "unable to terminate sync due to xyz reason"
				task.syncFuncs = &syncFuncs{
					terminateOperation: func(ctx context.Context, s string, n v1.Namespace, cs *utils.CredentialService, c client.Client, d time.Duration, l logr.Logger) error {
						return fmt.Errorf(expectedErr)
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err.Error()).Should(Equal(expectedErr))
				Expect(retry).To(BeTrue())
			})

			It("should not terminate if no sync operation is in progress", func() {
				By("create a SyncOperation with desired state 'Terminated' in the database")
				syncOperation := db.SyncOperation{
					SyncOperationID:     "test-syncoperation",
					ApplicationID:       applicationDB.ApplicationID,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Terminated,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).To(BeNil())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperationID, gitopsEngineInstanceID)

				By("verify there is no termination and retry should be false")
				task.syncFuncs = &syncFuncs{
					terminateOperation: func(ctx context.Context, s string, n v1.Namespace, cs *utils.CredentialService, c client.Client, d time.Duration, l logr.Logger) error {
						return fmt.Errorf("unable to terminate sync due to xyz reason")
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).Should(BeNil())
				Expect(retry).To(BeFalse())
			})

		})

		Context("Test if Operation is running for an Application", func() {

			var (
				applicationCR *appv1.Application
			)

			BeforeEach(func() {
				applicationCR = &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: workspace.Name,
					},
				}
			})

			AfterEach(func() {
				err = k8sClient.Delete(ctx, applicationCR)
				if err != nil {
					Expect(apierr.IsNotFound(err)).To(BeTrue())
				}
			})

			It("should return false if the Application doesn't have .status.OperationState set", func() {
				applicationCR.Operation = &appv1.Operation{
					Sync: &appv1.SyncOperation{
						Revision: "123",
					},
				}

				err = k8sClient.Create(ctx, applicationCR)
				Expect(err).To(BeNil())

				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).To(BeNil())
				Expect(isRunning).To(BeFalse())

			})

			It("should return false if the Application doesn't have .spec.Operation field set", func() {
				applicationCR.Status.OperationState = &appv1.OperationState{
					Operation: appv1.Operation{
						Sync: &appv1.SyncOperation{
							Revision: "123",
						},
					},
				}

				err = k8sClient.Create(ctx, applicationCR)
				Expect(err).To(BeNil())

				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).To(BeNil())
				Expect(isRunning).To(BeFalse())

			})

			It("should return true if the Application has both .spec.Operation and .status.OperationState set", func() {
				operation := appv1.Operation{
					Sync: &appv1.SyncOperation{Revision: "123"},
				}
				applicationCR.Operation = &operation
				applicationCR.Status.OperationState = &appv1.OperationState{
					Operation: operation,
				}

				err = k8sClient.Create(ctx, applicationCR)
				Expect(err).To(BeNil())

				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).To(BeNil())
				Expect(isRunning).To(BeTrue())

			})

			It("should return a missing error if the Application is not found", func() {
				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).ToNot(BeNil())
				Expect(apierr.IsNotFound(err)).Should(BeTrue())
				Expect(isRunning).To(BeFalse())
			})
		})
	})
})

func testTeardown() {
	err := db.SetupForTestingDBGinkgo()
	Expect(err).To(BeNil())
}

func newRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// Used to list down resources for deletion which are created while running tests.
type testResources struct {
	Operation_id                  []string
	PrimaryKeyID                  string
	Gitopsengineinstance_id       string
	ClusterCredentials_id         string
	ApplicationID                 string
	Managedenvironment_id         string
	kubernetesToDBResourceMapping db.KubernetesToDBResourceMapping
	SyncOperationID               string
}

// Delete resources from table
func deleteTestResources(ctx context.Context, dbQueries db.AllDatabaseQueries, resourcesToBeDeleted testResources) {
	var rowsAffected int
	var err error

	// Delete SyncOperation
	if resourcesToBeDeleted.SyncOperationID != "" {
		rowsAffected, err = dbQueries.DeleteSyncOperationById(ctx, resourcesToBeDeleted.SyncOperationID)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete Application
	if resourcesToBeDeleted.ApplicationID != "" {
		rowsAffected, err = dbQueries.DeleteApplicationById(ctx, resourcesToBeDeleted.ApplicationID)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete kubernetesToDBResourceMapping
	if resourcesToBeDeleted.kubernetesToDBResourceMapping.KubernetesResourceUID != "" {
		rowsAffected, err = dbQueries.DeleteKubernetesResourceToDBResourceMapping(ctx, &resourcesToBeDeleted.kubernetesToDBResourceMapping)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete ManagedEnvironment
	if resourcesToBeDeleted.Managedenvironment_id != "" {
		rowsAffected, err = dbQueries.DeleteManagedEnvironmentById(ctx, resourcesToBeDeleted.Managedenvironment_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete Operation
	for _, operationToDelete := range resourcesToBeDeleted.Operation_id {
		rowsAffected, err = dbQueries.DeleteOperationById(ctx, operationToDelete)
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
	if resourcesToBeDeleted.PrimaryKeyID != "" {
		rowsAffected, err = dbQueries.DeleteGitopsEngineClusterById(ctx, resourcesToBeDeleted.PrimaryKeyID)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete ClusterCredentials
	if resourcesToBeDeleted.ClusterCredentials_id != "" {
		rowsAffected, err = dbQueries.DeleteClusterCredentialsById(ctx, resourcesToBeDeleted.ClusterCredentials_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).To(Equal(1))
	}

}

func createDummyApplicationData() (appv1.Application, string, error) {
	return createCustomizedDummyApplicationData("guestbook")
}

func createCustomizedDummyApplicationData(repoPath string) (appv1.Application, string, error) {
	// Create dummy Application Spec to be saved in DB
	dummyApplicationSpec := appv1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operation",
			Namespace: "my-user",
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
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{},
			},
		},
	}

	dummyApplicationSpecBytes, err := yaml.Marshal(dummyApplicationSpec)

	if err != nil {
		return appv1.Application{}, "", err
	}

	return dummyApplicationSpec, string(dummyApplicationSpecBytes), nil
}

func createApplicationWithSyncOption(syncOptionParam string) (appv1.Application, string, error) {

	syncOptions := appv1.SyncOptions(nil)

	if syncOptionParam != "" {
		syncOptions = appv1.SyncOptions{syncOptionParam}
	}

	// Create dummy Application Spec to be saved in DB
	dummyApplicationSpec := appv1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operation",
			Namespace: "my-user",
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
			SyncPolicy: &appv1.SyncPolicy{
				SyncOptions: syncOptions,
			},
		},
	}

	dummyApplicationSpecBytes, err := yaml.Marshal(dummyApplicationSpec)

	if err != nil {
		return appv1.Application{}, "", err
	}

	return dummyApplicationSpec, string(dummyApplicationSpecBytes), nil
}
