package application

import (
	"fmt"

	. "github.com/onsi/gomega"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	matcher "github.com/onsi/gomega/types"

	"context"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HaveAutomatedSyncPolicy checks if the given Application have automated sync policy with prune, selfHeal and allowEmpty.
func HaveAutomatedSyncPolicy(syncPolicy appv1alpha1.SyncPolicyAutomated) matcher.GomegaMatcher {

	return WithTransform(func(app appv1alpha1.Application) bool {

		k8sClient, err := fixture.GetKubeClient()
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
