package fixture

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	routev1 "github.com/openshift/api/route/v1"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
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

	// ENVGitOpsInKCP is an environment variable that is set when running our e2e tests against KCP
	ENVGitOpsInKCP = "GITOPS_IN_KCP"
)

// EnsureCleanSlateNonKCPVirtualWorkspace should be called before every E2E tests:
// it ensures that the state of the GitOpsServiceE2ENamespace namespace (and other resources on the cluster) is reset
// to scratch before each test, including:
// - Removing any old GitOps Service API resources in GitOpsServiceE2ENamespace
// - Removing any old resources in GitOpsServiceE2ENamespace, by deleting the namespace
// - Recreating the GitOpsServiceE2ENamespace with the expected settings
// - Waiting for Argo CD to watch the namespace before exiting (to prevent race conditions)
// - Remove the second Argo CD namespace, for the limited # of tests that create a new Argo CD instance.
//
// This ensures that previous E2E tests runs do not interfere with the results of current test runs.
// This function can also be called after a test, in order to clean up any resources it create in the GitOpsServiceE2ENamespace.
func EnsureCleanSlateNonKCPVirtualWorkspace() error {

	clientconfig, err := GetSystemKubeConfig()
	if err != nil {
		return err
	}

	// Clean up after tests that target the non-default Argo CD instance (only used by a few E2E tests)
	if err := cleanUpOldArgoCDApplications(NewArgoCDInstanceDestNamespace, NewArgoCDInstanceDestNamespace, clientconfig); err != nil {
		return err
	}

	if err := DeleteNamespace(NewArgoCDInstanceNamespace, clientconfig); err != nil {
		return err
	}

	if err := DeleteNamespace(NewArgoCDInstanceDestNamespace, clientconfig); err != nil {
		return err
	}

	// Clean up after tests that target the default Argo CD E2E instance (used by most E2E tests)
	if err := cleanUpOldArgoCDApplications(dbutil.GetGitOpsEngineSingleInstanceNamespace(), GitOpsServiceE2ENamespace, clientconfig); err != nil {
		return err
	}

	if err := ensureDestinationNamespaceExists(GitOpsServiceE2ENamespace, dbutil.GetGitOpsEngineSingleInstanceNamespace(), clientconfig); err != nil {
		return err
	}

	if err := cleanUpOldKubeSystemResources(clientconfig); err != nil {
		return err
	}

	// Delete all Argo CD Cluster Secrets from the default Argo CD Namespace
	secretList := &corev1.SecretList{}
	k8sClient, err := GetKubeClient(clientconfig)
	if err != nil {
		return err
	}
	if err := k8sClient.List(context.Background(), secretList, &client.ListOptions{Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace()}); err != nil {
		return err
	}
	for idx := range secretList.Items {
		secret := secretList.Items[idx]
		if strings.HasPrefix(secret.Name, "managed-env") {
			if err := k8sClient.Delete(context.Background(), &secret); err != nil {
				return err
			}
		}
	}

	return nil
}

// cleanUpOldArgoCDApplications issues a Delete request to k8s, for any Argo CD Applications in 'namespaceParam' that
// have a .Spec.Destination of 'destNamespace'
func cleanUpOldArgoCDApplications(namespaceParam string, destNamespace string, clientConfig *rest.Config) error {
	k8sClient, err := GetKubeClient(clientConfig)
	if err != nil {
		return err
	}
	argoCDApplicationList := appv1alpha1.ApplicationList{}
	if err = k8sClient.List(context.Background(), &argoCDApplicationList, &client.ListOptions{Namespace: namespaceParam}); err != nil {
		return fmt.Errorf("unable to list '%s': %v ", namespaceParam, err)
	}

	// Delete an Argo CD Application resources that reference the destination namespace
	for idx := range argoCDApplicationList.Items {

		app := argoCDApplicationList.Items[idx]
		if app.Spec.Destination.Namespace == destNamespace {
			GinkgoWriter.Println("Deleting Argo CD Application:", app.Name)
			if err := k8sClient.Delete(context.Background(), &app); err != nil {
				if !apierr.IsNotFound(err) {
					return fmt.Errorf("unable to delete '%s': %v", namespaceParam, err)
				}
			}
		}
	}

	return nil
}

