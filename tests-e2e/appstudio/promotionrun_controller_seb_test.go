package appstudio

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiosharedv1beta1 "github.com/redhat-appstudio/application-api/api/v1beta1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	promotionRunFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/promotionrun"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Promotion Run Creation of SnapshotEnvironmentBinding E2E Tests.", func() {
	Context("Testing Promotion Run Creation of SnapshotEnvironmentBinding.", func() {
		var environmentProd appstudiosharedv1beta1.Environment
		var promotionRun appstudiosharedv1.PromotionRun
		var k8sClient client.Client
		var err error

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("Create Staging Environment.")
			environmentStage := buildEnvironmentResource("staging", "Staging Environment", "staging", appstudiosharedv1beta1.EnvironmentType_POC)
			err = k8s.Create(&environmentStage, k8sClient)
			Expect(err).To(Succeed())

			By("Create Production Environment.")
			environmentProd = buildEnvironmentResource("prod", "Production Environment", "prod", appstudiosharedv1beta1.EnvironmentType_POC)
			err = k8s.Create(&environmentProd, k8sClient)
			Expect(err).To(Succeed())

			By("Create Snapshot.")
			snapshot := buildSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&snapshot, k8sClient)
			Expect(err).To(Succeed())

			By("Create Component.")
			component := buildComponentResource("comp1", "component-a", "new-demo-app")
			err = k8s.Create(&component, k8sClient)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			promotionRun = promotionRunFixture.BuildPromotionRunResource("new-demo-app-manual-promotion", "new-demo-app", "my-snapshot", "prod")
		})

		It("Creates a SnapshotEnvironmentBinding if one doesn't exist that targets the application/environment.", func() {
			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			By("Check binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Eventually(binding, "60s", "5s").Should(k8s.ExistByName(k8sClient))
		})

		It("Creates a SnapshotEnvironmentBinding if one exists that targets the application, but NOT the environment.", func() {
			By("Create binding targeting the application but not the environment.")
			bindingStage := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingStage, k8sClient)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			By("Check binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Eventually(binding, "60s", "5s").Should(k8s.ExistByName(k8sClient))
		})

		It("Creates a SnapshotEnvironmentBinding if one exists that targets the environment, but NOT the application.", func() {
			By("Create binding targeting the environment but not the application.")
			bindingApp := buildSnapshotEnvironmentBindingResource("appx-prod-binding", "app-x", "prod", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingApp, k8sClient)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			By("Check binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Eventually(binding, "60s", "5s").Should(k8s.ExistByName(k8sClient))
		})

		It("Does not create a SnapshotEnvironmentBinding if one exists that targets the environment and the application.", func() {
			By("Create binding targeting the environment but not the application.")
			bindingProd := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "prod", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingProd, k8sClient)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			By("Check no binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Consistently(binding, "60s", "5s").ShouldNot(k8s.ExistByName(k8sClient))
		})
	})
})

func generatedBindingName(promotionRun *appstudiosharedv1.PromotionRun) string {
	return strings.ToLower(promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-generated-binding")
}

func buildComponentResource(name, componentName, appName string) appstudiosharedv1.Component {
	return appstudiosharedv1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ComponentSpec{
			ComponentName: componentName,
			Application:   appName,
			Source: appstudiosharedv1.ComponentSource{
				ComponentSourceUnion: appstudiosharedv1.ComponentSourceUnion{
					GitSource: &appstudiosharedv1.GitSource{
						URL: fixture.RepoURL,
					},
				},
			},
		},
	}
}
