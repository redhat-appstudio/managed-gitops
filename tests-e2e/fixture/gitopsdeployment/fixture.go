package gitopsdeployment

import (
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
)

// HaveLabel will succeed if the GitOpsDeployment contains the specified label, and fail otherwise.
func HaveLabel(key, value string) matcher.GomegaMatcher {

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

		// Look for the label on the GitOpsDeployment
		for gopsLabelKey, gopsLabelValue := range gitopsDepl.Labels {
			if gopsLabelKey == key && gopsLabelValue == value {
				return true
			}
		}

		fmt.Println("Label not found on '" + gitopsDepl.Name + "': key '" + key + "' with value '" + value + "'")

		return false
	}, BeTrue())

}

// HaveHealthStatusCode waits for the given GitOpsDeployment to have the expected Health status (e.g. "Healthy"/"Unhealthy").
//
// This indicates whether the GitOpsDeployment (based on the Argo CD Application) is 'healthy',
// that is, all resources are working as expected by Argo CD's definition.
func HaveHealthStatusCode(status managedgitopsv1alpha1.HealthStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {

		return HaveHealthStatusCodeFunc(status, gitopsDepl)

	}, BeTrue())
}

// HaveHealthStatusCodeFunc can be called for non-Gomega-based comparisons. See description above.
// For the vast majority of tests, you should not (need to) use this function.
func HaveHealthStatusCodeFunc(status managedgitopsv1alpha1.HealthStatusCode, gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {
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

}

// HaveSyncStatusCode waits for the given GitOpsDeployment to have the expected Sync status (e.g. "Unknown"/"Synced"/"OutOfSync")
//
// This value indicates whether the K8s resources defined in the GitOps repository are equal to (in sync with) the resources
// on the target cluster, according to Argo CD.
func HaveSyncStatusCode(status managedgitopsv1alpha1.SyncStatusCode) matcher.GomegaMatcher {

	return WithTransform(func(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {
		return HaveSyncStatusCodeFunc(status, gitopsDepl)
	}, BeTrue())
}

// HaveSyncStatusCodeFunc can be called for non-Gomega-based comparisons. See description above.
// For the vast majority of tests, you should not (need to) use this function.
func HaveSyncStatusCodeFunc(status managedgitopsv1alpha1.SyncStatusCode, gitopsDepl managedgitopsv1alpha1.GitOpsDeployment) bool {
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

}

// HaveResources checks if the .status.resources field of GitOpsDeployment have the required resources
func HaveResources(resourceStatusList []managedgitopsv1alpha1.ResourceStatus) matcher.GomegaMatcher {
	return WithTransform(func(gitopsDeployment managedgitopsv1alpha1.GitOpsDeployment) bool {

		return HaveResourcesFunc(resourceStatusList, gitopsDeployment)

	}, BeTrue())
}

// HaveResourcesFunc can be called for non-Gomega-based comparisons. See description above.
// For the vast majority of tests, you should not (need to) use this function.
func HaveResourcesFunc(resourceStatusList []managedgitopsv1alpha1.ResourceStatus, gitopsDeployment managedgitopsv1alpha1.GitOpsDeployment) bool {
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

// HaveReconciledState checks the GitOpsDeployment resource's '.status.reconciledState' field, to ensure the expected value matches the actual value.
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

// UpdateDeploymentWithFunction will update the GitOpsDeployment by:
// - retrieving the latest version of the GitOpsDeployment from the cluster
// - applying the mutation function
// - calling update to update the cluster object with the mutated version of the object.
// On fail, it will be reattempted until it succeeds.
func UpdateDeploymentWithFunction(gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment,
	mutationFn func(*managedgitopsv1alpha1.GitOpsDeployment)) error {

	GinkgoWriter.Printf("Updating GitOpsDeployment for '%v'\n", gitopsDeployment.ObjectMeta)
	config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
	Expect(err).To(BeNil())

	k8sClient, err := fixture.GetKubeClient(config)
	if err != nil {
		fmt.Println(k8sFixture.K8sClientError, err)
		return err
	}

	return k8sFixture.UntilSuccess(k8sClient, func(k8sClient client.Client) error {

		// Retrieve the latest version of the GitOpsDeployment resource
		err := k8sFixture.Get(gitopsDeployment, k8sClient)
		if err != nil {
			return err
		}

		// Call the mutation function, to change the deployment
		mutationFn(gitopsDeployment)

		// Attempt to update the object with the change made by the mutation function
		err = k8sFixture.Update(gitopsDeployment, k8sClient)

		// Report back the error, if we hit one
		return err
	})

}

// UpdateDeploymentStatusWithFunction works like UpdateDeploymentWithFunction, but updates the
// status field of the GitOpsDeployment.
func UpdateDeploymentStatusWithFunction(gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment,
	mutationFn func(*managedgitopsv1alpha1.GitOpsDeployment)) error {

	GinkgoWriter.Printf("Updating GitOpsDeployment status for '%v'\n", gitopsDeployment.ObjectMeta)
	config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
	Expect(err).To(BeNil())

	k8sClient, err := fixture.GetKubeClient(config)
	if err != nil {
		fmt.Println(k8sFixture.K8sClientError, err)
		return err
	}

	return k8sFixture.UntilSuccess(k8sClient, func(k8sClient client.Client) error {

		// Retrieve the latest version of the GitOpsDeployment resource
		err := k8sFixture.Get(gitopsDeployment, k8sClient)
		if err != nil {
			return err
		}

		// Call the mutation function, to change the deployment
		mutationFn(gitopsDeployment)

		// Attempt to update the object with the change made by the mutation function
		err = k8sFixture.UpdateStatus(gitopsDeployment, k8sClient)

		// Report back the error, if we hit one
		return err
	})

}

// BuildGitOpsDeploymentResource builds a GitOpsDeployment with 'opinionated' default values, which is self-contained to
// the GitGitOpsServiceE2ENamespace. This makes it easy to clean up after tests using EnsureCleanSlate.
// - Defaults to creation in GitOpsServiceE2ENamespace
// - Defaults to deployment of K8s resources to GitOpsServiceE2ENamespace
func BuildGitOpsDeploymentResource(name, repoURL, path, deploymentSpecType string) managedgitopsv1alpha1.GitOpsDeployment {

	gitOpsDeploymentResource := managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
			Source: managedgitopsv1alpha1.ApplicationSource{
				RepoURL: repoURL,
				Path:    path,
			},
			Destination: managedgitopsv1alpha1.ApplicationDestination{},
			Type:        deploymentSpecType,
		},
	}

	return gitOpsDeploymentResource
}

func BuildTargetRevisionGitOpsDeploymentResource(name, repoURL, path, target, deploymentSpecType string) managedgitopsv1alpha1.GitOpsDeployment {
	gitOpsDeploymentResource := BuildGitOpsDeploymentResource(name, repoURL, path, deploymentSpecType)
	gitOpsDeploymentResource.Spec.Source.TargetRevision = target
	return gitOpsDeploymentResource
}