// cleanUpOldKubeSystemResources cleans up ServiceAccounts, ClusterRoles, and ClusterRoleBindings created by these tests.
func cleanUpOldKubeSystemResources(clientConfig *rest.Config) error {
	k8sClient, err := GetKubeClient(clientConfig)
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

// cleanUpOldGitOpsServiceAPIs deletes old GitOpsDeployment* APIs
func cleanUpOldGitOpsServiceAPIs(namespace string, k8sClient client.Client) error {

	gitopsDeploymentList := managedgitopsv1alpha1.GitOpsDeploymentList{}
	if err := k8sClient.List(context.Background(), &gitopsDeploymentList, &client.ListOptions{Namespace: namespace}); err != nil {
		return fmt.Errorf("unable to cleanup old GitOpsDeployments: %v", err)
	}

	for idx := range gitopsDeploymentList.Items {
		gitopsDeployment := gitopsDeploymentList.Items[idx]
		if err := k8sClient.Delete(context.Background(), &gitopsDeployment); err != nil {
			return err
		}
	}

	gitopsManagedEnvironments := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentList{}
	if err := k8sClient.List(context.Background(), &gitopsManagedEnvironments, &client.ListOptions{Namespace: namespace}); err != nil {
		return fmt.Errorf("unable to cleanup old GitOpsDeployments: %v", err)
	}

	for idx := range gitopsManagedEnvironments.Items {
		gitopsManagedEnv := gitopsManagedEnvironments.Items[idx]
		if err := k8sClient.Delete(context.Background(), &gitopsManagedEnv); err != nil {
			return err
		}
	}

	return nil

}

func ensureDestinationNamespaceExists(namespaceParam string, argoCDNamespaceParam string, clientConfig *rest.Config) error {

	kubeClientSet, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	if err := DeleteNamespace(namespaceParam, clientConfig); err != nil {
		return fmt.Errorf("unable to delete namespace '%s': %v", namespaceParam, err)
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

	if IsRunningAgainstKCP() {
		if err = addMissingPermissions(kubeClientSet, namespaceParam, argoCDNamespaceParam); err != nil {
			return nil
		}

		// allow argocd to manage the target namespace
		err = wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {
			secretList, err := kubeClientSet.CoreV1().Secrets(argoCDNamespaceParam).List(context.Background(), metav1.ListOptions{
				LabelSelector: "argocd.argoproj.io/secret-type=cluster",
			})
			if err != nil {
				return false, err
			}

			if len(secretList.Items) > 0 {
				clusterSecret := secretList.Items[0]

				ns := []string{argoCDNamespaceParam, namespaceParam}
				clusterSecret.Data["namespaces"] = []byte(strings.Join(ns, ","))

				_, err = kubeClientSet.CoreV1().Secrets(argoCDNamespaceParam).Update(context.Background(), &clusterSecret, metav1.UpdateOptions{})
				if err != nil {
					return false, err
				}
			}

			return true, nil
		})
		if err != nil {
			return err
		}
	}

	// Wait for Argo CD to process the namespace, before we exit:
	// - This helps us avoid a race condition where the namespace is created, but Argo CD has not yet
	//   set up proper roles for it.

	if err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {
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

	if err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {
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
		return fmt.Errorf("argo CD never setup rolebindings for namespace '%s': %v", namespaceParam, err)
	}

	return nil
}

// DeleteNamespace deletes a namespace, and waits for it to be reported as deleted.
func DeleteNamespace(namespaceParam string, clientConfig *rest.Config) error {

	k8sClient, err := GetKubeClient(clientConfig)
	if err != nil {
		return err
	}

	// Delete the namespace:
	// - First, we do a bunch of clean up in the namespace (and in the Argo CD namespace), to ensure the namespace
	//   can be successfully clean.ed
	// - Next, we issue a request to Delete the namespace
	// - Finally, we check if it has been deleted.
	if err := wait.PollImmediate(time.Second*5, time.Minute*6, func() (done bool, err error) {

		// Deletes old GitOpsDeployment* APIs in this namespace
		if err := cleanUpOldGitOpsServiceAPIs(namespaceParam, k8sClient); err != nil {
			GinkgoWriter.Printf("unable to delete old GitOps Service APIs in '%s': %v\n", namespaceParam, err)
			return false, nil
		}

		// Delete any Argo CD Applications in the Argo CD namespace that target this namespace
		if err := cleanUpOldArgoCDApplications(dbutil.GetGitOpsEngineSingleInstanceNamespace(), namespaceParam, clientConfig); err != nil {
			GinkgoWriter.Printf("unable to clean up old Argo CD Applications targetting '%s': %v'\n", namespaceParam, err)
			return false, nil
		}

		// Remove finalizers from any Argo CD Applications in this Namespace
		if err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {

			argoCDApplicationList := appv1alpha1.ApplicationList{}
			if err = k8sClient.List(context.Background(), &argoCDApplicationList, &client.ListOptions{Namespace: namespaceParam}); err != nil {
				GinkgoWriter.Println("unable to list Argo CD Applications in '"+namespaceParam+"'", err)
				return false, nil
			}

			for idx := range argoCDApplicationList.Items {

				app := argoCDApplicationList.Items[idx]
				if len(app.Finalizers) > 0 {
					err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&app), &app)
					if err != nil {
						GinkgoWriter.Println("unable to get Application '"+app.Name+"'", err)
						return false, nil
					}
					app.Finalizers = []string{}
					err = k8sClient.Update(context.Background(), &app)
					if err != nil {
						GinkgoWriter.Println("unable to update Application '"+app.Name+"'", err)
						return false, nil
					}
				}
			}

			return true, nil
		}); err != nil {
			GinkgoWriter.Printf("unable to remove finalizer from Argo CD Applications in namespace '%s': %v\n", namespaceParam, err)
			return false, nil
		}

		if IsRunningAgainstKCP() {
			if _, err := removeKCPFinalizers(k8sClient, namespaceParam); err != nil {
				GinkgoWriter.Printf("unable to remove finalizers: %w\n", namespaceParam, err)
				return false, nil
			}
		}

		// Delete the namespace, if it exists
		namespace := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceParam,
			},
		}
		if err := k8sClient.Delete(context.Background(), &namespace); err != nil {
			if !apierr.IsNotFound(err) {
				GinkgoWriter.Printf("unable to delete namespace '%s': %v\n", namespaceParam, err)
				return false, nil
			}
		}

		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
			if apierr.IsNotFound(err) {
				return true, nil
			} else {
				GinkgoWriter.Printf("unable to Get namespace '%s': %v\n", namespaceParam, err)
				return false, nil
			}
		}

		return false, nil
	}); err != nil {
		return fmt.Errorf("namespace was never deleted, after delete was issued. '%s':%v", namespaceParam, err)
	}

	return nil
}

