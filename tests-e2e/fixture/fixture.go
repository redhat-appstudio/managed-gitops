package fixture

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// GitOpsServiceE2ENamespace is the namespace that GitOpsService API resources (GitOpsDeployments, etc) should created it
	GitOpsServiceE2ENamespace = "gitops-service-e2e"

	// NewArgoCDInstanceNamespace  is the namespace thats test should use if they wish to install a new Argo CD instance
	NewArgoCDInstanceNamespace = "my-argocd"

	// NewArgoCDInstanceDestNamespace is the destinaton Argo CD Application namespace tests should use if they wish to deploy from a new Argo CD instance
	NewArgoCDInstanceDestNamespace = "argocd-instance-dest-namespace"
)

// EnsureCleanSlate should be called before every E2E tests:
// it ensures that the state of the GitOpsServiceE2ENamespace namespace is reset to scratch before each test, including:
// - Removing any old resources in GitOpsServiceE2ENamespace, by deleting the namespace
// - Recreating the GitOpsServiceE2ENamespace with the expected settings
// - Waiting for Argo CD to watch the namespace before exiting (to prevent race conditions)
//
// This ensures that previous E2E tests runs do not interfere with the results of current test runs.
// This function can also be called after a test, in order to clean up any resources it create in the GitOpsServiceE2ENamespace.
func EnsureCleanSlate() error {

	if err := DeleteNamespace(NewArgoCDInstanceNamespace); err != nil {
		return err
	}

	if err := DeleteNamespace(NewArgoCDInstanceDestNamespace); err != nil {
		return err
	}

	if err := ensureDestinationNamespaceExists(GitOpsServiceE2ENamespace, dbutil.DefaultGitOpsEngineSingleInstanceNamespace); err != nil {
		return err
	}

	if err := cleanUpOldKubeSystemResources(); err != nil {
		return err
	}

	return nil
}

// cleanUpOldKubeSystemResources cleans up ServiceAccounts, ClusterRoles, and ClusterRoleBindings created by these tests.
func cleanUpOldKubeSystemResources() error {
	k8sClient, err := GetKubeClient()
	if err != nil {
		return err
	}

	saList := corev1.ServiceAccountList{}
	if err := k8sClient.List(context.Background(), &saList, &client.ListOptions{Namespace: "kube-system"}); err != nil {
		return err
	}

	for idx := range saList.Items {
		sa := saList.Items[idx]
		// Skip any service accounts that DON'T contain argocd
		if !strings.Contains(sa.Name, "argocd") {
			continue
		}

		if err := k8sClient.Delete(context.Background(), &sa); err != nil {
			return err
		}
	}

	crList := rbacv1.ClusterRoleBindingList{}
	if err := k8sClient.List(context.Background(), &crList, &client.ListOptions{}); err != nil {
		return err
	}

	for idx := range crList.Items {
		sa := crList.Items[idx]
		// Skip any CRBs that DON'T contain argocd-manager
		if !strings.Contains(sa.Name, "argocd-manager") {
			continue
		}

		if err := k8sClient.Delete(context.Background(), &sa); err != nil {
			return err
		}
	}

	crbList := rbacv1.ClusterRoleList{}
	if err := k8sClient.List(context.Background(), &crbList, &client.ListOptions{}); err != nil {
		return err
	}

	for idx := range crbList.Items {
		sa := crbList.Items[idx]
		// Skip any CRBs that DON'T contain argocd-manager
		if !strings.Contains(sa.Name, "argocd-manager") {
			continue
		}

		if err := k8sClient.Delete(context.Background(), &sa); err != nil {
			return err
		}
	}

	return nil
}

