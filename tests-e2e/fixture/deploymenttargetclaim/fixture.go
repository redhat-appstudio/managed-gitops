package deploymenttargetclaim

import (
	"context"
	"fmt"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasStatusPhase checks if the give DeploymentTargetClaim is in a given phase.
func HasStatusPhase(phase appstudiosharedv1.DeploymentTargetClaimPhase) matcher.GomegaMatcher {
	return WithTransform(func(dtc appstudiosharedv1.DeploymentTargetClaim) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&dtc), &dtc)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		if dtc.Status.Phase != phase {
			fmt.Printf("Phase mismatch for DTC %s: Expected: %s, Actual: %s\n", dtc.Name, phase, dtc.Status.Phase)
			return false
		}

		return true
	}, BeTrue())
}

// HasAnnotation checks if the DeploymentTargetClaim has a given annotation.
func HasAnnotation(key, value string) matcher.GomegaMatcher {
	return WithTransform(func(dtc appstudiosharedv1.DeploymentTargetClaim) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&dtc), &dtc)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		annotations := dtc.GetAnnotations()
		if annotations == nil {
			fmt.Printf("Annotation %s not found in DTC %s\n", key, dtc.Name)
			return false
		}

		v, found := annotations[key]
		if !found {
			fmt.Printf("Annotation %s not found in DTC %s\n", key, dtc.Name)
			return false
		}

		if v != value {
			fmt.Printf("Annotation value mismatch for DTC %s: Expected: %s Actual %s\n", dtc.Name, value, v)
			return false
		}

		return true
	}, BeTrue())
}

// HasANumberOfMatchingSpaceRequests checks if the DeploymentTargetClaim has the exact number of matching SpaceRequests.
func HasANumberOfMatchingSpaceRequests(num int) matcher.GomegaMatcher {
	return WithTransform(func(dtc appstudiosharedv1.DeploymentTargetClaim) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&dtc), &dtc)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		spaceRequestList := codereadytoolchainv1alpha1.SpaceRequestList{}
		opts := []client.ListOption{
			client.InNamespace(dtc.Namespace),
			client.MatchingLabels{
				"appstudio.openshift.io/dtc": dtc.Name,
			},
		}

		err = k8sClient.List(context.Background(), &spaceRequestList, opts...)
		if err != nil {
			fmt.Println(k8sFixture.K8sClientError, err)
			return false
		}

		if len(spaceRequestList.Items) != num {
			fmt.Println("HasAMatchingSpaceRequest resources found mismatch: ", false, "Expected: ", num, "Actual: ", len(spaceRequestList.Items))
			return false
		}

		return true
	}, BeTrue())
}