// setRateLimitOnRestConfig: set the QPS and Burst of the rest config
func setRateLimitOnRestConfig(restConfig *rest.Config) error {
	if restConfig != nil {
		// Prevent rate limiting of our requests
		restConfig.QPS = 100
		restConfig.Burst = 250

		// Sanity check that we're not running on a known staging system: don't want to accidentally break staging :|
		if strings.Contains(restConfig.Host, "x99m.p1.openshiftapps.com") {
			return fmt.Errorf("E2E tests should not be run on staging server")
		}
	}
	return nil
}

// Retrieve the system-level Kubernetes config (e.g. ~/.kube/config)
func GetSystemKubeConfig() (*rest.Config, error) {

	overrides := clientcmd.ConfigOverrides{}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	err = setRateLimitOnRestConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return restConfig, nil
}

// GetE2ETestUserWorkspaceKubeConfig retrieves the E2ETest User workspace Kubernetes config
// Return a k8s config that can be used to access user GitOpsDeployment* APIs or Secrets
// or just the normal openshift/k8s cluster (when not running in KCP);
func GetE2ETestUserWorkspaceKubeConfig() (*rest.Config, error) {

	if !IsRunningAgainstKCP() || sharedutil.IsKCPVirtualWorkspaceDisabled() {
		return GetSystemKubeConfig()
	} else {
		var kubeconfig *string
		userEnv, exists := os.LookupEnv("USER_KUBECONFIG")
		if exists {
			kubeconfig = flag.String("kubeconfig", filepath.Join("", userEnv), "(optional) absolute path to the kubeconfig file")
			flag.Parse()
		} else {
			return nil, fmt.Errorf("USER_KUBECONFIG env variable not set")
		}

		restConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, err
		}

		err = setRateLimitOnRestConfig(restConfig)
		if err != nil {
			return nil, err
		}

		return restConfig, nil
	}
}