func ensureDestinationNamespaceExists(namespaceParam string, argoCDNamespaceParam string) error {
	policy := metav1.DeletePropagationForeground

	kubeClientSet, err := GetKubeClientSet()
	if err != nil {
		return err
	}

	// Delete the e2e namespace, if it exists
	err = kubeClientSet.CoreV1().Namespaces().Delete(context.Background(), namespaceParam, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !apierr.IsNotFound(err) {
		return err
	}

	// Wait for namespace to delete
	if err := wait.Poll(time.Second*1, time.Minute*2, func() (done bool, err error) {

		_, err = kubeClientSet.CoreV1().Namespaces().Get(context.Background(), namespaceParam, metav1.GetOptions{})
		if err != nil {
			if apierr.IsNotFound(err) {
				return true, nil
			} else {
				return false, err
			}
		}

		return false, nil
	}); err != nil {
		return err
	}

	// Create the namespace again
	_, err = kubeClientSet.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: namespaceParam,
		Labels: map[string]string{
			"argocd.argoproj.io/managed-by": argoCDNamespaceParam,
		},
	}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	// Wait for Argo CD to process the namespace, before we exit:
	// - This helps us avoid a race condition where the namespace is created, but Argo CD has not yet
	//   set up proper roles for it.

	if err := wait.Poll(time.Second*1, time.Minute*2, func() (done bool, err error) {
		var roleBindings *rbacv1.RoleBindingList
		roleBindings, err = kubeClientSet.RbacV1().RoleBindings(namespaceParam).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		count := 0
		// Exit the poll loop when there exists at least one rolebinding containing 'gitops-service-argocd' in it's name
		// This helps us avoid a race condition where the namespace is created, but Argo CD has not yet
		// set up proper roles for it.
		for _, item := range roleBindings.Items {
			if strings.Contains(item.Name, argoCDNamespaceParam+"-") {
				count++
			}
		}

		if count >= 2 {
			// We expect at least 2 rolebindings
			return true, nil
		}

		return false, nil
	}); err != nil {
		return err
	}

	if err := wait.Poll(time.Second*1, time.Minute*2, func() (done bool, err error) {
		var roles *rbacv1.RoleList
		roles, err = kubeClientSet.RbacV1().Roles(namespaceParam).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		count := 0
		// Exit the poll loop when there exists at least one rolebinding containing 'gitops-service-argocd' in it's name
		// This helps us avoid a race condition where the namespace is created, but Argo CD has not yet
		// set up proper roles for it.
		for _, item := range roles.Items {
			if strings.Contains(item.Name, argoCDNamespaceParam+"-") {
				count++
			}
		}

		if count >= 2 {
			// We expect at least 2 roles
			return true, nil
		}

		return false, nil
	}); err != nil {
		return err
	}

	return nil
}

// DeleteNamespace deletes a namespace, and waits for it to be reported as deleted.
func DeleteNamespace(namespaceParam string) error {
	policy := metav1.DeletePropagationForeground

	kubeClientSet, err := GetKubeClientSet()
	if err != nil {
		return err
	}

	// Delete the namespace, if it exists
	err = kubeClientSet.CoreV1().Namespaces().Delete(context.Background(), namespaceParam, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !apierr.IsNotFound(err) {
		return err
	}

	// Wait for namespace to delete
	if err := wait.Poll(time.Second*1, time.Minute*2, func() (done bool, err error) {

		_, err = kubeClientSet.CoreV1().Namespaces().Get(context.Background(), namespaceParam, metav1.GetOptions{})
		if err != nil {
			if apierr.IsNotFound(err) {
				return true, nil
			} else {
				return false, err
			}
		}

		return false, nil
	}); err != nil {
		return err
	}

	return nil
}

// Retrieve the system-level Kubernetes config (e.g. ~/.kube/config)
func GetKubeConfig() (*rest.Config, error) {

	overrides := clientcmd.ConfigOverrides{}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)

	restConfig, err := clientConfig.ClientConfig()

	if restConfig != nil {
		// Sanity check that we're not running on a known staging system: don't want to accidentally break staging :|
		if strings.Contains(restConfig.Host, "x99m.p1.openshiftapps.com") {
			return nil, fmt.Errorf("E2E tests should not be run on staging server")
		}

		// fmt.Println("Using Kube Config, targeting host:", restConfig.Host)
	}

	return restConfig, err
}

// GetKubeClientSet returns a Clientset for accesing K8s API resources.
// This API has the advantage over `GetKubeClient` in that it is strongly typed, but cannot be used for
// custom resources.
func GetKubeClientSet() (*kubernetes.Clientset, error) {
	config, err := GetKubeConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// GetKubeClient returns a controller-runtime Client for accessing K8s API resources.
// The client returned by this function will, by default, already be aware of all
// the necessary schemes for interacting with Argo CD/GitOps Service.
func GetKubeClient() (client.Client, error) {

	config, err := GetKubeConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	err = managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = operation.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = apps.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = rbacv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = argocdoperator.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = routev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = appv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = appstudiosharedv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return k8sClient, nil

}
