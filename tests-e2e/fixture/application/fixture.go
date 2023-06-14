package application

import (
	"fmt"
	"reflect"

	. "github.com/onsi/gomega"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	matcher "github.com/onsi/gomega/types"

	"context"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// DeleteArgoCDApplication deletes an Argo CD Application, but first removes the finalizer if present
func DeleteArgoCDApplication(argocdAppName string, argoCDNamespace string) error {

	config, err := fixture.GetServiceProviderWorkspaceKubeConfig()
	Expect(err).To(BeNil())

	k8sClient, err := fixture.GetKubeClient(config)
	if err != nil {
		return err
	}

	argoCDApplication := appv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      argocdAppName,
			Namespace: argoCDNamespace,
		},
	}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&argoCDApplication), &argoCDApplication); err != nil {

		if apierr.IsNotFound(err) {
			// No work needed.
			return nil
		}

		return err
	}

	if len(argoCDApplication.Finalizers) > 0 {
		argoCDApplication.Finalizers = []string{}
		err := k8sClient.Update(context.Background(), &argoCDApplication)
		Expect(err).To(BeNil())
	}

	if err := k8sClient.Delete(context.Background(), &argoCDApplication); err != nil {
		if apierr.IsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}

func HaveStatusConditionMessage(expectedMessage string) matcher.GomegaMatcher {
	return expectedCondition(func(app appv1alpha1.Application) bool {
		for i := range app.Status.Conditions {
			if app.Status.Conditions[i].Message == expectedMessage {
				return true
			}
		}
		return false
	})
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

func HaveSyncOption(expectedSyncOption string) matcher.GomegaMatcher {

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

		isSyncOption := false
		if app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.SyncOptions != nil {
			isSyncOption = app.Spec.SyncPolicy.SyncOptions.HasOption(expectedSyncOption)
		}

		fmt.Println("HaveSyncOption:", expectedSyncOption, "/ Expected:", expectedSyncOption, "/ Actual:", isSyncOption)

		return isSyncOption
	}, BeTrue())
}

func HaveRetryOption(expectedRetryOption *appv1alpha1.RetryStrategy) matcher.GomegaMatcher {

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

		res := false
		if app.Spec.SyncPolicy != nil && app.Spec.SyncPolicy.Retry != nil {
			res = reflect.DeepEqual(expectedRetryOption, app.Spec.SyncPolicy.Retry)
		}

		fmt.Println("HaveRetry:", app.Spec.SyncPolicy.Retry, "/ Expected:", expectedRetryOption, "/ Actual:", res)

		return res
	}, BeTrue())
}

func HaveHealthStatusCode(expectedHealth health.HealthStatusCode) matcher.GomegaMatcher {

	return expectedCondition(func(app appv1alpha1.Application) bool {

		fmt.Println("HaveHealthStatusCode - current health:", app.Status.Health.Status, " / expected health:", expectedHealth)

		return app.Status.Health.Status == expectedHealth

	})

}

// HaveSyncStatusCode waits for Argo CD to have the given sync status
func HaveSyncStatusCode(expected appv1alpha1.SyncStatusCode) matcher.GomegaMatcher {

	return expectedCondition(func(app appv1alpha1.Application) bool {

		fmt.Println("HaveSyncStatusCode - current syncStatusCode:", app.Status.Sync.Status, " / expected syncStatusCode:", expected)

		return app.Status.Sync.Status == expected

	})

}

// HaveOperationState checks if the Application has the given OperationState
func HaveOperationState(opState appv1alpha1.OperationState) matcher.GomegaMatcher {

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

		if app.Status.OperationState == nil {
			return false
		}

		res := app.Status.OperationState.Phase == opState.Phase && app.Status.OperationState.Message == opState.Message
		fmt.Println("HaveOperationState:", res, "/ ExpectedOperationPhase:", opState.Phase, "/ ActualOperationPhase:", app.Status.OperationState.Phase, "/ ExpectedMessage:", opState.Message, "/ ActualMessage:", app.Status.OperationState.Message)

		return res
	}, BeTrue())
}

// HaveApplicationSyncError checks the Application .status.conditions.Message fiels is set with syncError.
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