// GetServiceProviderWorkspaceKubeConfig Return a K8s config that can be used to write to service provider workspace (when running in KCP),
// or just the normal openshift/k8s cluster (when not running in KCP); For example, to see Argo CD Application CRs
func GetServiceProviderWorkspaceKubeConfig() (*rest.Config, error) {

	if !IsRunningAgainstKCP() || sharedutil.IsKCPVirtualWorkspaceDisabled() {
		return GetSystemKubeConfig()
	} else {
		var kubeconfig *string
		userEnv, exists := os.LookupEnv("SERVICE_PROVIDER_KUBECONFIG")
		if exists {
			kubeconfig = flag.String("kubeconfig", filepath.Join("", userEnv), "(optional) absolute path to the kubeconfig file")
			flag.Parse()
		} else {
			return nil, fmt.Errorf("SERVICE_PROVIDER_KUBECONFIG env variable not set")
		}

		restConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, err
		}

		err = setRateLimitOnRestConfig(restConfig)
		if err != nil {
			return nil, err
		}

		return restConfig, nil
	}
}

// GetKubeClientSet returns a Clientset for accesing K8s API resources.
// This API has the advantage over `GetKubeClient` in that it is strongly typed, but cannot be used for
// custom resources.
func GetKubeClientSet() (*kubernetes.Clientset, error) {
	config, err := GetSystemKubeConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// GetKubeClient returns a controller-runtime Client for accessing K8s API resources.
// The client returned by this function will, by default, already be aware of all
// the necessary schemes for interacting with Argo CD/GitOps Service.
func GetKubeClient(config *rest.Config) (client.Client, error) {

	scheme := runtime.NewScheme()
	err := managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = managedgitopsv1alpha1.AddToScheme(scheme)
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

// reportRemainingArgoCDApplications outputs the current contents of all Argo CD Applications. This can be useful while debugging failing tests,
// to determine what the final state of an Argo CD Application was at the end of a test.
func ReportRemainingArgoCDApplications(k8sClient client.Client) error {
	argoCDApplicationList := appv1alpha1.ApplicationList{}
	if err := k8sClient.List(context.Background(), &argoCDApplicationList); err != nil {
		return err
	}

	GinkgoWriter.Println("Argo CD Applications present at end of test:")

	for idx, application := range argoCDApplicationList.Items {
		jsonStr, err := json.Marshal(application)
		if err != nil {
			return err
		}

		GinkgoWriter.Printf("- %d) %s\n", idx+1, (string)(jsonStr))
	}
	GinkgoWriter.Println()

	return nil
}

func addMissingPermissions(kubeClientSet *kubernetes.Clientset, namespace, argocdNamespace string) error {
	if !IsRunningAgainstKCP() {
		return nil
	}

	addNamespacePrefix := func(name string) string {
		return fmt.Sprintf("%s-%s", argocdNamespace, name)
	}

	getAdminRole := func(name string) *rbacv1.Role {
		return &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addNamespacePrefix(name),
				Namespace: namespace,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"*"},
					Resources: []string{"*"},
					APIGroups: []string{"*"},
				},
			},
		}
	}

	getRolebinding := func(name string) *rbacv1.RoleBinding {
		return &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addNamespacePrefix(name),
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Name:      name,
					Namespace: argocdNamespace,
					Kind:      "ServiceAccount",
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Name:     addNamespacePrefix(name),
				Kind:     "Role",
			},
		}
	}

	_, err := kubeClientSet.RbacV1().Roles(namespace).Create(context.Background(), getAdminRole("argocd-server"), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = kubeClientSet.RbacV1().Roles(namespace).Create(context.Background(), getAdminRole("argocd-application-controller"), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = kubeClientSet.RbacV1().RoleBindings(namespace).Create(context.Background(), getRolebinding("argocd-server"), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = kubeClientSet.RbacV1().RoleBindings(namespace).Create(context.Background(), getRolebinding("argocd-application-controller"), metav1.CreateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func IsRunningAgainstKCP() bool {
	return os.Getenv(ENVGitOpsInKCP) == "true"
}

func removeKCPFinalizers(k8sClient client.Client, namespaceParam string) (bool, error) {
	if !IsRunningAgainstKCP() {
		return true, nil
	}

	// Remove KCP finalizers from secrets in this namespace
	if err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {

		secretList := corev1.SecretList{}
		if err = k8sClient.List(context.Background(), &secretList, &client.ListOptions{Namespace: namespaceParam}); err != nil {
			GinkgoWriter.Println("unable to list secrets in '"+namespaceParam+"'", err)
			return false, nil
		}

		for idx := range secretList.Items {

			secret := secretList.Items[idx]
			if len(secret.Finalizers) > 0 {
				err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&secret), &secret)
				if err != nil {
					GinkgoWriter.Println("unable to get Secret '"+secret.Name+"'", err)
					return false, nil
				}
				secret.Finalizers = []string{}
				err = k8sClient.Update(context.Background(), &secret)
				if err != nil {
					GinkgoWriter.Println("unable to update Secret '"+secret.Name+"'", err)
					return false, nil
				}
			}
		}

		return true, nil
	}); err != nil {
		return false, fmt.Errorf("unable to remove finalizer from secret in namespace '%s': %v", namespaceParam, err)
	}

	// Remove KCP finalizers from service accounts in this namespace
	if err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {

		saList := corev1.ServiceAccountList{}
		if err = k8sClient.List(context.Background(), &saList, &client.ListOptions{Namespace: namespaceParam}); err != nil {
			GinkgoWriter.Println("unable to list service accounts in '"+namespaceParam+"'", err)
			return false, nil
		}

		for idx := range saList.Items {

			sa := saList.Items[idx]
			if len(sa.Finalizers) > 0 {
				err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&sa), &sa)
				if err != nil {
					GinkgoWriter.Println("unable to get service account '"+sa.Name+"'", err)
					return false, nil
				}
				sa.Finalizers = []string{}
				err = k8sClient.Update(context.Background(), &sa)
				if err != nil {
					GinkgoWriter.Println("unable to update service account '"+sa.Name+"'", err)
					return false, nil
				}
			}
		}

		return true, nil
	}); err != nil {
		return false, fmt.Errorf("unable to remove finalizer from service account in namespace '%s': %v", namespaceParam, err)
	}

	// Remove KCP finalizers from configmaps in this namespace
	if err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {

		cmList := corev1.ConfigMapList{}
		if err = k8sClient.List(context.Background(), &cmList, &client.ListOptions{Namespace: namespaceParam}); err != nil {
			GinkgoWriter.Println("unable to list configmaps in '"+namespaceParam+"'", err)
			return false, nil
		}

		for idx := range cmList.Items {

			cm := cmList.Items[idx]
			if len(cm.Finalizers) > 0 {
				err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&cm), &cm)
				if err != nil {
					GinkgoWriter.Println("unable to get configmap '"+cm.Name+"'", err)
					return false, nil
				}
				cm.Finalizers = []string{}
				err = k8sClient.Update(context.Background(), &cm)
				if err != nil {
					GinkgoWriter.Println("unable to update configmap '"+cm.Name+"'", err)
					return false, nil
				}
			}
		}

		return true, nil
	}); err != nil {
		return false, fmt.Errorf("unable to remove finalizer from configmap in namespace '%s': %v", namespaceParam, err)
	}

	return true, nil
}

