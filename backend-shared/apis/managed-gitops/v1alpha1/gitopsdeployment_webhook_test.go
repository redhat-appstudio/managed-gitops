package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("GitOpsDeployment validation webhook", func() {

	Context("Create GitOpsDeployment CR with .spec.Type field as unknown", func() {
		It("Should fail with error saying spec type must be manual or automated", func() {
			ctx := context.Background()

			gitopsDepl := &GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: "test-namespace",
				},
				Spec: GitOpsDeploymentSpec{
					Source: ApplicationSource{},
					Type:   "unknown",
				},
			}

			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring("spec type must be manual or automated"))
		})
	})

})
