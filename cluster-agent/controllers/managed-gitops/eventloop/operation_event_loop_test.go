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
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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

	argocdoperatorv1alph1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
)

var _ = Describe("Operation Controller", func() {
	const (
		name      = "operation"
		dbID      = "databaseID"
		namespace = "argocd"
	)

	Context("Managed Environment Unit Tests", func() {

		var scheme *runtime.Scheme
		var ctx context.Context
		var dbQueries db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var argoCDNamespace *corev1.Namespace
		var kubesystemNamespace *corev1.Namespace
		var logger logr.Logger
		var workspace *corev1.Namespace

		var gitopsEngineInstance *db.GitopsEngineInstance

		var opConfigVal operationConfig

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			scheme, argoCDNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, kubesystemNamespace).Build()

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
			gitopsEngineInstance = &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance",
				Namespace_name:          namespace,
				Namespace_uid:           string(workspace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			opConfigVal = operationConfig{
				dbQueries:         dbQueries,
				argoCDNamespace:   *argoCDNamespace,
				eventClient:       k8sClient,
				credentialService: nil,
				log:               logger,
				syncFuncs:         nil,
			}

		})

		It("generateExpectedClusterSecret should generate a valid cluster secret", func() {

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test",
				Host:                        "https://my-cluster-url.com",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err := dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my env",
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			By("Create Application in Database")
			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			secret, shouldDelete, err := generateExpectedClusterSecret(ctx, *applicationDB, opConfigVal)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldDelete).To(BeFalse())

			Expect(string(secret.Data["name"])).To(Equal(argosharedutil.GenerateArgoCDClusterSecretName(managedEnvironment)))
			Expect(string(secret.Data["server"])).To(ContainSubstring(ManagedEnvironmentQueryParameter))

			configData := string(secret.Data["config"])

			Expect(len(configData)).To(BeNumerically(">", 2))

		})

		It("generateExpectedClusterSecret should reject an invalid URL containing query parameters", func() {

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test",
				Host:                        "https://my-cluster-url.com?queryParameter=hello",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err := dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my env",
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			By("create Application in Database")
			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("calling function to test")
			_, shouldDelete, err := generateExpectedClusterSecret(ctx, *applicationDB, opConfigVal)
			Expect(err).To(HaveOccurred(), "should return an error due to unsupported query parameter characters in the URL")
			Expect(err.Error()).To(ContainSubstring("the Kubernetes API URL contained unsupported characters"))
			Expect(shouldDelete).To(BeFalse())
		})

		It("generateExpectedClusterSecret should return shouldDelete of true if the ManagedEnvironment DB entry doesn't exist", func() {

			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  "does-not-exist",
			}

			By("calling function to test")
			_, shouldDelete, err := generateExpectedClusterSecret(ctx, *applicationDB, opConfigVal)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldDelete).To(BeTrue())

		})

		It("EnsureManagedEnvironment should create a Secret for ManagedEnvironment, if the Secret doesn't exist", func() {

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test",
				Host:                        "https://my-cluster-url.com",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err := dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my env",
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			By("calling function to test")
			err = ensureManagedEnvironmentExists(ctx, *applicationDB, opConfigVal)
			Expect(err).ToNot(HaveOccurred())

			secretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: applicationDB.Managed_environment_id})
			Expect(secretName).ToNot(BeEmpty())

			managedEnvironmentSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: argoCDNamespace.Name,
					Labels:    map[string]string{},
				},
				Data: map[string][]byte{},
			}

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvironmentSecret), &managedEnvironmentSecret)
			Expect(err).ToNot(HaveOccurred())

		})

		It("EnsureManagedEnvironment should update an existing Secret for ManagedEnvironment, if the Secret contains a different value", func() {

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test",
				Host:                        "https://my-cluster-url.com",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err := dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my env",
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).ToNot(HaveOccurred())

			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, applicationDB)
			Expect(err).ToNot(HaveOccurred())

			secretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: applicationDB.Managed_environment_id})
			Expect(secretName).ToNot(BeEmpty())

			existingSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: argoCDNamespace.Name,
					Labels:    map[string]string{},
				},
				Data: map[string][]byte{
					"name":   ([]byte)(name),
					"server": ([]byte)(clusterCredentials.Host),
					"config": ([]byte)(string("{}")),
				},
			}

			err = k8sClient.Create(ctx, &existingSecret)
			Expect(err).ToNot(HaveOccurred())

			By("calling function to test")
			err = ensureManagedEnvironmentExists(ctx, *applicationDB, opConfigVal)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironmentSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: argoCDNamespace.Name,
					Labels:    map[string]string{},
				},
				Data: map[string][]byte{},
			}

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvironmentSecret), &managedEnvironmentSecret)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(managedEnvironmentSecret.Data["name"])).To(Equal(argosharedutil.GenerateArgoCDClusterSecretName(managedEnvironment)))
			Expect(string(managedEnvironmentSecret.Data["server"])).To(ContainSubstring(ManagedEnvironmentQueryParameter))

			configData := string(managedEnvironmentSecret.Data["config"])
			Expect(len(configData)).To(BeNumerically(">", 2))

		})

		It("EnsureManagedEnvironment should delete Secret if ManagedEnvironment doesn't exist", func() {

			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    name,
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  "does-not-exist",
			}

			secretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: applicationDB.Managed_environment_id})
			Expect(secretName).ToNot(BeEmpty())

			managedEnvironmentSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: argoCDNamespace.Name,
					Labels:    map[string]string{},
				},
				Data: map[string][]byte{},
			}
			err := k8sClient.Create(ctx, &managedEnvironmentSecret)
			Expect(err).ToNot(HaveOccurred())

			err = ensureManagedEnvironmentExists(ctx, *applicationDB, opConfigVal)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvironmentSecret), &managedEnvironmentSecret)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

		})
	})

	Context("Operation Controller Test", func() {

		var ctx context.Context
		var dbQueries db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var task processOperationEventTask
		var logger logr.Logger
		var kubesystemNamespace *corev1.Namespace
		var workspace *corev1.Namespace
		var scheme *runtime.Scheme
		var testClusterUser *db.ClusterUser
		var err error

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)
			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			scheme, _, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			err = argocdoperatorv1alph1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}
			defaultProject := &appv1.AppProject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: namespace,
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, kubesystemNamespace, defaultProject).Build()

			task = processOperationEventTask{
				log: logger,
				event: operationEventLoopEvent{
					request: newRequest(namespace, name),
					client:  k8sClient,
				},
			}

			namespaceToCreate := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
					UID:  workspace.UID,
				},
			}
			err = task.event.client.Create(ctx, namespaceToCreate)
			Expect(err).ToNot(HaveOccurred())

		})
		It("Ensure that calling perform task on an operation CR for Application that doesn't exist, it doesn't return an error, and retry is false", func() {
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(retry).To(BeFalse())

		})

		It("ensures that if the operation row doesn't exist, an error is not returned, and retry is false", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())

			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			By("Operation row insertion")
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
			Expect(err).ToNot(HaveOccurred())

			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(retry).To(BeFalse())

		})

		It("ensures that if the operation has a resource-type of GitOpsEngineInstance then the function processOperation_GitOpsEngineInstance() picks it successfully", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			newArgoCDNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-new-argocd-namespace",
					UID:  "test-new-argocd-namespace-uuid",
				},
			}
			err = k8sClient.Create(context.Background(), newArgoCDNamespace)
			Expect(err).ToNot(HaveOccurred())

			task.event.request.Namespace = newArgoCDNamespace.Name

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id-1",
				Namespace_name:          newArgoCDNamespace.Name,
				Namespace_uid:           string(newArgoCDNamespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbQueries.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			operationDB := &db.Operation{
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_type:           db.OperationResourceType_GitOpsEngineInstance,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			By("Operation CR creation")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: gitopsEngineInstance.Namespace_name,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = task.event.client.Create(ctx, operationCR)
			Expect(err).ToNot(HaveOccurred())

			// Wait for cluster agent to create the ArgoCD operand. Since Argo CD is not actually running,
			// we simulate Argo CD creating the 'default' AppProject
			go func() {
				defer GinkgoRecover() // Allow Ginkgo to catch the Expects

			outer_for:
				for {

					var argoCDList argocdoperatorv1alph1.ArgoCDList

					err := k8sClient.List(context.Background(), &argoCDList)
					Expect(err).ToNot(HaveOccurred())

					for _, argoCDItem := range argoCDList.Items {

						By("Simulating Argo CD creating a default AppProject, as soon as the ArgoCD CR exists")
						appProject := appv1.AppProject{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "default",
								Namespace: argoCDItem.Namespace,
							},
							Spec: appv1.AppProjectSpec{},
						}

						err := k8sClient.Create(context.Background(), &appProject)
						Expect(err).ToNot(HaveOccurred())

						break outer_for

					}

					time.Sleep(100 * time.Millisecond)

				}
			}()

			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(retry).To(BeFalse())

			By("check if the Operation state is updated to InProgress")
			err = dbQueries.GetOperationById(ctx, operationDB)
			Expect(err).ToNot(HaveOccurred())
			Expect(operationDB.State).To(Equal(db.OperationState_In_Progress))
		})

		It("ensures that if the kube-system namespace does not having a matching namespace uid, an error is not returned, but retry is true", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			By("'kube-system' namespace has a UID that is not found in a corresponding row in GitOpsEngineCluster database")
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
			Expect(err).ToNot(HaveOccurred())
			Expect(kubesystemNamespace.UID).ToNot(Equal(gitopsEngineInstance.Namespace_uid))

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			By("Operation CR exists")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: gitopsEngineInstance.Namespace_name,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}

			err = task.event.client.Create(ctx, operationCR)
			Expect(err).ToNot(HaveOccurred())

			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(retry).To(BeTrue())

		})

		It("Ensures that if the GitopsEngineInstance's namespace_name field doesn't exist, an error is returned, and retry is false as the operation namespace check overrides", func() {
			By("Close database connection")
			defer dbQueries.CloseDatabase()
			defer testTeardown()

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())

			By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
			gitopsEngineInstance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance",
				Namespace_name:          "doesn't-exist",
				Namespace_uid:           string("doesnt-exist-uid"),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).ToNot(HaveOccurred())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(HaveOccurred())
			Expect(retry).To(BeFalse())

			kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  string(kubesystemNamespace.UID),
				DBRelationType:         "GitopsEngineCluster",
				DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
			}

			By("deleting resources and cleaning up db entries created by test.")
			resourcesToBeDeleted := testResources{
				Operation_id:                  []string{operationDB.Operation_id},
				Gitopsenginecluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
				ClusterCredentials_id:         gitopsEngineCluster.Clustercredentials_id,
				kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
			}

			deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

		})

		Context("Process Application Operation Test", func() {

			It("Verify that When an Operation row points to an Application row that doesn't exist, any Argo Application CR and AppProject CR that relates to that Application row should be removed.", func() {
				By("Close database connection")
				err = db.SetupForTestingDBGinkgo()
				Expect(err).ToNot(HaveOccurred())
				defer dbQueries.CloseDatabase()

				appProject := &appv1.AppProject{
					ObjectMeta: metav1.ObjectMeta{
						Name:      appProjectPrefix + testClusterUser.Clusteruser_id,
						Namespace: namespace,
					},
					Spec: appv1.AppProjectSpec{
						SourceRepos: []string{"test-url"},
					},
				}

				err = task.event.client.Create(ctx, appProject)
				Expect(err).ToNot(HaveOccurred())

				applicationCR := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels: map[string]string{
							dbID: "doesnt-exist",
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
					},
				}

				err = task.event.client.Create(ctx, applicationCR)
				Expect(err).ToNot(HaveOccurred())

				// The Argo CD Applications used by GitOps Service use finalizers, so the Applicaitonwill not be deleted until the finalizer is removed.
				// Normally it us Argo CD's job to do this, but since this is a unit test, there is no Argo CD. Instead we wait for the deletiontimestamp
				// to be set (by the delete call of PerformTask, and then just remove the finalize and update, simulating what Argo CD does)
				go func() {
					err = wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
						if applicationCR.DeletionTimestamp != nil {
							err = k8sClient.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
							Expect(err).ToNot(HaveOccurred())

							applicationCR.Finalizers = nil

							err = k8sClient.Update(ctx, applicationCR)
							Expect(err).ToNot(HaveOccurred())

							err = task.event.client.Delete(ctx, applicationCR)
							return true, nil
						}
						return false, nil
					})
				}()

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}

				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "doesnt-exist",
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				By("Verifying whether Application CR is deleted")
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, applicationCR)
				Expect(apierr.IsNotFound(err)).To(BeTrue())

				By("Verifying whether AppProject CR is deleted")
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, appProject)
				Expect(apierr.IsNotFound(err)).To(BeTrue())

				kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
					KubernetesResourceType: "Namespace",
					KubernetesResourceUID:  string(kubesystemNamespace.UID),
					DBRelationType:         "GitopsEngineCluster",
					DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
				}

				By("deleting resources and cleaning up db entries created by test.")
				resourcesToBeDeleted := testResources{
					Operation_id:                  []string{operationDB.Operation_id},
					Gitopsenginecluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
					Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:         gitopsEngineCluster.Clustercredentials_id,
					kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify that when an Operation row points to an Application row that exists in the database, but doesn't exist in the Argo CD namespace, it should be created.", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				By("Create Application in Database")
				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				Expect(retry).To(BeFalse())
				By("Verifying whether Application CR is created")
				applicationCR := appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: name}, &applicationCR)
				Expect(err).ToNot(HaveOccurred())
				Expect(dummyApplicationSpec.Spec).To(Equal(applicationCR.Spec))

				kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
					KubernetesResourceType: "Namespace",
					KubernetesResourceUID:  string(kubesystemNamespace.UID),
					DBRelationType:         "GitopsEngineCluster",
					DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
				}

				By("deleting resources and cleaning up db entries created by test.")
				resourcesToBeDeleted := testResources{
					Application_id:                applicationDB.Application_id,
					Operation_id:                  []string{operationDB.Operation_id},
					Gitopsenginecluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
					Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:         gitopsEngineCluster.Clustercredentials_id,
					kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify that Application CR should be updated to be consistent with the Application row", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				_, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				By("Create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				By("creating a new spec and putting it into the Application in the database, so that operation wlil update it")
				newSpecApp, newSpecString, err := createCustomizedDummyApplicationData("different-path")
				Expect(err).ToNot(HaveOccurred())

				By("Update Application in Database")
				applicationUpdate := &db.Application{
					Application_id:          "test-my-application",
					Name:                    applicationDB.Name,
					Spec_field:              newSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
					SeqID:                   101,
					Created_on:              applicationDB.Created_on,
				}

				err = dbQueries.UpdateApplication(ctx, applicationUpdate)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDB2 := &db.Operation{
					Operation_id:            "test-operation-2",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB2, operationDB2.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				By("Verifying whether Application CR is created")
				applicationCR := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).ToNot(HaveOccurred())

				By("Call Perform task again and verify that update works: the Application CR should now have the updated spec from the database.")
				retry, err = task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())
				Expect(newSpecApp.Spec).To(Equal(applicationCR.Spec), "PerformTask should have updated the Application CR to be consistent with the new spec in the database")

				kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
					KubernetesResourceType: "Namespace",
					KubernetesResourceUID:  string(kubesystemNamespace.UID),
					DBRelationType:         "GitopsEngineCluster",
					DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
				}

				By("deleting resources and cleaning up db entries created by test.")

				resourcesToBeDeleted := testResources{
					Application_id:                applicationDB.Application_id,
					Operation_id:                  []string{operationDB.Operation_id, operationDB2.Operation_id},
					Gitopsenginecluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
					Gitopsengineinstance_id:       gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:         gitopsEngineCluster.Clustercredentials_id,
					kubernetesToDBResourceMapping: kubernetesToDBResourceMapping,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify that SyncOption is picked up by Perform Task to be in sync for CreateNamespace=true", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				dummyApplication, dummyApplicationSpecString, err := createApplicationWithSyncOption("CreateNamespace=true")
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				By("Create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("Verifying whether Application CR is created")
				applicationCR := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).ToNot(HaveOccurred())

				By("Verify that the SyncOption in the Application has Option - CreateNamespace=true")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))
				Expect(dummyApplication.Spec.SyncPolicy.SyncOptions).To(Equal(applicationCR.Spec.SyncPolicy.SyncOptions))
				Expect(applicationCR.Spec.SyncPolicy.SyncOptions.HasOption("CreateNamespace=true")).To(BeTrue())

				//############################################################################

				By("Update the SyncOption to not have option CreateNamespace=true")
				newSpecApp, newSpecString, err := createApplicationWithSyncOption("")
				Expect(err).ToNot(HaveOccurred())

				By("Update Application in Database")
				applicationUpdate := &db.Application{
					Application_id:          "test-my-application",
					Name:                    applicationDB.Name,
					Spec_field:              newSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
					SeqID:                   101,
					Created_on:              applicationDB.Created_on,
				}

				err = dbQueries.UpdateApplication(ctx, applicationUpdate)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDB2 := &db.Operation{
					Operation_id:            "test-operation-2",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB2, operationDB2.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				By("Verifying whether Application CR is created")
				applicationCR = &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).ToNot(HaveOccurred())

				By("Call Perform task again and verify that update works: the Application CR should now have the updated spec from the database.")
				retry, err = task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
				Expect(err).ToNot(HaveOccurred())
				Expect(newSpecApp.Spec.SyncPolicy.SyncOptions).To(Equal(applicationCR.Spec.SyncPolicy.SyncOptions), "PerformTask should have updated the Application CR to be consistent with the new spec(SyncOption) in the database")
				Expect(applicationCR.Spec.SyncPolicy.SyncOptions.HasOption("CreateNamespace=true")).To(BeFalse())

				By("Verify that the SyncOption in the Application has Option - CreateNamespace=true and the operation is completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				//############################################################################

				By("Update the SyncOption to have option CreateNamespace=true")
				newSpecApp2, newSpecString2, err := createApplicationWithSyncOption("CreateNamespace=true")
				Expect(err).ToNot(HaveOccurred())

				By("Update Application in Database")
				applicationUpdate2 := &db.Application{
					Application_id:          "test-my-application",
					Name:                    applicationDB.Name,
					Spec_field:              newSpecString2,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
					SeqID:                   101,
					Created_on:              applicationDB.Created_on,
				}

				err = dbQueries.UpdateApplication(ctx, applicationUpdate2)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDBUpdate2 := &db.Operation{
					Operation_id:            "test-operation-3",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDBUpdate2, operationDBUpdate2.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				By("Verifying whether Application CR is created")
				applicationCR2 := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR2), applicationCR2)
				Expect(err).ToNot(HaveOccurred())

				By("Call Perform task again and verify that update works: the Application CR should now have the updated spec from the database.")
				retry, err = task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(applicationCR2), applicationCR2)
				Expect(err).ToNot(HaveOccurred())
				Expect(newSpecApp2.Spec.SyncPolicy.SyncOptions).To(Equal(applicationCR2.Spec.SyncPolicy.SyncOptions), "PerformTask should have updated the Application CR to be consistent with the new spec(SyncOption) in the database")
				Expect(applicationCR2.Spec.SyncPolicy.SyncOptions.HasOption("CreateNamespace=true")).To(BeTrue())

				By("Verify whether AppProject has been created")
				appProject := &appv1.AppProject{
					ObjectMeta: metav1.ObjectMeta{
						Name:      appProjectPrefix + operationDB.Operation_owner_user_id,
						Namespace: namespace,
					},
				}

				var appProjectRepositories []db.AppProjectRepository
				err = dbQueries.UnsafeListAllAppProjectRepositories(ctx, &appProjectRepositories)
				Expect(err).ToNot(HaveOccurred())

				var repoURLs []string
				for _, v := range appProjectRepositories {
					if v.Clusteruser_id == operationDB.Operation_owner_user_id {
						repoURLs = append(repoURLs, v.RepoURL)
					}
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: appProject.Namespace, Name: appProject.Name}, appProject)
				Expect(err).ToNot(HaveOccurred())
				Expect(appProject).ToNot(BeNil())
				Expect(appProject.Name).To(Equal(appProjectPrefix + operationDB.Operation_owner_user_id))
				Expect(appProject.Namespace).To(Equal(namespace))
				Expect(appProject.Spec.SourceRepos).To(Equal(repoURLs))
				Expect(appProject.Spec.Destinations).To(Equal([]appv1.ApplicationDestination{{
					Namespace: "*",
					Name:      "in-cluster",
				}}))

				By("Verify whether Project field of Application CR is pointing to AppProject")
				Expect(applicationCR2.Spec.Project).To(Equal(appProject.Name))

			})

			It("Verify whether the existing app project is updated when the generated app project differs from the existing app project.", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				_, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				By("Create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				By("Create a RepositoryCredentials DB entry.")

				repoCredentials := db.RepositoryCredentials{
					RepositoryCredentialsID: "test-cred-id" + string(uuid.NewUUID()),
					UserID:                  testClusterUser.Clusteruser_id,
					PrivateURL:              "https://test-private-url",
					AuthUsername:            "test-auth-username",
					AuthPassword:            "test-auth-password",
					AuthSSHKey:              "test-auth-ssh-key",
					SecretObj:               "test-secret-obj",
					EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
				}
				err = dbQueries.CreateRepositoryCredentials(ctx, &repoCredentials)
				Expect(err).ToNot(HaveOccurred())

				dbAppProjectRepo := &db.AppProjectRepository{
					AppprojectRepositoryID:  "test-appProject-repo-id",
					Clusteruser_id:          testClusterUser.Clusteruser_id,
					RepositorycredentialsID: repoCredentials.RepositoryCredentialsID,
					RepoURL:                 "test-url",
				}

				err = dbQueries.CreateAppProjectRepository(ctx, dbAppProjectRepo)
				Expect(err).ToNot(HaveOccurred())

				By("Creating existingAppProject to verify the consistency of AppProject")
				existingAppProject := &appv1.AppProject{
					ObjectMeta: metav1.ObjectMeta{
						Name: appProjectPrefix + operationDB.Operation_owner_user_id,
						Annotations: map[string]string{
							"username": "test",
						},
						Namespace: namespace,
					},
					Spec: appv1.AppProjectSpec{
						SourceRepos: []string{"test-url"},
						Destinations: []appv1.ApplicationDestination{
							{
								Namespace: "test",
								Server:    argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: applicationDB.Managed_environment_id}),
							},
						},
					},
				}

				err = task.event.client.Create(ctx, existingAppProject)
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("Verify whether the generated AppProject is equal to the existing AppProject.")
				appProject := &appv1.AppProject{
					ObjectMeta: metav1.ObjectMeta{
						Name:      appProjectPrefix + operationDB.Operation_owner_user_id,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: appProject.Namespace, Name: appProject.Name}, appProject)
				Expect(err).ToNot(HaveOccurred())
				Expect(appProject).ToNot(BeNil())
				Expect(existingAppProject).ToNot(Equal(appProject))

				By("checking whether existingAppProject is updated")
				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: existingAppProject.Namespace, Name: existingAppProject.Name}, existingAppProject)
				Expect(err).ToNot(HaveOccurred())
				Expect(appProject).ToNot(BeNil())
				Expect(existingAppProject).ToNot(BeNil())
				Expect(existingAppProject).To(Equal(appProject))

				By("deleting resources and cleaning up db entries created by test.")

				resourcesToBeDeleted := testResources{
					Application_id:          applicationDB.Application_id,
					Operation_id:            []string{operationDB.Operation_id},
					Gitopsenginecluster_id:  gitopsEngineCluster.Gitopsenginecluster_id,
					Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:   gitopsEngineCluster.Clustercredentials_id,
					RepositoryCredentialsID: repoCredentials.RepositoryCredentialsID,
					AppProjectRepositoryID:  dbAppProjectRepo.AppprojectRepositoryID,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("verifies that managed environment/cluster credential database values are correctly applied in the Argo CD cluster secret", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				By("creating a new managed env")
				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				By("creating new cluster credentials for that managed env, containing non-default namespaces/clusterresource fields")
				clusterCredentials := db.ClusterCredentials{
					Clustercredentials_cred_id:  "test-cluster-credentials",
					Host:                        "https://fake-host-url.com",
					Kube_config:                 "",
					Kube_config_context:         "",
					Serviceaccount_bearer_token: string(uuid.NewUUID()),
					Serviceaccount_ns:           "",
					AllowInsecureSkipTLSVerify:  true,
					Namespaces:                  "a,b,c",
					ClusterResources:            true,
				}
				err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
				Expect(err).ToNot(HaveOccurred())

				managedEnvironment.Clustercredentials_id = clusterCredentials.Clustercredentials_cred_id
				err = dbQueries.UpdateManagedEnvironment(ctx, managedEnvironment)
				Expect(err).ToNot(HaveOccurred())

				dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				By("creating an Application row, pointing to the ManagedEnvironment")
				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating Operation row in database, pointing to the Application")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				Expect(retry).To(BeFalse())
				By("verifying that the Application CR is created")
				applicationCR := appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: namespace,
					},
				}

				By("verifying the argo cd cluster secret was created, and has expected values")
				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: name}, &applicationCR)
				Expect(err).ToNot(HaveOccurred())
				Expect(dummyApplicationSpec.Spec).To(Equal(applicationCR.Spec))

				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      argosharedutil.GenerateArgoCDClusterSecretName(*managedEnvironment),
						Namespace: namespace,
					},
				}
				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
				Expect(err).ToNot(HaveOccurred())

				Expect(string(secret.Data["clusterResources"])).To(Equal("true"),
					"cluster resources should match the value from clustercredentials db row")
				Expect(string(secret.Data["namespaces"])).To(Equal(clusterCredentials.Namespaces),
					"should match the value from cluster credentials db row")

				secretJSON := argosharedutil.ClusterSecretConfigJSON{}
				err = json.Unmarshal(secret.Data["config"], &secretJSON)
				Expect(err).ToNot(HaveOccurred())

				Expect(secretJSON.BearerToken).To(Equal(clusterCredentials.Serviceaccount_bearer_token))
				Expect(secretJSON.TLSClientConfig.Insecure).To(Equal(clusterCredentials.AllowInsecureSkipTLSVerify))

				By("creating new cluster credentials for that managed env, containing different namespaces/clusterresource fields")
				clusterCredentials = db.ClusterCredentials{
					Clustercredentials_cred_id:  "test-cluster-credentials-2",
					Host:                        "https://fake-host-url.com",
					Kube_config:                 "",
					Kube_config_context:         "",
					Serviceaccount_bearer_token: string(uuid.NewUUID()),
					Serviceaccount_ns:           "",
					AllowInsecureSkipTLSVerify:  false,
					Namespaces:                  "",
					ClusterResources:            false,
				}
				err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
				Expect(err).ToNot(HaveOccurred())

				managedEnvironment.Clustercredentials_id = clusterCredentials.Clustercredentials_cred_id
				err = dbQueries.UpdateManagedEnvironment(ctx, managedEnvironment)
				Expect(err).ToNot(HaveOccurred())

				By("reusing the operation we created previously: re-using operations isn't normally supported, but it saves us some extra lines in the unit test")
				operationDB.State = db.OperationState_In_Progress
				err = dbQueries.UpdateOperation(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())

				By("calling PerformTask to process the operation, pointing to the updated managed environment")
				retry, err = task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("verifying the argo cd cluster secret was updated, and has expected values")
				secret = corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      argosharedutil.GenerateArgoCDClusterSecretName(*managedEnvironment),
						Namespace: namespace,
					},
				}
				err = task.event.client.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)
				Expect(err).ToNot(HaveOccurred())

				Expect(string(secret.Data["clusterResources"])).To(Equal(""),
					"cluster resources should match the value from clustercredentials db row")
				Expect(string(secret.Data["namespaces"])).To(Equal(clusterCredentials.Namespaces),
					"should match the value from cluster credentials db row")

				secretJSON = argosharedutil.ClusterSecretConfigJSON{}
				err = json.Unmarshal(secret.Data["config"], &secretJSON)
				Expect(err).ToNot(HaveOccurred())

				Expect(secretJSON.TLSClientConfig.Insecure).To(Equal(clusterCredentials.AllowInsecureSkipTLSVerify))

			})

			DescribeTable("Checks whether the user updated tls-certificate verification maps correctly from database to cluster secret",
				func(tlsVerifyStatus bool) {
					By("Close database connection")
					defer dbQueries.CloseDatabase()
					defer testTeardown()

					_, dummyApplicationSpecString, err := createDummyApplicationData()
					Expect(err).ToNot(HaveOccurred())

					gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
					Expect(gitopsEngineCluster).ToNot(BeNil())
					Expect(err).ToNot(HaveOccurred())

					By("creating a cluster credential with tls-cert-verification set to true")

					clusterCredentials := &db.ClusterCredentials{
						Clustercredentials_cred_id:  "test-clustercredentials_cred_id-1",
						Host:                        "test-host",
						Kube_config:                 "test-kube_config",
						Kube_config_context:         "test-kube_config_context",
						Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
						Serviceaccount_ns:           "test-serviceaccount_ns",
						AllowInsecureSkipTLSVerify:  tlsVerifyStatus,
					}

					managedEnvironment := &db.ManagedEnvironment{
						Managedenvironment_id: "test-managed-env-1",
						Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
						Name:                  "test-my-env101",
					}
					gitopsEngineInstance := &db.GitopsEngineInstance{
						Gitopsengineinstance_id: "test-fake-engine-instance",
						Namespace_name:          namespace,
						Namespace_uid:           string(workspace.UID),
						EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
					}

					err = dbQueries.CreateClusterCredentials(ctx, clusterCredentials)
					Expect(err).ToNot(HaveOccurred())
					err = dbQueries.CreateManagedEnvironment(ctx, managedEnvironment)
					Expect(err).ToNot(HaveOccurred())
					err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
					Expect(err).ToNot(HaveOccurred())

					applicationDB := &db.Application{
						Application_id:          "test-my-application-new-1",
						Name:                    name,
						Spec_field:              dummyApplicationSpecString,
						Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
						Managed_environment_id:  managedEnvironment.Managedenvironment_id,
					}

					By("Create Application in Database")
					err = dbQueries.CreateApplication(ctx, applicationDB)
					Expect(err).ToNot(HaveOccurred())

					By("Creating new operation row in database")
					operationDB := &db.Operation{
						Operation_id:            "test-operation",
						Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
						Resource_id:             applicationDB.Application_id,
						Resource_type:           "Application",
						State:                   db.OperationState_Waiting,
						Operation_owner_user_id: testClusterUser.Clusteruser_id,
					}

					err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
					Expect(err).ToNot(HaveOccurred())

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
					Expect(err).ToNot(HaveOccurred())

					By("Verifying whether Cluster secret is created and unmarshalling the tls-config byte value")
					secretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: applicationDB.Managed_environment_id})
					clusterSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: gitopsEngineInstance.Namespace_name,
						},
						Data: map[string][]byte{},
					}

					retry, err := task.PerformTask(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(retry).To(BeFalse())

					err = task.event.client.Get(ctx, client.ObjectKeyFromObject(clusterSecret), clusterSecret)
					Expect(err).ToNot(HaveOccurred())

					getClusterSecretData := clusterSecret.Data
					tlsconfigbyte := getClusterSecretData["config"]
					var tlsunmarshalled argosharedutil.ClusterSecretConfigJSON
					err = json.Unmarshal(tlsconfigbyte, &tlsunmarshalled)
					Expect(err).ToNot(HaveOccurred())

					By("The tlsConfig value from cluster-secret should be equal to the tlsVerify value from the database")
					Expect(tlsunmarshalled.TLSClientConfig.Insecure).To(Equal(clusterCredentials.AllowInsecureSkipTLSVerify))

					By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
					err = dbQueries.GetOperationById(ctx, operationDB)
					Expect(err).ToNot(HaveOccurred())
					Expect(operationDB.State).To(Equal(db.OperationState_Completed))

					By("deleting resources and cleaning up db entries created by test.")

					resourcesToBeDeleted := testResources{
						Application_id:          applicationDB.Application_id,
						Operation_id:            []string{operationDB.Operation_id},
						Gitopsenginecluster_id:  gitopsEngineCluster.Gitopsenginecluster_id,
						Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
						ClusterCredentials_id:   gitopsEngineCluster.Clustercredentials_id,
					}

					deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)
				},
				Entry("TLS status set TRUE", bool(managedgitopsv1alpha1.TLSVerifyStatusTrue)),
				Entry("TLS status set FALSE", bool(managedgitopsv1alpha1.TLSVerifyStatusFalse)),
			)

			It("Verify whether AppProject has been created or not when processing operation application", func() {
				By("Close database connection")
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				_, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				By("Create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating new operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				By("Create a RepositoryCredentials DB entry.")

				repoCredentials := db.RepositoryCredentials{
					RepositoryCredentialsID: "test-cred-id" + string(uuid.NewUUID()),
					UserID:                  testClusterUser.Clusteruser_id,
					PrivateURL:              "https://test-private-url",
					AuthUsername:            "test-auth-username",
					AuthPassword:            "test-auth-password",
					AuthSSHKey:              "test-auth-ssh-key",
					SecretObj:               "test-secret-obj",
					EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
				}
				err = dbQueries.CreateRepositoryCredentials(ctx, &repoCredentials)
				Expect(err).ToNot(HaveOccurred())

				dbAppProjectRepo := &db.AppProjectRepository{
					AppprojectRepositoryID:  "test-appProject-repo-id",
					Clusteruser_id:          testClusterUser.Clusteruser_id,
					RepositorycredentialsID: repoCredentials.RepositoryCredentialsID,
					RepoURL:                 "test-url",
				}

				err = dbQueries.CreateAppProjectRepository(ctx, dbAppProjectRepo)
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("Verify whether AppProject has been created")
				appProject := &appv1.AppProject{
					ObjectMeta: metav1.ObjectMeta{
						Name:      appProjectPrefix + operationDB.Operation_owner_user_id,
						Namespace: namespace,
					},
				}

				var appProjectRepositories []db.AppProjectRepository
				err = dbQueries.UnsafeListAllAppProjectRepositories(ctx, &appProjectRepositories)
				Expect(err).ToNot(HaveOccurred())

				var repoURLs []string
				for _, v := range appProjectRepositories {
					if v.Clusteruser_id == operationDB.Operation_owner_user_id {
						repoURLs = append(repoURLs, v.RepoURL)
					}
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: appProject.Namespace, Name: appProject.Name}, appProject)
				Expect(err).ToNot(HaveOccurred())
				Expect(appProject).ToNot(BeNil())
				Expect(appProject.Name).To(Equal(appProjectPrefix + operationDB.Operation_owner_user_id))
				Expect(appProject.Namespace).To(Equal(namespace))
				Expect(appProject.Spec.SourceRepos).To(Equal(repoURLs))
				Expect(appProject.Spec.Destinations).To(Equal([]appv1.ApplicationDestination{{
					Namespace: "*",
					Name:      "in-cluster",
				}}))
				Expect(appProject.Annotations["username"]).To(Equal(testClusterUser.Clusteruser_id))

				By("deleting resources and cleaning up db entries created by test.")

				resourcesToBeDeleted := testResources{
					Application_id:          applicationDB.Application_id,
					Operation_id:            []string{operationDB.Operation_id},
					Gitopsenginecluster_id:  gitopsEngineCluster.Gitopsenginecluster_id,
					Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
					ClusterCredentials_id:   gitopsEngineCluster.Clustercredentials_id,
					RepositoryCredentialsID: repoCredentials.RepositoryCredentialsID,
					AppProjectRepositoryID:  dbAppProjectRepo.AppprojectRepositoryID,
				}

				deleteTestResources(ctx, dbQueries, resourcesToBeDeleted)

			})

			It("Verify whether appProject CR is not deleted if appProjectManagedEnv/appProjectRepo row still exists", func() {
				applicationCR := &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels: map[string]string{
							dbID: "doesnt-exist",
						},
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
					},
				}

				err = task.event.client.Create(ctx, applicationCR)
				Expect(err).ToNot(HaveOccurred())

				// The Argo CD Applications used by GitOps Service use finalizers, so the Applicaitonwill not be deleted until the finalizer is removed.
				// Normally it us Argo CD's job to do this, but since this is a unit test, there is no Argo CD. Instead we wait for the deletiontimestamp
				// to be set (by the delete call of PerformTask, and then just remove the finalize and update, simulating what Argo CD does)
				go func() {
					err = wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
						if applicationCR.DeletionTimestamp != nil {
							err = k8sClient.Get(ctx, client.ObjectKeyFromObject(applicationCR), applicationCR)
							Expect(err).ToNot(HaveOccurred())

							applicationCR.Finalizers = nil

							err = k8sClient.Update(ctx, applicationCR)
							Expect(err).ToNot(HaveOccurred())

							err = task.event.client.Delete(ctx, applicationCR)
							return true, nil
						}
						return false, nil
					})
				}()

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())
				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "doesnt-exist",
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				By("Creating AppProjectManagedEnvironment")
				appProjectManagedEnv := db.AppProjectManagedEnvironment{
					AppprojectManagedenvID: "test-app-managedenv-id-1",
					Managed_environment_id: managedEnvironment.Managedenvironment_id,
					Clusteruser_id:         operationDB.Operation_owner_user_id,
				}
				err = dbQueries.CreateAppProjectManagedEnvironment(ctx, &appProjectManagedEnv)
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				appProject := &appv1.AppProject{
					ObjectMeta: metav1.ObjectMeta{
						Name:      appProjectPrefix + operationDB.Operation_owner_user_id,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: appProject.Namespace, Name: appProject.Name}, appProject)
				Expect(err).ToNot(HaveOccurred())
				Expect(appProject).ToNot(BeNil())

			})

			It("Verify appProjectEqual function works as expected", func() {
				var isAppProjectEqual bool

				By("verify whether existingAppProject and generatedAppProject are nil and it should return false")
				var existingAppProject *appv1.AppProject
				var generatedAppProject *appv1.AppProject

				existingAppProject = nil
				generatedAppProject = nil

				isAppProjectEqual = appProjectEqual(existingAppProject, generatedAppProject)
				Expect(isAppProjectEqual).To(BeFalse())

				By("verify whether existingAppProject and generatedAppProject have different lengths of SourceRepos and it should return false")
				existingAppProject = &appv1.AppProject{}
				generatedAppProject = &appv1.AppProject{}

				existingAppProject.Spec.SourceRepos = []string{"repo1", "repo2"}
				generatedAppProject.Spec.SourceRepos = []string{"repo1"}

				isAppProjectEqual = appProjectEqual(existingAppProject, generatedAppProject)
				Expect(isAppProjectEqual).To(BeFalse())

				By("verify whether existingAppProject and generatedAppProject have different repositories in SourceRepos and it should return false")
				existingAppProject = &appv1.AppProject{}
				generatedAppProject = &appv1.AppProject{}

				existingAppProject.Spec.SourceRepos = []string{"repo1", "repo2"}
				generatedAppProject.Spec.SourceRepos = []string{"repo1", "repo3"}

				isAppProjectEqual = appProjectEqual(existingAppProject, generatedAppProject)
				Expect(isAppProjectEqual).To(BeFalse())

				By("verify whether existingAppProject and generatedAppProject have different lengths of Destinations and it should return false")
				existingAppProject = &appv1.AppProject{}
				generatedAppProject = &appv1.AppProject{}

				existingAppProject.Spec.Destinations = []appv1.ApplicationDestination{{Name: "dest1"}, {Name: "dest2"}}
				generatedAppProject.Spec.Destinations = []appv1.ApplicationDestination{{Name: "dest1"}}

				isAppProjectEqual = appProjectEqual(existingAppProject, generatedAppProject)
				Expect(isAppProjectEqual).To(BeFalse())

				By("verify whether existingAppProject and generatedAppProject have different Destinations and it should return false")
				existingAppProject = &appv1.AppProject{}
				generatedAppProject = &appv1.AppProject{}

				existingAppProject.Spec.Destinations = []appv1.ApplicationDestination{{Name: "dest1"}, {Name: "dest2"}}
				generatedAppProject.Spec.Destinations = []appv1.ApplicationDestination{{Name: "dest1"}, {Name: "dest3"}}

				isAppProjectEqual = appProjectEqual(existingAppProject, generatedAppProject)
				Expect(isAppProjectEqual).To(BeFalse())

				By("verify whether existingAppProject and generatedAppProject have the same SourceRepos and Destinations and it should return true")
				existingAppProject = &appv1.AppProject{}
				generatedAppProject = &appv1.AppProject{}

				existingAppProject.Spec.SourceRepos = []string{"repo1", "repo2"}
				generatedAppProject.Spec.SourceRepos = []string{"repo1", "repo2"}
				existingAppProject.Spec.Destinations = []appv1.ApplicationDestination{{Name: "dest1"}, {Name: "dest2"}}
				generatedAppProject.Spec.Destinations = []appv1.ApplicationDestination{{Name: "dest1"}, {Name: "dest2"}}

				isAppProjectEqual = appProjectEqual(existingAppProject, generatedAppProject)
				Expect(isAppProjectEqual).To(BeTrue())

			})

		})

		Context("Process SyncOperation", func() {

			var (
				applicationDB          *db.Application
				applicationCR          *appv1.Application
				gitopsEngineInstanceID string
				closeRefreshHandler    chan struct{}
				refreshAnnotationFound chan struct{}
			)

			createOperationDBAndCR := func(resourceID, gitopsEngineInstanceID string) {
				By("creating new operation row of type SyncOperation in the database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstanceID,
					Resource_id:             resourceID,
					Resource_type:           db.OperationResourceType_SyncOperation,
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())
			}

			// refreshHandler mocks the refresh handling behaviour of the Argo CD Application Controller.
			// Argo CD removes the refresh annotation once the refresh is done. Similarly, the refresh
			// handler mock watches the Argo CD Applications and removes the refresh annotation indicating
			// that the refresh was successful.
			refreshHandler := func(appCR appv1.Application, closeRefreshHandler, refreshAnnotationFound chan struct{}) {
				defer GinkgoRecover()
				for {
					<-time.After(100 * time.Millisecond)

					select {
					case <-closeRefreshHandler:
						return
					default:
					}

					found := false
					err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&appCR), &appCR)
						if err != nil {
							if apierr.IsNotFound(err) {
								return nil
							}
							return err
						}

						if _, ok := appCR.Annotations[appv1.AnnotationKeyRefresh]; ok {
							found = true
							delete(appCR.Annotations, appv1.AnnotationKeyRefresh)
							return k8sClient.Update(ctx, &appCR)
						}
						return nil
					})
					if found {
						refreshAnnotationFound <- struct{}{}
					}
					Expect(err).ToNot(HaveOccurred())
				}
			}

			BeforeEach(func() {

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				_, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineInstanceID = gitopsEngineInstance.Gitopsengineinstance_id

				applicationDB = &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				By("create Application in Database")
				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("create Application CR")
				applicationCR = &appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: namespace,
					},
				}
				err = k8sClient.Create(ctx, applicationCR)
				Expect(err).ToNot(HaveOccurred())

				closeRefreshHandler = make(chan struct{})
				refreshAnnotationFound = make(chan struct{})
				go refreshHandler(*applicationCR, closeRefreshHandler, refreshAnnotationFound)
			})

			AfterEach(func() {
				closeRefreshHandler <- struct{}{}
				dbQueries.CloseDatabase()
				testTeardown()
			})

			It("should handle a valid SyncOperation", func() {

				By("create a SyncOperation in the database")
				syncOperation := db.SyncOperation{
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Running,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("verify there is no retry for a successful sync")
				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return nil
					},
					refreshApp: refreshApplication,
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("verify if the refresh annotation was added")
				Expect(<-refreshAnnotationFound).To(Equal(struct{}{}))
			})

			It("should return an error and retry if the sync fails", func() {

				By("create a SyncOperation in the database")
				syncOperation := db.SyncOperation{
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Running,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("check if the sync failed error is returned with retry")
				expectedErr := "sync failed due to xyz reason"
				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return fmt.Errorf(expectedErr)
					},
					refreshApp: refreshApplication,
				}

				retry, err := task.PerformTask(ctx)
				Expect(err.Error()).Should(Equal(expectedErr))
				Expect(retry).To(BeTrue())

				By("verify if the refresh annotation was added")
				Expect(<-refreshAnnotationFound).To(Equal(struct{}{}))
			})

			It("should return an error and retry if the refresh fails", func() {
				By("create a SyncOperation in the database")
				syncOperation := db.SyncOperation{
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Running,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("check if the sync failed error is returned with retry")
				expectedErr := "failed to refresh app: operation"
				task.syncFuncs = &syncFuncs{
					refreshApp: func(ctx context.Context, c client.Client, s1, s2 string) error {
						return fmt.Errorf("failed to refresh app: %s", s1)
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err.Error()).Should(Equal(expectedErr))
				Expect(retry).To(BeTrue())
			})

			It("should handle conflicts while refreshing application", func() {
				By("create a SyncOperation in the database")
				syncOperation := db.SyncOperation{
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Running,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("introduce a conflict and verify if it is handled while adding refresh annotation")
				appCRClone := applicationCR.DeepCopy()
				appCRClone.Annotations = make(map[string]string)
				appCRClone.Annotations = map[string]string{
					"key-1": "value-1",
					"key-2": "value-2",
				}
				err = k8sClient.Update(ctx, appCRClone)
				Expect(err).ToNot(HaveOccurred())

				// verify if a conflict has been introduced.
				applicationCR.Annotations = make(map[string]string)
				applicationCR.Annotations["key-3"] = "value-3"
				err = k8sClient.Update(ctx, applicationCR)
				Expect(apierr.IsConflict(err)).To(BeTrue())

				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return nil
					},
					refreshApp: refreshApplication,
				}

				// verify if the Application was refreshed by overcoming the conflict introduced in the previous step.
				retry, err := task.PerformTask(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(retry).To(BeFalse())

				By("verify if the refresh annotation was added")
				Expect(<-refreshAnnotationFound).To(Equal(struct{}{}))
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
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        "uknown",
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				task.syncFuncs = &syncFuncs{
					appSync: func(ctx context.Context, s1, s2, s3 string, c client.Client, cs *utils.CredentialService, b bool) error {
						return nil
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(retry).To(BeFalse())
			})

			It("don't retry if we can successfully terminate a sync operation", func() {
				By("update the Application operation state so it can be terminated later")
				updateApplicationOperationState(applicationCR)

				By("create a SyncOperation with desired state 'Terminated' in the database")
				syncOperation := db.SyncOperation{
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Terminated,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("verify that there is no retry and error for a successful termination")
				task.syncFuncs = &syncFuncs{
					terminateOperation: func(ctx context.Context, s string, n corev1.Namespace, cs *utils.CredentialService, c client.Client, d time.Duration, l logr.Logger) error {
						return nil
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(retry).To(BeFalse())
			})

			It("should return an error and retry if we can't terminate a sync operation", func() {
				By("update the Application operation state so it can be terminated later")
				updateApplicationOperationState(applicationCR)

				By("create a SyncOperation with desired state 'Terminated' in the database")
				syncOperation := db.SyncOperation{
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Terminated,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("check if an error is returned for the failed termination")
				expectedErr := "unable to terminate sync due to xyz reason"
				task.syncFuncs = &syncFuncs{
					terminateOperation: func(ctx context.Context, s string, n corev1.Namespace, cs *utils.CredentialService, c client.Client, d time.Duration, l logr.Logger) error {
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
					SyncOperation_id:    "test-syncoperation",
					Application_id:      applicationDB.Application_id,
					DeploymentNameField: "test",
					Revision:            "main",
					DesiredState:        db.SyncOperation_DesiredState_Terminated,
				}
				err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
				Expect(err).ToNot(HaveOccurred())

				By("create Operation DB row and CR for the SyncOperation")
				createOperationDBAndCR(syncOperation.SyncOperation_id, gitopsEngineInstanceID)

				By("verify there is no termination and retry should be false")
				task.syncFuncs = &syncFuncs{
					terminateOperation: func(ctx context.Context, s string, n corev1.Namespace, cs *utils.CredentialService, c client.Client, d time.Duration, l logr.Logger) error {
						return fmt.Errorf("unable to terminate sync due to xyz reason")
					},
				}

				retry, err := task.PerformTask(ctx)
				Expect(err).ShouldNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())

				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())

				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())

				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(isRunning).To(BeTrue())

			})

			It("should return a missing error if the Application is not found", func() {
				isRunning, err := isOperationRunning(ctx, k8sClient, applicationCR.Name, applicationCR.Namespace)
				Expect(err).To(HaveOccurred())
				Expect(apierr.IsNotFound(err)).Should(BeTrue())
				Expect(isRunning).To(BeFalse())
			})
		})

		Context("Operation CRs should only be processed if they are created in GitOpsEngineInstance namespace", func() {

			It("ensures that Operation should be processed if created in a GitopsEngineInstance namespace", func() {
				err = db.SetupForTestingDBGinkgo()
				Expect(err).ToNot(HaveOccurred())
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          namespace,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				By("Create Application in Database")
				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).ToNot(HaveOccurred())

				By("If no error was returned, and retry is false, then verify that the 'state' field of the Operation row is Completed")
				err = dbQueries.GetOperationById(ctx, operationDB)
				Expect(err).ToNot(HaveOccurred())
				Expect(operationDB.State).To(Equal(db.OperationState_Completed))

				Expect(retry).To(BeFalse())
				By("Verifying whether Application CR is created")
				applicationCR := appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: name}, &applicationCR)
				Expect(err).ToNot(HaveOccurred())
				Expect(dummyApplicationSpec.Spec).To(Equal(applicationCR.Spec))

			})

			It("ensures that Operation should not be processed if created outside a GitopsEngineInstance namespace, and error should be returned", func() {
				By("Close database connection")
				err = db.SetupForTestingDBGinkgo()
				Expect(err).ToNot(HaveOccurred())
				defer dbQueries.CloseDatabase()
				defer testTeardown()

				_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
				Expect(err).ToNot(HaveOccurred())

				dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
				Expect(err).ToNot(HaveOccurred())

				gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
				Expect(gitopsEngineCluster).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				By("creating a gitops engine instance with a namespace name/uid that don't exist in fakeclient")
				gitopsEngineInstance := &db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance",
					Namespace_name:          workspace.Name,
					Namespace_uid:           string(workspace.UID),
					EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
				}
				err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
				Expect(err).ToNot(HaveOccurred())

				By("Create Application in Database")
				applicationDB := &db.Application{
					Application_id:          "test-my-application",
					Name:                    name,
					Spec_field:              dummyApplicationSpecString,
					Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
					Managed_environment_id:  managedEnvironment.Managedenvironment_id,
				}

				err = dbQueries.CreateApplication(ctx, applicationDB)
				Expect(err).ToNot(HaveOccurred())

				By("Creating Operation row in database")
				operationDB := &db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             applicationDB.Application_id,
					Resource_type:           "Application",
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: testClusterUser.Clusteruser_id,
				}

				err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
				Expect(err).ToNot(HaveOccurred())

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
				Expect(err).ToNot(HaveOccurred())

				retry, err := task.PerformTask(ctx)
				Expect(err).To(HaveOccurred())
				Expect(retry).To(BeFalse())
				errString := err.Error()
				mismatchedNamespaceErr := "OperationNS: " + operationCR.Namespace + " " + "GitopsEngineInstanceNS: " + gitopsEngineInstance.Namespace_name
				Expect(errString).To(Equal("OperationCR namespace did not match with existing namespace of GitopsEngineInstance " + mismatchedNamespaceErr))

				By("Verifying whether Application CR is not created")
				applicationCR := appv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      applicationDB.Name,
						Namespace: namespace,
					},
				}

				err = task.event.client.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: name}, &applicationCR)
				Expect(err).To(HaveOccurred())
				Expect(dummyApplicationSpec.Spec).ToNot(Equal(applicationCR.Spec))
			})
		})
	})

})

