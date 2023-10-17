package appproject

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HaveAppProjectSourceRepos checks the AppProject sourceRepos are equal to gitopsDeployment RepoURls
func HaveAppProjectSourceRepos(appProjectSpec appv1alpha1.AppProjectSpec) matcher.GomegaMatcher {

	return WithTransform(func(appProject *appv1alpha1.AppProject) bool {
		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(appProject), appProject)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := compareStringArraysInAnyOrder(appProject.Spec.SourceRepos, appProjectSpec.SourceRepos)

		fmt.Println("HaveAppProjectSourceRepos:", res, "/ Expected:", appProjectSpec.SourceRepos, "/ Actual:", appProject.Spec.SourceRepos)

		return res
	}, BeTrue())
}

// HaveAppProjectDestinations checks whether AppProject destinations refering to managedEnv
func HaveAppProjectDestinations(appProjectDestinations []appv1alpha1.ApplicationDestination) matcher.GomegaMatcher {
	return WithTransform(func(appProject *appv1alpha1.AppProject) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(appProject), appProject)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := reflect.DeepEqual(appProjectDestinations, appProject.Spec.Destinations)
		GinkgoWriter.Println("HaveAppProjectDestinations:", "expected: ", appProjectDestinations, "actual: ", appProject.Spec.Destinations)
		return res
	}, BeTrue())
}

func compareStringArraysInAnyOrder(arr1, arr2 []string) bool {
	if len(arr1) != len(arr2) {
		return false
	}

	for i := 0; i < len(arr1); i++ {

		match := false

		for j := 0; j < len(arr2); j++ {

			if arr1[i] == arr2[j] {
				match = true
				break
			}
		}

		if !match {
			return false
		}
	}

	return true
}
