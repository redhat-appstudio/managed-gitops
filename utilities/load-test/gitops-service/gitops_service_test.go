package gitopsservice

import (
	"context"
	"fmt"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Simulate GitOps Service on RHTAP production environment", func() {
	Context("create large number of user resources to investigate memory/CPU usage", func() {

		var (
			k8sClient client.Client
			ctx       context.Context
		)
		BeforeEach(func() {
			config, err := fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())

			k8sClient, err = fixture.GetKubeClient(config)
			Expect(err).To(BeNil())

			ctx = context.Background()
		})

		It("should create user namespaces, GitOpsDeployments and secrets", func() {

			numberOfUsers := 291

			createUserResources := func(user int) {
				defer GinkgoRecover()
				By("create a namespace")
				ns := corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("user-%d", user),
					},
				}

				err := k8sClient.Create(ctx, &ns)
				if !apierr.IsAlreadyExists(err) {
					Expect(err).To(BeNil())
				}

				By("create two GitOpsDeployments in the user namespace")
				depl1 := managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("user-%d", user),
						Namespace: ns.Name,
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/managed-gitops-test-data/deployment-permutations-a",
							Path:           "pathB",
							TargetRevision: "branchA",
						},
						Destination: managedgitopsv1alpha1.ApplicationDestination{},
						Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					},
				}

				err = k8sClient.Create(ctx, &depl1)
				if !apierr.IsAlreadyExists(err) {
					Expect(err).To(BeNil())
				}

				depl2 := &managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("user-secondary-%d", user),
						Namespace: ns.Name,
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL: "https://github.com/redhat-appstudio/managed-gitops",
							Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						},
						Destination: managedgitopsv1alpha1.ApplicationDestination{},
						Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					},
				}

				err = k8sClient.Create(ctx, depl2)
				if !apierr.IsAlreadyExists(err) {
					Expect(err).To(BeNil())
				}

				By("create secrets in the user namespace")
				for i := 0; i < 10; i++ {
					secret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("user-secret-%d", i),
							Namespace: ns.Name,
						},
						Data: map[string][]byte{
							"sample-key": []byte(strings.Repeat("samplesecret", 50)),
						},
					}

					err := k8sClient.Create(context.Background(), &secret)
					if !apierr.IsAlreadyExists(err) {
						Expect(err).To(BeNil())
					}
				}
			}

			runTest := func(user int, wg *sync.WaitGroup) {
				createUserResources(user)
				wg.Done()
			}

			var wg sync.WaitGroup
			wg.Add(numberOfUsers)
			for i := 1; i <= numberOfUsers; i++ {
				go runTest(i, &wg)
			}

			GinkgoWriter.Println("Waiting for Goroutines to finish")
			wg.Wait()
		})
	})
})
