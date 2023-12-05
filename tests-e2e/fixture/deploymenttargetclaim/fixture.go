package deploymenttargetclaim

import (
	"context"
	"fmt"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"

	. "github.com/onsi/gomega"
	matcher "github.com/onsi/gomega/types"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	k8sFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasStatusPhase checks if the give DeploymentTargetClaim is in a given phase.
func HasStatusPhase(phase appstudiosharedv1.DeploymentTargetClaimPhase) matcher.GomegaMatcher {
	return WithTransform(func(dtc appstudiosharedv1.DeploymentTargetClaim) bool {
		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

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
				appstudiocontrollers.DeploymentTargetClaimLabel: dtc.Name,
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

// BuildDeploymentTargetAndDeploymentTargetClaim creates an instance of DeploymentTarget and DeploymentTargetClaim
func BuildDeploymentTargetAndDeploymentTargetClaim(kubeConfigContents, apiServerURL, secretName, secretNamespace, defaultNamespace, dtName, dtcName, dtcClassName string, allowInsecureSkipTLSVerify bool) (appstudiosharedv1.DeploymentTarget, appstudiosharedv1.DeploymentTargetClaim) {

	dt := appstudiosharedv1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dtName,
			Namespace: secretNamespace,
		},
		Spec: appstudiosharedv1.DeploymentTargetSpec{
			DeploymentTargetClassName: "test-class",
			KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
				APIURL:                     apiServerURL,
				ClusterCredentialsSecret:   secretName,
				DefaultNamespace:           defaultNamespace,
				AllowInsecureSkipTLSVerify: allowInsecureSkipTLSVerify,
			},
		},
	}

	dtc := appstudiosharedv1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dtcName,
			Namespace: dt.Namespace,
		},
		Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
			TargetName:                dt.Name,
			DeploymentTargetClassName: dt.Spec.DeploymentTargetClassName,
		},
	}
	return dt, dtc
}

func HaveDeploymentTargetClaimCondition(expected metav1.Condition) matcher.GomegaMatcher {
	return WithTransform(func(dtc appstudiosharedv1.DeploymentTargetClaim) bool {

		config, err := fixture.GetE2ETestUserWorkspaceKubeConfig()
		Expect(err).ToNot(HaveOccurred())

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

		if len(dtc.Status.Conditions) == 0 {
			fmt.Println("HaveDeploymentTargetClaimCondition: DeploymentTargetClaimCondition is nil")
			return false
		}
		actual := dtc.Status.Conditions[0]
		fmt.Println("HaveDeploymentTargetClaimCondition:", "expected: ", expected, "actual: ", actual)
		return actual.Type == expected.Type &&
			actual.Status == expected.Status &&
			actual.Reason == expected.Reason &&
			actual.Message == expected.Message

	}, BeTrue())
}
