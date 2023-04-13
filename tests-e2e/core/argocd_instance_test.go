package core

import (
	"context"
	"fmt"
	"reflect"
	"time"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	argocdv1 "github.com/redhat-appstudio/managed-gitops/cluster-agent/utils"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	appFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/argoproj/argo-cd/v2/util/settings"
	routev1 "github.com/openshift/api/route/v1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Standalone ArgoCD instance E2E tests", func() {

	const (
		argocdNamespace      = fixture.NewArgoCDInstanceNamespace
		argocdCRName         = "argocd"
		destinationNamespace = fixture.NewArgoCDInstanceDestNamespace
	)

	Context("Create a Standalone ArgoCD instance", func() {

		BeforeEach(func() {

			By("Delete old namespaces, and kube-system resources")
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("deleting the namespace before the test starts, so that the code can create it")
			config, err := fixture.GetSystemKubeConfig()
			if err != nil {
				panic(err)
			}
			err = fixture.DeleteNamespace(argocdNamespace, config)
			Expect(err).To(BeNil())

		})

		It("should create ArgoCD resource and application, wait for it to be installed and synced", func() {

			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			By("creating ArgoCD resource")
			ctx := context.Background()
			log := log.FromContext(ctx)

			config, err := fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())
			apiHost := config.Host

			k8sClient, err := fixture.GetKubeClient(config)
			Expect(err).To(BeNil())

			err = reconcileNamespaceScopedArgoCD(ctx, argocdCRName, argocdNamespace, k8sClient, log)
			Expect(err).To(BeNil())

			By("ensuring ArgoCD service resource exists")
			argocdInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: argocdCRName + "-server", Namespace: argocdNamespace},
			}

			Eventually(argocdInstance, "60s", "5s").Should(k8s.ExistByName(k8sClient))
			Expect(err).To(BeNil())

			By("ensuring ArgoCD resource exists in kube-system namespace")
			err = argocdv1.SetupArgoCD(ctx, apiHost, argocdNamespace, k8sClient, log)
			Expect(err).To(BeNil())

			destinationNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fixture.NewArgoCDInstanceDestNamespace,
					Labels: map[string]string{
						"argocd.argoproj.io/managed-by": argocdNamespace,
					},
				},
			}
			err = k8sClient.Create(ctx, destinationNamespace)
			Expect(err).To(BeNil())

			By("creating ArgoCD application")
			app := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argo-app-6",
					Namespace: argocdNamespace,
				},
				Spec: appv1.ApplicationSpec{
					Source: appv1.ApplicationSource{
						RepoURL:        "https://github.com/redhat-appstudio/managed-gitops",
						Path:           "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						TargetRevision: "HEAD",
					},
					Destination: appv1.ApplicationDestination{
						Name:      argocdv1.ClusterSecretName,
						Namespace: destinationNamespace.Name,
					},
				},
			}

			err = k8s.Create(&app, k8sClient)
			Expect(err).To(BeNil())

			cs := argocdv1.NewCredentialService(nil, true)
			Expect(cs).ToNot(BeNil())

			By("calling AppSync and waiting for it to return with no error")
			Eventually(func() bool {
				GinkgoWriter.Println("Attempting to sync application: ", app.Name)
				err := argocdv1.AppSync(context.Background(), app.Name, "", app.Namespace, k8sClient, cs, true)
				GinkgoWriter.Println("- AppSync result: ", err)
				return err == nil
			}).WithTimeout(time.Minute * 4).WithPolling(time.Second * 1).Should(BeTrue())

			Eventually(app, "2m", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(appv1.SyncStatusCodeSynced),
					appFixture.HaveHealthStatusCode(health.HealthStatusHealthy),
				))

		})
	})
})

