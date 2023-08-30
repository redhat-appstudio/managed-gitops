package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("GitOpsDeploymentManagedEnvironment validation webhook", func() {
	var namespace *corev1.Namespace
	var managedEnv *GitOpsDeploymentManagedEnvironment
	var ctx context.Context

	BeforeEach(func() {

		ctx = context.Background()

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				UID:  uuid.NewUUID(),
			},
			Spec: corev1.NamespaceSpec{},
		}

		managedEnv = &GitOpsDeploymentManagedEnvironment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-managed-env",
				Namespace: namespace.Name,
				UID:       uuid.NewUUID(),
			},
			Spec: GitOpsDeploymentManagedEnvironmentSpec{
				ClusterCredentialsSecret: "fake-secret-name",
			},
		}

	})

	Context("Create GitOpsDeploymentManagedEnvironment CR with invalid API URL", func() {
		It("Should fail with error saying cluster api url must start with https://", func() {

			err := k8sClient.Create(ctx, namespace)
			Expect(err).ToNot(HaveOccurred())

			managedEnv.Spec.APIURL = "smtp://api-url"
			err = k8sClient.Create(ctx, managedEnv)

			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring(error_invalid_cluster_api_url))
		})
	})

	Context("Update GitOpsDeploymentManagedEnvironment CR with invalid invalid API URL", func() {
		It("Should fail with error saying cluster api url must start with https://", func() {

			managedEnv.Spec.APIURL = "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"

			err := k8sClient.Create(ctx, managedEnv)
			Expect(err).Should(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(managedEnv), managedEnv)
			Expect(err).To(Succeed())

			managedEnv.Spec.APIURL = "smtp://api-url"
			err = k8sClient.Update(ctx, managedEnv)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(error_invalid_cluster_api_url))

			err = k8sClient.Delete(context.Background(), managedEnv)
			Expect(err).ToNot(HaveOccurred())

		})
	})

})
