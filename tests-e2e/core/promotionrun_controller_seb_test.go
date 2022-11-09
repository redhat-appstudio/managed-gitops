package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

var _ = Describe("Promotion Run Creation of SnapshotEnvironmentBinding E2E Tests.", func() {
	Context("Testing Promotion Run Creation of SnapshotEnvironmentBinding.", func() {
		var environmentProd appstudiosharedv1.Environment
		var promotionRun appstudiosharedv1.PromotionRun

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("Create Staging Environment.")
			environmentStage := buildEnvironmentResource("staging", "Staging Environment", "staging", appstudiosharedv1.EnvironmentType_POC)
			err := k8s.Create(&environmentStage)
			Expect(err).To(Succeed())

			By("Create Production Environment.")
			environmentProd = buildEnvironmentResource("prod", "Production Environment", "prod", appstudiosharedv1.EnvironmentType_POC)
			err = k8s.Create(&environmentProd)
			Expect(err).To(Succeed())

			By("Create Snapshot.")
			snapshot := buildSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&snapshot)
			Expect(err).To(Succeed())

			By("Create Component.")
			component := buildComponentResource("comp1", "component-a", "new-demo-app")
			err = k8s.Create(&component)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			promotionRun = buildPromotionRunResource("new-demo-app-manual-promotion", "new-demo-app", "my-snapshot", "prod")
		})

		It("Creates a SnapshotEnvironmentBinding if one doesn't exist that targets the application/environment.", func() {
			By("Create PromotionRun CR.")
			err := k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			By("Check binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Eventually(binding, "60s", "5s").Should(k8s.ExistByName())
		})

		It("Creates a SnapshotEnvironmentBinding if one exists that targets the application, but NOT the environment.", func() {
			By("Create binding targeting the application but not the environment.")
			bindingStage := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&bindingStage)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			By("Check binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Eventually(binding, "60s", "5s").Should(k8s.ExistByName())
		})

		It("Creates a SnapshotEnvironmentBinding if one exists that targets the environment, but NOT the application.", func() {
			By("Create binding targeting the environment but not the application.")
			bindingApp := buildSnapshotEnvironmentBindingResource("appx-prod-binding", "app-x", "prod", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&bindingApp)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			By("Check binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Eventually(binding, "60s", "5s").Should(k8s.ExistByName())
		})

		It("Does not create a SnapshotEnvironmentBinding if one exists that targets the environment and the application.", func() {
			By("Create binding targeting the environment but not the application.")
			bindingProd := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "prod", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&bindingProd)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			By("Check no binding was created.")
			bindingName := generatedBindingName(&promotionRun)
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Name:      bindingName,
					Namespace: promotionRun.Namespace,
				},
			}
			Consistently(binding, "60s", "5s").ShouldNot(k8s.ExistByName())
		})
	})
})

func generatedBindingName(promotionRun *appstudiosharedv1.PromotionRun) string {
	return strings.ToLower(promotionRun.Spec.Application + "-" + promotionRun.Spec.ManualPromotion.TargetEnvironment + "-generated-binding")
}

func buildComponentResource(name, componentName, appName string) applicationv1alpha1.Component {
	return applicationv1alpha1.Component{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: applicationv1alpha1.ComponentSpec{
			ComponentName: componentName,
			Application:   appName,
		},
	}
}
