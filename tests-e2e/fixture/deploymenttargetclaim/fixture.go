package deploymenttargetclaim

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
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
			fmt.Printf("Phase mismatch for DTC %s: Expected: %s Actual %s\n", dtc.Name, phase, dtc.Status.Phase)
			return false
		}

		return true
	}, BeTrue())
}

// HasAnnotation checks if the DeploymentTargetClaim has a given annotation.
func HaveAnnotation(key, value string) matcher.GomegaMatcher {
	return WithTransform(func(dtc appstudiosharedv1.DeploymentTargetClaim) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).To(BeNil())

		k8sClient, err := fixture.GetKubeClient(config)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&dtc), &dtc)
		if err != nil {
			fmt.Println(k8s.K8sClientError, err)
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