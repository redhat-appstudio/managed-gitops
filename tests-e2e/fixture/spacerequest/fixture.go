package spacerequest

import (
	"context"
	"fmt"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasStatus checks if the give SpaceRequest is in a given status.
func HasStatus(status corev1.ConditionStatus) matcher.GomegaMatcher {
	return WithTransform(func(spacerequest codereadytoolchainv1alpha1.SpaceRequest) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&spacerequest), &spacerequest)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}
		if !condition.IsTrue(spacerequest.Status.Conditions, codereadytoolchainv1alpha1.ConditionReady) {
			fmt.Printf("Status mismatch for SpaceRequest %s: Expected: %s \n", spacerequest.Name, status)
			return false
		}

		return true
	}, BeTrue())
}

// HasANumberOfMatchingDTs checks if the SpaceRequest has the exact number of matching DeploymentTarget.
func HasANumberOfMatchingDTs(num int) matcher.GomegaMatcher {
	return WithTransform(func(spacerequest codereadytoolchainv1alpha1.SpaceRequest) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&spacerequest), &spacerequest)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		dtList := appstudiosharedv1.DeploymentTargetList{}
		opts := []client.ListOption{
			client.InNamespace(spacerequest.Namespace),
		}

		err = k8sClient.List(context.Background(), &dtList, opts...)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		var count int
		count = 0
		if len(dtList.Items) > 0 {
			for _, d := range dtList.Items {
				if string(d.Spec.ClaimRef) == spacerequest.Labels["appstudio.openshift.io/dtc"] {
					count = count + 1
				}
			}
		}

		if count != num {
			fmt.Println("HasAMatchingSpaceRequest resources found mismatch: ", false, "Expected: ", num, "Actual: ", count)
			return false
		}

		return true
	}, BeTrue())
}