func testTeardown() {
	err := db.SetupForTestingDBGinkgo()
	Expect(err).ToNot(HaveOccurred())
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
	Gitopsenginecluster_id        string
	Gitopsengineinstance_id       string
	ClusterCredentials_id         string
	Application_id                string
	Managedenvironment_id         string
	kubernetesToDBResourceMapping db.KubernetesToDBResourceMapping
	SyncOperation_id              string
	RepositoryCredentialsID       string
	AppProjectRepositoryID        string
}

// Delete resources from table
func deleteTestResources(ctx context.Context, dbQueries db.AllDatabaseQueries, resourcesToBeDeleted testResources) {
	var rowsAffected int
	var err error

	// Delete SyncOperation
	if resourcesToBeDeleted.SyncOperation_id != "" {
		rowsAffected, err = dbQueries.DeleteSyncOperationById(ctx, resourcesToBeDeleted.SyncOperation_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete Application
	if resourcesToBeDeleted.Application_id != "" {
		rowsAffected, err = dbQueries.DeleteApplicationById(ctx, resourcesToBeDeleted.Application_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete AppProjectRepository
	if resourcesToBeDeleted.AppProjectRepositoryID != "" {
		appProjectRepository := &db.AppProjectRepository{
			RepositorycredentialsID: resourcesToBeDeleted.RepositoryCredentialsID,
		}
		rowsAffected, err = dbQueries.DeleteAppProjectRepositoryByRepoCredId(ctx, appProjectRepository)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete RepositoryCredentials
	if resourcesToBeDeleted.RepositoryCredentialsID != "" {
		rowsAffected, err = dbQueries.DeleteRepositoryCredentialsByID(ctx, resourcesToBeDeleted.RepositoryCredentialsID)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete kubernetesToDBResourceMapping
	if resourcesToBeDeleted.kubernetesToDBResourceMapping.KubernetesResourceUID != "" {
		rowsAffected, err = dbQueries.DeleteKubernetesResourceToDBResourceMapping(ctx, &resourcesToBeDeleted.kubernetesToDBResourceMapping)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete ManagedEnvironment
	if resourcesToBeDeleted.Managedenvironment_id != "" {
		rowsAffected, err = dbQueries.DeleteManagedEnvironmentById(ctx, resourcesToBeDeleted.Managedenvironment_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete Operation
	for _, operationToDelete := range resourcesToBeDeleted.Operation_id {
		rowsAffected, err = dbQueries.DeleteOperationById(ctx, operationToDelete)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))

	}

	// Delete GitopsEngineInstance
	if resourcesToBeDeleted.Gitopsengineinstance_id != "" {
		rowsAffected, err = dbQueries.DeleteGitopsEngineInstanceById(ctx, resourcesToBeDeleted.Gitopsengineinstance_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete GitopsEngineCluster
	if resourcesToBeDeleted.Gitopsenginecluster_id != "" {
		rowsAffected, err = dbQueries.DeleteGitopsEngineClusterById(ctx, resourcesToBeDeleted.Gitopsenginecluster_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

	// Delete ClusterCredentials
	if resourcesToBeDeleted.ClusterCredentials_id != "" {
		rowsAffected, err = dbQueries.DeleteClusterCredentialsById(ctx, resourcesToBeDeleted.ClusterCredentials_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).To(Equal(1))
	}

}

func createDummyApplicationData() (appv1.Application, string, error) {
	return createCustomizedDummyApplicationData("guestbook")
}

func createCustomizedDummyApplicationData(repoPath string) (appv1.Application, string, error) {
	// Create dummy Application Spec to be saved in DB
	dummyApplicationSpec := appv1.Application{
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
			Project: "app-project-test-user",
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
