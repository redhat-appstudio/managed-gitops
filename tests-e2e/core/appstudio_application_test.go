package core

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AppStudio Application Tests", func() {

	createAppAndComponent := func(skipAnnotationIsTrue bool) applicationv1alpha1.Application {

		asApp := &applicationv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: fixture.GitOpsServiceE2ENamespace,
			},
			Spec: applicationv1alpha1.ApplicationSpec{
				DisplayName: "name",
				// AppModelRepository: applicationv1alpha1.ApplicationGitRepository{},
				GitOpsRepository: applicationv1alpha1.ApplicationGitRepository{},
				Description:      "desc",
			},
		}

		asApp.Annotations = map[string]string{
			"skipGitOpsDeploymentCreation": fmt.Sprintf("%v", skipAnnotationIsTrue),
		}

		k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
		Expect(err).To(Succeed())

		err = k8s.Create(asApp, k8sClient)
		Expect(err).To(Succeed())

		asApp.Status = applicationv1alpha1.ApplicationStatus{
			Conditions: []metav1.Condition{},
			Devfile: `
metadata:
  attributes:
    appModelRepository.context: /
    appModelRepository.url: https://github.com/redhat-appstudio-appdata/red-tiger-pdave-organise-require
    gitOpsRepository.context: ./
    gitOpsRepository.url: https://github.com/redhat-appstudio-appdata/red-tiger-pdave-organise-require
  name: Red Tiger
projects:
- git:
    remotes:
      origin: https://github.com/devfile-samples/devfile-sample-java-springboot-basic.git
  name: basic-spring-boot
- git:
    remotes:
      origin: https://github.com/nodeshift-starters/devfile-sample.git
  name: basic-node-js

schemaVersion: 2.1.0`,
		}

		err = k8sClient.Status().Update(context.Background(), asApp)
		Expect(err).To(Succeed())

		asComponent := &applicationv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-component",
				Namespace: fixture.GitOpsServiceE2ENamespace,
			},
			Spec: applicationv1alpha1.ComponentSpec{
				ComponentName:  "basic-spring-boot",
				Application:    asApp.Name,
				ContainerImage: "quay.io/redhat-appstudio/user-workload:pdave-basic-spring-boot",
				Source: applicationv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationv1alpha1.ComponentSourceUnion{
						GitSource: &applicationv1alpha1.GitSource{
							URL: "https://github.com/devfile-samples/devfile-sample-java-springboot-basic.git",
						},
					},
				},
			},
			Status: applicationv1alpha1.ComponentStatus{
				ContainerImage: "quay.io/redhat-appstudio/user-workload:pdave-basic-spring-boot",
				GitOps: applicationv1alpha1.GitOpsStatus{
					RepositoryURL: "https://github.com/redhat-appstudio-appdata/red-tiger-pdave-organise-require",
					// Branch: "",
					Context: "./",
					// ResourceGenerationSkipped: "",
					// CommitID: "",
				},
			},
		}

		err = k8s.Create(asComponent, k8sClient)
		Expect(err).To(Succeed())

		return *asApp
	}

	Context("Test the appstudio-controller Application controller", func() {

		DescribeTable("Verify skip annotation on Application works as expected", func(annotationValue bool) {

			// TODO: GITOPSRVCE-373: remove this test once this behaviour is not required anymore
			Skip("Skipping this tests as this functionality is removed and will remove this test completely once the functionality is completely removed from the ApplicationReconciler")

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			asApp := createAppAndComponent(annotationValue)

			gitOpsDeploymentResource := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      asApp.Name + "-deployment",
					Namespace: asApp.Namespace,
				},
			}

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if annotationValue {

				Consistently(gitOpsDeploymentResource, "30s", "1s").ShouldNot(
					SatisfyAll(
						k8s.ExistByName(k8sClient),
					),
				)

			} else {

				Eventually(gitOpsDeploymentResource, "1m", "1s").Should(
					SatisfyAll(
						k8s.ExistByName(k8sClient),
					),
				)
			}

		},
			Entry("skip annotation is true, so gitopsdepl should not be created", true),
			Entry("skip annotation is false, so gitopsdepl should not be created", false))

	})
})
