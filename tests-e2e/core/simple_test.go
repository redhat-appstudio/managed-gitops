package core

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	argocdv1 "github.com/redhat-appstudio/managed-gitops/cluster-agent/utils"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	argocdnamespace = "my-argocd"
	argocdname      = "argocd"
)

var _ = Describe("GitOpsDeployment E2E tests", func() {

	Context("Create a new GitOpsDeployment", func() {

		It("should be healthy and have synced status, and resources should be deployed", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeployment")

			gitOpsDeploymentResource := buildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/gitops-repository-template", "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring GitOpsDeployment should have expected health and status")

			Eventually(gitOpsDeploymentResource, "30s", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)))

			By("ensuring the resources of the GitOps repo are successfully deployed")

			componentADepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-a", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			componentBDepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-b", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			Eventually(componentADepl, "60s", "1s").Should(k8s.ExistByName())
			Eventually(componentBDepl, "60s", "1s").Should(k8s.ExistByName())

			By("deleting the GitOpsDeployment")

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring the resources of the GitOps repo are successfully deleted")

			Eventually(componentADepl, "60s", "1s").ShouldNot(k8s.ExistByName())
			Eventually(componentBDepl, "60s", "1s").ShouldNot(k8s.ExistByName())

		})
	})
})

// buildGitOpsDeploymentResource builds a GitOpsDeployment with 'opinionated' default values, which is self-contained to
// the GitGitOpsServiceE2ENamespace. This makes it easy to clean up after tests using EnsureCleanSlate.
// - Defaults to creation in GitOpsServiceE2ENamespace
// - Defaults to deployment of K8s resources to GitOpsServiceE2ENamespace
func buildGitOpsDeploymentResource(name, repoURL, path, deploymentSpecType string) managedgitopsv1alpha1.GitOpsDeployment {

	gitOpsDeploymentResource := managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
			Source: managedgitopsv1alpha1.ApplicationSource{
				RepoURL: repoURL,
				Path:    path,
			},
			Destination: managedgitopsv1alpha1.ApplicationDestination{},
			Type:        deploymentSpecType,
		},
	}

	return gitOpsDeploymentResource
}

var _ = Describe("Standalone ArgoCD instance E2E tests", func() {

	Context("Create a Standalone ArgoCD instance", func() {
		kubeClientSet, err := fixture.GetKubeClientSet()
		Expect(err).To(BeNil())

		BeforeEach(func() {
			By("deleting the namespace before the test starts, so that the code can create it")
			policy := metav1.DeletePropagationForeground

			// Delete the e2e namespace, if it exists
			err = kubeClientSet.CoreV1().Namespaces().Delete(context.Background(), argocdnamespace, metav1.DeleteOptions{PropagationPolicy: &policy})
			if err != nil && !apierr.IsNotFound(err) {
				Expect(err).To(BeNil())
			}

			// Wait for namespace to delete
			if err := wait.Poll(time.Second*1, time.Minute*2, func() (done bool, err error) {

				_, err = kubeClientSet.CoreV1().Namespaces().Get(context.Background(), argocdnamespace, metav1.GetOptions{})
				if err != nil {
					if apierr.IsNotFound(err) {
						return true, nil
					} else {
						return false, err
					}
				}

				return false, nil
			}); err != nil {
				Expect(err).To(BeNil())
			}

			k8sClient, err := fixture.GetKubeClient()
			Expect(err).To(BeNil())

			saList := corev1.ServiceAccountList{}
			err = k8sClient.List(context.Background(), &saList, &client.ListOptions{Namespace: "kube-system"})
			Expect(err).To(BeNil())

			for idx := range saList.Items {
				sa := saList.Items[idx]
				// Skip any service accounts that DON'T contain argocd
				if !strings.Contains(sa.Name, "argocd") {
					continue
				}

				err = k8sClient.Delete(context.Background(), &sa)
				Expect(err).To(BeNil())
			}

			crList := rbacv1.ClusterRoleBindingList{}
			err = k8sClient.List(context.Background(), &crList, &client.ListOptions{})
			Expect(err).To(BeNil())

			for idx := range crList.Items {
				sa := crList.Items[idx]
				// Skip any CRBs that DON'T contain argocd-managed
				if !strings.Contains(sa.Name, "argocd-manager") {
					continue
				}

				err = k8sClient.Delete(context.Background(), &sa)
				Expect(err).To(BeNil())
			}

			crbList := rbacv1.ClusterRoleList{}
			err = k8sClient.List(context.Background(), &crbList, &client.ListOptions{})
			Expect(err).To(BeNil())

			for idx := range crbList.Items {
				sa := crbList.Items[idx]
				// Skip any CRBs that DON'T contain argocd-managed
				if !strings.Contains(sa.Name, "argocd-manager") {
					continue
				}

				err = k8sClient.Delete(context.Background(), &sa)
				Expect(err).To(BeNil())
			}

		})

		It("should create ArgoCD resource and application, wait for it to be installed and synced", func() {

			By("creating ArgoCD resource")
			ctx := context.Background()

			k8sClient, err := fixture.GetKubeClient()
			Expect(err).To(BeNil())

			argoCDResource := argocdoperator.ArgoCD{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{Name: argocdname, Namespace: argocdnamespace},
				Spec:       argocdoperator.ArgoCDSpec{},
				Status:     argocdoperator.ArgoCDStatus{},
			}
			err = argocdv1.CreateNamespaceScopedArgoCD(ctx, argoCDResource.Name, argoCDResource.Namespace, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring ArgoCD service resource exists")
			argocdInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: argocdname + "-server", Namespace: argocdnamespace},
			}
			Eventually(argocdInstance, "60s", "5s").Should(k8s.ExistByName())
			Expect(err).To(BeNil())

			By("ensuring ArgoCD resource exists in kube-system namespace")
			err = argocdv1.SetupArgoCD(k8sClient, kubeClientSet)
			Expect(err).To(BeNil())

			By("creating ArgoCD application")
			app := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argo-app-6",
					Namespace: argocdnamespace,
				},
				Spec: appv1.ApplicationSpec{
					Source: appv1.ApplicationSource{
						RepoURL:        "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:           "environments/overlays/dev",
						TargetRevision: "HEAD",
					},
					Destination: appv1.ApplicationDestination{
						Name:      "in-cluster",
						Namespace: fixture.GitOpsServiceE2ENamespace,
					},
				},
			}
			err = k8s.Create(&app)
			Expect(err).To(BeNil())

			cs := argocdv1.NewCredentialService(nil, true)
			Expect(cs).ToNot(BeNil())

			By("calling AppSync and waiting for it to return with no error")
			Eventually(func() bool {
				GinkgoWriter.Println("Attempting to sync application: ", app.Name)
				err := argocdv1.AppSync(context.Background(), app.Name, "", app.Namespace, k8sClient, cs, true)
				GinkgoWriter.Println("- AppSync result: ", err)
				return err == nil
			}).WithTimeout(time.Minute * 2).WithPolling(time.Second * 1).Should(BeTrue())

			//error on above line, rpc error: code = Unauthenticated desc = no session information

			// Eventually(app, "2m", "1s").Should(
			// 	SatisfyAll(
			// 		gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
			// 		gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)))

		})
	})
})
