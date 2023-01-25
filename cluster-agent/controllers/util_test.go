package controllers

import (
	"context"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Tests for the small number of utility functions in cluster-agent/controllers util.go", func() {

	Context("DeleteArgoCDApplication tests", func() {

		var ctx context.Context
		var k8sClient client.WithWatch
		var logger logr.Logger
		var kubesystemNamespace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var workspace *corev1.Namespace
		var scheme *runtime.Scheme
		var err error

		simulateArgoCD := func(goApplication *appv1.Application) {
			Eventually(func() bool {

				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(goApplication), goApplication)
				if err != nil {
					GinkgoWriter.Println("error from client on Get", err)
					return false
				}

				// Wait for the finalizer to be added by 'DeleteArgoCDApplication'
				finalizerFound := false
				for _, finalizer := range goApplication.Finalizers {
					if finalizer == argoCDResourcesFinalizer {
						finalizerFound = true
					}
				}
				if !finalizerFound {
					GinkgoWriter.Println("finalizer not yet set on Application")
					return false
				}

				// Wait for DeleteArgoCDApplication to delete the Application
				if goApplication.DeletionTimestamp == nil {
					GinkgoWriter.Println("DeletionTimestamp is not yet set")
					return false
				}

				// The remaining steps simulate Argo CD deleting the Application

				// Remove the finalizer
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(goApplication), goApplication)
				if err != nil {
					GinkgoWriter.Println("error from client on second Get", err)
					return false
				}

				goApplication.Finalizers = []string{}
				err = k8sClient.Update(ctx, goApplication)
				if err != nil {
					GinkgoWriter.Println("unable to update Application")
					return false
				}

				err = k8sClient.Delete(ctx, goApplication)
				if err != nil {
					GinkgoWriter.Println("error from client on Delete", err)
					return false
				}

				GinkgoWriter.Println("The Application was successfully deleted.")

				return true
			}, "5s", "10ms").Should(BeTrue())

		}

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			scheme, argocdNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			By("Initialize fake kube client")
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, argocdNamespace, kubesystemNamespace).Build()

		})

		It("should not delete an Argo CD Application which is missing the databaseID label", func() {

			By("creating an Argo CD Application without a databaseID label")
			application := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-name",
					Namespace: "my-namespace",
					Labels:    map[string]string{},
				},
			}
			err := k8sClient.Create(ctx, &application)
			Expect(err).To(BeNil())

			By("calling the DeleteArgoCDApplication function")
			err = DeleteArgoCDApplication(ctx, application, k8sClient, logger)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&application), &application)
			Expect(err).To(BeNil(), "Application should still exist: it should not have been deleted")
			Expect(len(application.Finalizers)).To(BeZero(), "no finalizers should have been added")
		})

		It("should delete an Argo CD Application which has the databaseID label", func() {

			By("creating an Argo CD Application with a databaseID label")
			application := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-name",
					Namespace: "my-namespace",
					Labels: map[string]string{
						ArgoCDApplicationDatabaseIDLabel: "test-my-database-id-label",
					},
				},
			}
			err := k8sClient.Create(ctx, &application)
			Expect(err).To(BeNil())

			By("starting a Goroutine to simulate Argo CD's deletion behaviour")
			goApplication := application.DeepCopy()
			go func() {
				simulateArgoCD(goApplication)
			}()

			By("calling the DeleteArgoCDApplication function")
			err = DeleteArgoCDApplication(ctx, application, k8sClient, logger)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&application), &application)
			Expect(err).ToNot(BeNil(), "Application should not exist: it should have been deleted")

		})

		It("should delete an Argo CD Application which has the databaseID label, and the Application already has a deletion finalizer set", func() {

			By("creating an Argo CD Application with a finalizer and a databaseID label")
			application := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-name",
					Namespace: "my-namespace",
					Labels: map[string]string{
						ArgoCDApplicationDatabaseIDLabel: "test-my-database-id-label",
					},
					Finalizers: []string{
						argoCDResourcesFinalizer,
					},
				},
			}
			err := k8sClient.Create(ctx, &application)
			Expect(err).To(BeNil())

			By("starting a Goroutine to simulate Argo CD's deletion behaviour")
			goApplication := application.DeepCopy()
			go func() {
				simulateArgoCD(goApplication)
			}()

			By("calling the DeleteArgoCDApplication function")
			err = DeleteArgoCDApplication(ctx, application, k8sClient, logger)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&application), &application)
			Expect(err).ToNot(BeNil(), "Application should not exist: it should have been deleted")

		})

	})

	Context("Testing for CompareApplications function.", func() {
		createDummyApplicationData := func() (fauxargocd.FauxApplication, string, appv1.Application, error) {
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
						RepoURL:        "https://github.com/redhat-appstudio/managed-gitops",
						Path:           "resources/test-data/sample-gitops-repository/environments/overlays/dev",
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
						RepoURL:        "https://github.com/redhat-appstudio/managed-gitops",
						Path:           "resources/test-data/sample-gitops-repository/environments/overlays/dev",
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

		It("Should compare applications.", func() {

			var dbApp db.Application

			applicationFromDB, yamlData, applicationFromArgoCD, err := createDummyApplicationData()
			Expect(err).To(BeNil())

			dbApp.Spec_field = yamlData

			var ctx context.Context
			log := log.FromContext(ctx)

			result, err := CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).To(BeEmpty())

			applicationFromArgoCD.Spec.Source.RepoURL = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Source.RepoURL = applicationFromDB.Spec.Source.RepoURL

			applicationFromArgoCD.Spec.Source.Path = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Source.Path = applicationFromDB.Spec.Source.Path

			applicationFromArgoCD.Spec.Source.TargetRevision = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Source.TargetRevision = applicationFromDB.Spec.Source.TargetRevision

			applicationFromArgoCD.Spec.Destination.Server = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Destination.Server = applicationFromDB.Spec.Destination.Server

			applicationFromArgoCD.Spec.Destination.Namespace = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Destination.Namespace = applicationFromDB.Spec.Destination.Namespace

			applicationFromArgoCD.Spec.Destination.Name = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Destination.Name = applicationFromDB.Spec.Destination.Name

			applicationFromArgoCD.Spec.Project = "test"
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.Project = applicationFromDB.Spec.Project

			applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune = true
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune = applicationFromDB.Spec.SyncPolicy.Automated.Prune

			applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal = true
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal = applicationFromDB.Spec.SyncPolicy.Automated.SelfHeal

			applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty = true
			result, err = CompareApplication(applicationFromArgoCD, dbApp, log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())
			applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty = applicationFromDB.Spec.SyncPolicy.Automated.AllowEmpty
		})

		It("Should compare applications if fields are nil.", func() {

			// Convert a FauxApplication into a db.Application, by marshalling the FA back into YAML
			convert := func(fa fauxargocd.FauxApplication) db.Application {

				bytes, err := yaml.Marshal(&fa)

				if err != nil {
					return db.Application{}
				}

				res := db.Application{
					Spec_field: string(bytes),
				}

				return res

			}

			appDB, _, appArgo, err := createDummyApplicationData()
			Expect(err).To(BeNil())

			var ctx context.Context
			log := log.FromContext(ctx)

			// Store value for later use
			tempSyncPolicyDB, tempSyncPolicyArgo := appDB.Spec.SyncPolicy, appArgo.Spec.SyncPolicy

			By("SyncPolicy is nil in both Argo CD and DB entry, so consider fields are in Sync.")

			appDB.Spec.SyncPolicy,
				appArgo.Spec.SyncPolicy = nil, nil
			result, err := CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())
			Expect(result).To(BeEmpty())

			By("SyncPolicy is nil in Argo CD, but not in DB entry, hence it is not in sync.")

			appDB.Spec.SyncPolicy = tempSyncPolicyDB
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())

			By("SyncPolicy is nil in DB, but not in Argo CD, hence it is not in sync.")

			appDB.Spec.SyncPolicy,
				appArgo.Spec.SyncPolicy = nil, tempSyncPolicyArgo
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())

			// Reset SyncPolicy values
			appDB.Spec.SyncPolicy, appArgo.Spec.SyncPolicy = tempSyncPolicyDB, tempSyncPolicyArgo

			// Store value for later use
			tempAutomatedDB, tempAutomatedArgo := appDB.Spec.SyncPolicy.Automated, appArgo.Spec.SyncPolicy.Automated

			By("SyncPolicy.Automated is nil in both Argo CD and DB entry, so consider fields are in Sync.")

			appDB.Spec.SyncPolicy.Automated,
				appArgo.Spec.SyncPolicy.Automated = nil, nil
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())

			Expect(result).To(BeEmpty())

			By("SyncPolicy.Automated is nil in Argo CD but not in DB entry, hence it is not in sync.")

			appDB.Spec.SyncPolicy.Automated = tempAutomatedDB
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeEmpty())

			By("SyncPolicy.Automated is nil in DB but not in Argo CD, hence it is not in sync.")

			appDB.Spec.SyncPolicy.Automated,
				appArgo.Spec.SyncPolicy.Automated = nil, tempAutomatedArgo
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())

			Expect(result).ToNot(BeEmpty())

			// Reset SyncPolicy.Automated values
			appDB.Spec.SyncPolicy.Automated, appArgo.Spec.SyncPolicy.Automated = tempAutomatedDB, tempAutomatedArgo

			By("SyncPolicy values are different in Argo CD and DB, hence it is not in sync.")

			appDB.Spec.SyncPolicy.Automated.Prune,
				appArgo.Spec.SyncPolicy.Automated.Prune = true, false
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())

			Expect(result).ToNot(BeEmpty())

			By("SyncPolicy values are same in Argo CD and DB, hence it is in sync.")

			appDB.Spec.SyncPolicy.Automated.Prune,
				appArgo.Spec.SyncPolicy.Automated.Prune = true, true
			result, err = CompareApplication(appArgo, convert(appDB), log)
			Expect(err).To(BeNil())

			Expect(result).To(BeEmpty())
		})
	})

})
