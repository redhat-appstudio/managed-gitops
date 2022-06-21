package binding

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"sigs.k8s.io/controller-runtime/pkg/client"

	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

func HaveStatusGitOpsDeployments(gitOpsDeployments []appstudiosharedv1.BindingStatusGitOpsDeployment) matcher.GomegaMatcher {

	return WithTransform(func(binding appstudiosharedv1.ApplicationSnapshotEnvironmentBinding) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&binding), &binding)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := reflect.DeepEqual(gitOpsDeployments, binding.Status.GitOpsDeployments)

		fmt.Println("HaveStatusGitOpsDeployments:", res, "/ Expected:", gitOpsDeployments, "/ Actual:", binding.Status.GitOpsDeployments)

		return res
	}, BeTrue())
}
