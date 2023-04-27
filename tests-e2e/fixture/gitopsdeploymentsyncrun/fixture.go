package gitopsdeploymentsyncrun

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

func HaveConditions(conditions []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition) types.GomegaMatcher {
	return WithTransform(func(syncRun managedgitopsv1alpha1.GitOpsDeploymentSyncRun) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&syncRun), &syncRun)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		actualConditions := syncRun.Status.Conditions

		if len(conditions) != len(syncRun.Status.Conditions) {
			fmt.Println("HaveConditions number mismatch: ", false, "Expected: ", conditions, "Actual: ", actualConditions)
			return false
		}

		sanitizeCondition := func(cond managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition) managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition {
			return managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
				Type:    cond.Type,
				Message: cond.Message,
				Status:  cond.Status,
				Reason:  cond.Reason,
			}
		}

		for i := 0; i < len(conditions); i++ {
			if !reflect.DeepEqual(sanitizeCondition(conditions[i]), sanitizeCondition(actualConditions[i])) {
				fmt.Println("HaveConditions: ", false, "Expected: ", conditions[i], "Actual: ", actualConditions[i])
				return false
			}
		}

		return true

	}, BeTrue())
}

func BuildGitOpsDeploymentSyncRunResource(name, ns, deplyName, revision string) managedgitopsv1alpha1.GitOpsDeploymentSyncRun {
	return managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
			GitopsDeploymentName: deplyName,
			RevisionID:           revision,
		},
	}
}
