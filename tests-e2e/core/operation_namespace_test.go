package core

import (
	"context"
	"fmt"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Operation CR namespace E2E tests", func() {

	const (
		operationNamespace = "test-argocd-namespace"
		operationName      = "test-operation"
		applicationName    = "test-application"
		testNamespace      = fixture.GitOpsServiceE2ENamespace
	)

	Context("Operation CR created in an invalid namespace should be ignored", func() {
		var err error
		var ctx context.Context

		var dbQueries db.AllDatabaseQueries
		var k8sClient client.Client
		var config *rest.Config
		var operationDB *db.Operation
		var operationCR *managedgitopsv1alpha1.Operation
		var namespaceToTarget *corev1.Namespace

		BeforeEach(func() {

			config, err = fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())
			k8sClient, err = fixture.GetKubeClient(config)
			Expect(err).To(BeNil())
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())
			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

		})
		AfterEach(func() {
			log := log.FromContext(ctx)
			err = operations.CleanupOperation(ctx, *operationDB, *operationCR, operationCR.Namespace, dbQueries, k8sClient, true, log)
			Expect(err).To(BeNil())
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())
			err = fixture.DeleteNamespace(operationNamespace, config)
			Expect(err).To(BeNil())
			err = fixture.DeleteNamespace(testNamespace, config)
			Expect(err).To(BeNil())

		})

		It("should create an Operation CR in a valid namespace, and we should see the Operation is processed", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			ctx = context.Background()
			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			By("Creating Operation CR")
			log := log.FromContext(ctx)
			namespaceToTarget = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: operationNamespace,
				},
			}
			err = k8s.Create(namespaceToTarget, k8sClient)
			Expect(err).To(Succeed())

			kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
			err = k8s.Get(kubeSystemNamespace, k8sClient)
			Expect(err).To(Succeed())

			dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
			Expect(err).To(BeNil())

			_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubeSystemNamespace.UID), dbQueries, log)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			namespaceToTarget = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operationNamespace}}
			err = k8s.Get(namespaceToTarget, k8sClient)
			Expect(err).To(Succeed())

			By("Creating a gitopsEngineInstance")
			gitopsEngineInstance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-engine-instance",
				Namespace_name:          namespaceToTarget.Name,
				Namespace_uid:           string(namespaceToTarget.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Create Application in Database")
			applicationDB := &db.Application{
				Application_id:          "test-my-application-id",
				Name:                    applicationName,
				Spec_field:              dummyApplicationSpecString,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())
			operationDB = &db.Operation{
				Operation_id:            operationName,
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             applicationDB.Application_id,
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: "test-user",
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			operationCR = &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: namespaceToTarget.Name,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = k8sClient.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			By("Verifying whether Application CR is created")
			applicationCR := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      applicationName,
					Namespace: namespaceToTarget.Name,
				},
			}

			Eventually(func() bool {
				isComplete, _ := operations.IsOperationComplete(ctx, operationDB, dbQueries)
				fmt.Println("- Operation state result achieved : ", isComplete)
				return isComplete
			}, "1m", "5s").Should(BeTrue())

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: applicationCR.Name}, &applicationCR)
			Expect(err).To(BeNil())
			Expect(dummyApplicationSpec.Spec).To(Equal(applicationCR.Spec))

		})
		It("should create an Operation CR in an invalid namespace, a non-Argo CD namespace, and we should see the Operation is never processed", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			ctx = context.Background()
			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			By("Creating Operation CR")
			log := log.FromContext(ctx)
			namespaceToTarget = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: operationNamespace,
				},
			}
			err = k8s.Create(namespaceToTarget, k8sClient)
			Expect(err).To(Succeed())

			kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
			err = k8s.Get(kubeSystemNamespace, k8sClient)
			Expect(err).To(Succeed())

			dummyApplicationSpec, dummyApplicationSpecString, err := createDummyApplicationData()
			Expect(err).To(BeNil())

			_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubeSystemNamespace.UID), dbQueries, log)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			namespaceToTarget = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operationNamespace}}
			err = k8s.Get(namespaceToTarget, k8sClient)
			Expect(err).To(Succeed())

			By("Creating a gitopsEngineInstancee")
			gitopsEngineInstance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-E2E",
				Namespace_name:          namespaceToTarget.Name,
				Namespace_uid:           string(namespaceToTarget.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Create Application in Database")
			applicationDB := &db.Application{
				Application_id:          "test-my-application-id",
				Name:                    applicationName,
				Spec_field:              dummyApplicationSpecString,
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbQueries.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())
			operationDB = &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_Application,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: "test-user",
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			operationCR = &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fake-operation",
					Namespace: testNamespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = k8sClient.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			By("Verifying whether Application CR is created")
			applicationCR := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "operation",
					Namespace: "my-user",
				},
			}

			Consistently(func() bool {
				isComplete, _ := operations.IsOperationComplete(ctx, operationDB, dbQueries)
				fmt.Println("- Operation state result achieved : ", isComplete)
				return isComplete
			}, "1m", "5s").Should(BeFalse())

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: applicationCR.Namespace, Name: applicationCR.Name}, &applicationCR)
			Expect(err).ToNot(BeNil())
			Expect(dummyApplicationSpec.Spec).ToNot(Equal(applicationCR.Spec))

		})
	})
})

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
			Name:      "test-application",
			Namespace: "test-argocd-namespace",
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
		Status: appv1.ApplicationStatus{
			Sync: appv1.SyncStatus{
				ComparedTo: appv1.ComparedTo{
					Source: appv1.ApplicationSource{
						Path:           "guestbook",
						TargetRevision: "HEAD",
						RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
					},
					Destination: appv1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: "in-cluster",
						Name:      "in-cluster",
					},
				},
			},
		},
	}

	dummyApplicationSpecBytes, err := yaml.Marshal(dummyApplicationSpec)

	if err != nil {
		return appv1.Application{}, "", err
	}

	return dummyApplicationSpec, string(dummyApplicationSpecBytes), nil
}
