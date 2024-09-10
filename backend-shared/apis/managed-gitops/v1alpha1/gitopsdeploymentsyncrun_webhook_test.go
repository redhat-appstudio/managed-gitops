package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("GitOpsDeploymentSyncRun validation webhook", func() {
	var namespace *corev1.Namespace
	var gitopsDeplSyncRunCr *GitOpsDeploymentSyncRun
	var ctx context.Context

	BeforeEach(func() {

		ctx = context.Background()

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-3",
				UID:  uuid.NewUUID(),
			},
			Spec: corev1.NamespaceSpec{},
		}

		gitopsDeplSyncRunCr = &GitOpsDeploymentSyncRun{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				UID:       uuid.NewUUID(),
			},
			Spec: GitOpsDeploymentSyncRunSpec{
				GitopsDeploymentName: "test-app",
				RevisionID:           "HEAD",
			},
		}

	})

	Context("Create GitOpsDeploymentSyncRun CR with invalid name", func() {
		It("Should fail with error saying name should not be zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz", func() {

			// TODO: Re-enable webhook tests
			Skip("webhook ports are conflicting")

			err := k8sClient.Create(ctx, namespace)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeplSyncRunCr.Name = invalid_name
			err = k8sClient.Create(ctx, gitopsDeplSyncRunCr)

			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring(error_invalid_name))

		})
	})

})