// EnsureCleanSlateKCPVirtualWorkspace should be called before every E2E tests:
// it ensures that in KCP Virtual workspace the state of the GitOpsServiceE2ENamespace namespace
//	(and other resources on the cluster) is reset to scratch before each test, including:
// - In user workspace, the function will:
// 		- Deleting any old namespaces that exists within the user-workspace
// 		- Deleting any cluster role/rolebindings existing within the user-workspace
// 		- Delete the e2e namespaces, and create a new e2e namespace for testing
//		- Clean up old kube system resources from the workspace
// - In the gitops-service-provider workspace, the function will:
//		- Delete all Argo CD Cluster Secrets from the Argo CD Namespace
//		- Clean up old argo cd applications targetting the e2e namespace
//
// Need two different client ===> client virtual workspace enabled for workspace
//
// This ensures that previous E2E tests runs do not interfere with the results of current test runs.
// This function can also be called after a test, in order to clean up any resources it creates in respective workspaces.
func EnsureCleanSlateKCPVirtualWorkspace() error {

	if !IsRunningAgainstKCP() {
		return fmt.Errorf("Tests are not running in a KCP enviroment")
	}

	// Service Provider WS is where gitops service is running so we can delete from the same clientset
	userConfig, err := GetE2ETestUserWorkspaceKubeConfig()
	if err != nil {
		return err
	}

	if err := DeleteNamespace(NewArgoCDInstanceNamespace, userConfig); err != nil {
		return err
	}

	if err := DeleteNamespace(NewArgoCDInstanceDestNamespace, userConfig); err != nil {
		return err
	}

	if err := ensureDestinationNamespaceExists(GitOpsServiceE2ENamespace, dbutil.GetGitOpsEngineSingleInstanceNamespace(), userConfig); err != nil {
		return err
	}

	if err := cleanUpOldKubeSystemResources(userConfig); err != nil {
		return err
	}

	// Service Provider WS is where gitops service is running so we can delete from the same clientset
	serviceConfig, err := GetServiceProviderWorkspaceKubeConfig()
	if err != nil {
		return err
	}

	// Delete all Argo CD Cluster Secrets from the default Argo CD Namespace
	secretList := &corev1.SecretList{}
	serviceWSk8sClient, err := GetKubeClient(serviceConfig)
	if err != nil {
		return err
	}
	if err := serviceWSk8sClient.List(context.Background(), secretList, &client.ListOptions{Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace()}); err != nil {
		return err
	}
	for idx := range secretList.Items {
		secret := secretList.Items[idx]
		if strings.HasPrefix(secret.Name, "managed-env") {
			if err := serviceWSk8sClient.Delete(context.Background(), &secret); err != nil {
				return err
			}
		}
	}

	// Clean up after tests that target the default Argo CD E2E instance (used by most E2E tests)
	if err := cleanUpOldArgoCDApplications(dbutil.GetGitOpsEngineSingleInstanceNamespace(), GitOpsServiceE2ENamespace, serviceConfig); err != nil {
		return err
	}

	return nil
}

func EnsureCleanSlate() error {

	if !sharedutil.IsKCPVirtualWorkspaceDisabled() {
		err := EnsureCleanSlateNonKCPVirtualWorkspace()
		return err
	} else {
		err := EnsureCleanSlateNonKCPVirtualWorkspace()
		return err
	}
}

func GetE2ETestUserWorkspaceKubeClient() (client.Client, error) {
	config, err := GetE2ETestUserWorkspaceKubeConfig()
	if err != nil {
		return nil, err
	}

	k8sClient, err := GetKubeClient(config)
	if err != nil {
		return nil, err
	}

	return k8sClient, nil
}