// ReconcileNamespaceScopedArgoCD will create/update an ArgoCD operand within the specified namespace.
func reconcileNamespaceScopedArgoCD(ctx context.Context, argocdCRName string, namespace string, k8sClient client.Client, log logr.Logger) error {
	policy := "g, system:authenticated, role:admin"
	scopes := "[groups]"

	resourceExclusions, err := yaml.Marshal([]settings.FilteredResource{
		{
			APIGroups: []string{"*.kcp.dev"},
			Clusters:  []string{"*"},
			Kinds:     []string{"*"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal resource exclusions: %v", err)
	}

	// The values from manifests/staging-cluster-resources/argo-cd.yaml are converted in a Go struct.

	expectedArgoCDOperand := &argocdoperator.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{"argoproj.io/finalizer"},
			Name:       argocdCRName,
			Namespace:  namespace,
		},
		Spec: argocdoperator.ArgoCDSpec{

			ApplicationSet: &argocdoperator.ArgoCDApplicationSet{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			Controller: argocdoperator.ArgoCDApplicationControllerSpec{
				Processors: argocdoperator.ArgoCDApplicationControllerProcessorsSpec{},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				Sharding: argocdoperator.ArgoCDApplicationControllerShardSpec{},
			},
			Dex: &argocdoperator.ArgoCDDexSpec{
				OpenShiftOAuth: true,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			Grafana: argocdoperator.ArgoCDGrafanaSpec{
				Enabled: false,
				Ingress: argocdoperator.ArgoCDIngressSpec{
					Enabled: false,
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				Route: argocdoperator.ArgoCDRouteSpec{
					Enabled: false,
				},
			},
			HA: argocdoperator.ArgoCDHASpec{
				Enabled: false,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			InitialSSHKnownHosts: argocdoperator.SSHHostsSpec{},
			Prometheus: argocdoperator.ArgoCDPrometheusSpec{
				Enabled: false,
				Ingress: argocdoperator.ArgoCDIngressSpec{
					Enabled: false,
				},
				Route: argocdoperator.ArgoCDRouteSpec{
					Enabled: false,
				},
			},
			RBAC: argocdoperator.ArgoCDRBACSpec{
				Policy: &policy,
				Scopes: &scopes,
			},
			Redis: argocdoperator.ArgoCDRedisSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			Repo: argocdoperator.ArgoCDRepoSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			Server: argocdoperator.ArgoCDServerSpec{
				Autoscale: argocdoperator.ArgoCDServerAutoscaleSpec{
					Enabled: false,
				},
				GRPC: argocdoperator.ArgoCDServerGRPCSpec{
					Ingress: argocdoperator.ArgoCDIngressSpec{
						Enabled: false,
					},
				},
				Ingress: argocdoperator.ArgoCDIngressSpec{
					Enabled: false,
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("125m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				Route: argocdoperator.ArgoCDRouteSpec{
					Enabled: true,
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationReencrypt,
					},
				},
				Service: argocdoperator.ArgoCDServerServiceSpec{
					Type: "",
				},
			},
			TLS: argocdoperator.ArgoCDTLSSpec{
				CA: argocdoperator.ArgoCDCASpec{},
			},
			ResourceExclusions: string(resourceExclusions),
		},
	}

	newArgoCDNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	// 1) Get the namespace, or create it if it doesn't already exist
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(newArgoCDNamespace), newArgoCDNamespace); err != nil {
		if apierr.IsNotFound(err) {
			// It doesn't exist, so create it
			if err := k8sClient.Create(ctx, newArgoCDNamespace); err != nil {
				return fmt.Errorf("while creating Argo CD instance, a namespace could not be created: %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(newArgoCDNamespace.Namespace, newArgoCDNamespace.Name, newArgoCDNamespace, sharedutil.ResourceCreated, log)
		} else {
			return fmt.Errorf("while creating Argo CD instance, an unexpected error on retrieving Namespace: %v", err)
		}
	}

	// 2) Retrieve the ArgoCD operand: if it doesn't exist, create it. If it does exist, update it.
	existingArgoCDOperand := &argocdoperator.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      argocdCRName,
			Namespace: newArgoCDNamespace.Name,
		},
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(existingArgoCDOperand), existingArgoCDOperand); err != nil {

		if apierr.IsNotFound(err) {
			// A) Operand doesn't exist, so create it
			if errk8s := k8sClient.Create(ctx, expectedArgoCDOperand); errk8s != nil {

				return fmt.Errorf("error on creating: %s, %v ", expectedArgoCDOperand.GetName(), errk8s)
			}
			sharedutil.LogAPIResourceChangeEvent(expectedArgoCDOperand.Namespace, expectedArgoCDOperand.Name, expectedArgoCDOperand,
				sharedutil.ResourceCreated, log)

		} else {
			log.Error(err, "unexpected error on retrieving ArgoCD operand")
			return fmt.Errorf("unexpected error on retrieving ArgoCD operand, %v", err)
		}

	} else {
		// B) The existing ArgoCD resource already exists, so make sure it is up to date
		if !reflect.DeepEqual(existingArgoCDOperand.Spec, expectedArgoCDOperand) {
			existingArgoCDOperand.Spec = expectedArgoCDOperand.Spec
			if err := k8sClient.Update(ctx, existingArgoCDOperand); err != nil {
				log.Error(err, "unexpected error on updating existing ArgoCD operand")
				return fmt.Errorf("unexpected error on updating existing ArgoCD operand, %v", err)
			}

			sharedutil.LogAPIResourceChangeEvent(existingArgoCDOperand.Namespace, existingArgoCDOperand.Name, existingArgoCDOperand,
				sharedutil.ResourceModified, log)
		}
	}

	// Wait for Argo CD to be installed by gitops operator.
	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {

		// 'default' AppProject will be created by Argo CD if Argo CD is successfully started.
		appProject := &appv1.AppProject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: namespace,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(appProject), appProject); err != nil {
			if apierr.IsNotFound(err) {
				log.V(sharedutil.LogLevel_Debug).Info("Waiting for AppProject to exist in namespace " + namespace)
				return false, nil
			} else {
				log.Error(err, "unable to retrieve AppProject")
				return false, err
			}
		} else {
			return true, nil
		}
	})

	return err

}
