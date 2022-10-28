package application

import (
	"fmt"
	"reflect"

	. "github.com/onsi/gomega"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This is intentionally NOT exported, for now. Create another function in this file/package that calls this function, and export that.
func expectedCondition(f func(app appv1alpha1.Application) bool) matcher.GomegaMatcher {

	return WithTransform(func(app appv1alpha1.Application) bool {

		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		return f(app)

	}, BeTrue())

}

func HasDestinationField(expectedDestination appv1alpha1.ApplicationDestination) matcher.GomegaMatcher {

	return expectedCondition(func(app appv1alpha1.Application) bool {

		fmt.Println("Current destination for", app.Name, "is", app.Spec.Destination)

		return app.Spec.Destination.Equals(expectedDestination)
	})
}

// HaveAutomatedSyncPolicy checks if the given Application have automated sync policy with prune, selfHeal and allowEmpty.
func HaveAutomatedSyncPolicy(syncPolicy appv1alpha1.SyncPolicyAutomated) matcher.GomegaMatcher {

	return WithTransform(func(app appv1alpha1.Application) bool {

		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		syncPolicy := app.Spec.SyncPolicy
		syncPolicyEnabled := false
		if syncPolicy != nil && syncPolicy.Automated != nil {
			syncPolicyEnabled = syncPolicy.Automated.AllowEmpty && syncPolicy.Automated.Prune && syncPolicy.Automated.SelfHeal
		}
		fmt.Println("HaveAutomatedSyncPolicy:", syncPolicyEnabled, "/ Expected:", syncPolicy, "/ Actual:", app.Spec.SyncPolicy.Automated)

		return syncPolicyEnabled
	}, BeTrue())
}

// HaveHealthStatusCode waits for Argo CD Application to have the given health
func HaveHealthStatusCode(status appv1alpha1.ApplicationStatus) matcher.GomegaMatcher {

	return WithTransform(func(app appv1alpha1.Application) bool {
		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status.Health.Status == app.Status.Health.Status

		fmt.Println("HaveHealthStatusCode:", res, "/ Expected:", status, "/ Actual:", app.Status.Health.Status)

		return res
	}, BeTrue())
}

// HaveSyncStatusCode waits for Argo CD to have the given sync status
func HaveSyncStatusCode(status appv1alpha1.ApplicationStatus) matcher.GomegaMatcher {

	return WithTransform(func(app appv1alpha1.Application) bool {
		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		res := status.Sync.Status == app.Status.Sync.Status
		fmt.Println("HaveSyncStatusCode:", res, "/ Expected:", status, "/ Actual:", app.Status.Sync.Status)

		return res
	}, BeTrue())
}

//  HaveApplicationSyncError checks the Application .status.conditions.Message fiels is set with syncError.
func HaveApplicationSyncError(syncError appv1alpha1.ApplicationStatus) matcher.GomegaMatcher {

	return WithTransform(func(app appv1alpha1.Application) bool {
		config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		var existingConditionMessage string
		var res bool

		for _, msg := range app.Status.Conditions {
			if msg.Type == appv1alpha1.ApplicationConditionSyncError {
				existingConditionMessage = msg.Message
			}
		}

		for _, syncError := range syncError.Conditions {
			res = reflect.DeepEqual(syncError.Message, existingConditionMessage)
			fmt.Println("HaveApplicationSyncError:", res, "/ Expected:", syncError, "/ Actual:", existingConditionMessage)
		}

		return res
	}, BeTrue())
}
