package gitopsdeploymenrepositorycredential

import (
	"context"
	"fmt"
	"pkg/mod/sigs.k8s.io/controller-runtime@v0.11.0/pkg/client"
	"reflect"

	"github.com/managed-services/managed-gitops/tests-e2e/fixture"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HaveConditions will return a matcher that will check whether a GitOpsDeployment has the expected conditons.
// - When comparing conditions, it will ignore the LastProbeTime/LastTransitionTime fields.
func HaveConditions(conditions []*metav1.Condition) matcher.GomegaMatcher {

	// sanitizeCondition removes ephemeral fields from the GitOpsDeploymentCondition which should not be compared using
	// reflect.DeepEqual
	sanitizeCondition := func(cond *metav1.Condition) metav1.Condition {

		res := metav1.Condition{
			Type:   cond.Type,
			Status: cond.Status,
			Reason: cond.Reason,
		}

		return res

	}

	return WithTransform(func(gitopsDeploymentRepositoryCredentialCR managedgitopsv1alpha1.GitopsDeploymentRepositoryCredential) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDeploymentRepositoryCredentialCR), &gitopsDeploymentRepositoryCredentialCR)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		conditionExists := false
		existingConditionList := gitopsDeploymentRepositoryCredentialCR.Status.Conditions

		if len(conditions) != len(existingConditionList) {
			fmt.Println("HaveConditions:", conditionExists, "/ Expected:", conditions, "/ Actual:", gitopsDeploymentRepositoryCredentialCR.Status.Conditions)
			return false
		}

		for _, resourceCondition := range conditions {
			conditionExists = false
			for _, existingCondition := range existingConditionList {
				if reflect.DeepEqual(sanitizeCondition(resourceCondition), sanitizeCondition(existingCondition)) {
					conditionExists = true
					break
				}
			}
			if !conditionExists {
				fmt.Println("GitOpsDeploymentCondition:", conditionExists, "/ Expected:", conditions, "/ Actual:", gitopsDeploymentRepositoryCredentialCR.Status.Conditions)
				break
			}
		}
		return conditionExists

	}, BeTrue())

}
