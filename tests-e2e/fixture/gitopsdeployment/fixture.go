package gitopsdeployment

import (
	"fmt"
	"reflect"

	. "github.com/onsi/gomega"

	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
)

// HaveHealthStatusCode waits for the given GitOpsDeployment to have the expected Health status (e.g. "Healthy"/"Unhealthy").
//
// This indicates whether the GitOpsDeployment (based on the Argo CD Application) is 'healthy',
// that is, all resources are working as expected by Argo CD's definition.
func HaveHealthStatusCode(status managedgitopsv1alpha1.HealthStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status == gitopsDepl.Status.Health.Status

		fmt.Println("HaveHealthStatusCode:", res, "/ Expected:", status, "/ Actual:", gitopsDepl.Status.Health.Status)

		return res
	}, BeTrue())
}

// HaveSyncStatusCode waits for the given GitOpsDeployment to have the expected Sync status (e.g. "Unknown"/"Synced"/"OutOfSync")
//
// This value indicates whether the K8s resources defined in the GitOps repository are equal to (in sync with) the resources
// on the target cluster, according to Argo CD.
func HaveSyncStatusCode(status managedgitopsv1alpha1.SyncStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status == gitopsDepl.Status.Sync.Status
		fmt.Println("HaveSyncStatusCode:", res, "/ Expected:", status, "/ Actual:", gitopsDepl.Status.Sync.Status)

		return res
	}, BeTrue())
}

// HaveResources checks if the .status.resources field of GitOpsDeployment have the required resources
func HaveResources(resourceStatusList []managedgitopsv1alpha1.ResourceStatus) matcher.GomegaMatcher {
	return WithTransform(func(gitopsDeployment managedgitopsv1alpha1.GitOpsDeployment) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDeployment), &gitopsDeployment)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		// compare the slices irrespective of their order
		resourceExists := false
		existingResourceStatusList := gitopsDeployment.Status.Resources

		if len(resourceStatusList) != len(existingResourceStatusList) {
			fmt.Println("HaveResources:", resourceExists, "/ Expected:", resourceStatusList, "/ Actual:", gitopsDeployment.Status.Resources)
			return false
		}

		for _, resourceStatus := range resourceStatusList {
			resourceExists = false
			for _, existingResourceStatus := range existingResourceStatusList {
				if reflect.DeepEqual(resourceStatus, existingResourceStatus) {
					resourceExists = true
					break
				}
			}
			if !resourceExists {
				fmt.Println("HaveResources:", resourceExists, "/ Expected:", resourceStatusList, "/ Actual:", gitopsDeployment.Status.Resources)
				fmt.Println("- missing: ", resourceStatus)
				break
			}
		}
		return resourceExists

	}, BeTrue())
}

func HaveSpecSource(source managedgitopsv1alpha1.ApplicationSource) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := reflect.DeepEqual(source, gitopsDepl.Spec.Source)
		fmt.Println("HaveSource:", res, "/ Expected:", source, "/ Actual:", gitopsDepl.Spec.Source)

		return res
	}, BeTrue())
}

// HaveConditions will return a matcher that will check whether a GitOpsDeployment has the expected conditons.
// - When comparing conditions, it will ignore the LastProbeTime/LastTransitionTime fields.
func HaveConditions(conditions []managedgitopsv1alpha1.GitOpsDeploymentCondition) matcher.GomegaMatcher {

	// sanitizeCondition removes ephemeral fields from the GitOpsDeploymentCondition which should not be compared using
	// reflect.DeepEqual
	sanitizeCondition := func(cond managedgitopsv1alpha1.GitOpsDeploymentCondition) managedgitopsv1alpha1.GitOpsDeploymentCondition {

		res := managedgitopsv1alpha1.GitOpsDeploymentCondition{
			Type:    cond.Type,
			Message: cond.Message,
			Status:  cond.Status,
			Reason:  cond.Reason,
		}

		return res

	}

	return WithTransform(func(gitopsDeployment managedgitopsv1alpha1.GitOpsDeployment) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDeployment), &gitopsDeployment)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		conditionExists := false
		existingConditionList := gitopsDeployment.Status.Conditions

		if len(conditions) != len(existingConditionList) {
			fmt.Println("HaveConditions:", conditionExists, "/ Expected:", conditions, "/ Actual:", gitopsDeployment.Status.Conditions)
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
				fmt.Println("GitOpsDeploymentCondition:", conditionExists, "/ Expected:", conditions, "/ Actual:", gitopsDeployment.Status.Conditions)
				break
			}
		}
		return conditionExists

	}, BeTrue())

}

//  HaveReconciledState checks the GitOpsDeployment resource's '.status.reconciledState' field, to ensure the expected value matches the actual value.
func HaveReconciledState(reconciledState managedgitopsv1alpha1.ReconciledState) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&gitopsDepl), &gitopsDepl)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := reflect.DeepEqual(reconciledState, gitopsDepl.Status.ReconciledState)
		fmt.Println("HaveReconciledState:", res, "/ Expected:", reconciledState, "/ Actual:", gitopsDepl.Status.ReconciledState)

		return res
	}, BeTrue())
}
