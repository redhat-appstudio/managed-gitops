package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	matcher "github.com/onsi/gomega/types"
	apierr "k8s.io/apimachinery/pkg/api/errors"
)

const (
	// K8sClientError is a prefix that can/should be used when outputting errors from K8s client
	K8sClientError = "Error from k8s client:"
)

// Create creates the given K8s resource, returning an error on failure, or nil otherwise.
func Create(obj client.Object, k8sClient client.Client) error {

	if err := k8sClient.Create(context.Background(), obj); err != nil {
		fmt.Println(K8sClientError, "Error on creating ", obj.GetName(), err)
		return err
	}

	return nil
}

// Get the given K8s resource, returning an error on failure, or nil otherwise.
func Get(obj client.Object, k8sClient client.Client) error {

	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
		fmt.Println(K8sClientError, "Unable to Get ", obj.GetName(), err)
		return err
	}

	return nil
}

// List instances of a given K8s resource, returning an error on failure, or nil otherwise.
func List(obj client.ObjectList, namespace string, k8sClient client.Client) error {

	if err := k8sClient.List(context.Background(), obj, &client.ListOptions{
		Namespace: namespace,
	}); err != nil {
		fmt.Println(K8sClientError, "Unable to List ", err)
		return err
	}

	return nil
}

// ExistByName checks if the given resource exists, when retrieving it by name/namespace.
// Does NOT check if the resource content matches.
func ExistByName(k8sClient client.Client) matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {

		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByName:", k8sObject.GetName())
		}
		return err == nil
	}, BeTrue())
}

// NotExist checks if the given resource does not exist.
func NotExist(k8sClient client.Client) matcher.GomegaMatcher {
	return WithTransform(func(obj client.Object) bool {
		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			if apierr.IsNotFound(err) {
				fmt.Println("Object does not exist in NotExist:", obj.GetName())
				return true
			}
			fmt.Println(K8sClientError, err)
			return false
		}
		fmt.Println("Object still exists in NotExist:", obj.GetName())
		return false
	}, BeTrue())
}

// Delete deletes a K8s object from the namespace; it returns an error on failure, or nil otherwise.
func Delete(obj client.Object, k8sClient client.Client) error {

	err := k8sClient.Delete(context.Background(), obj)
	if err != nil {
		if apierr.IsNotFound(err) {
			fmt.Println("Object no longer exists:", obj.GetName(), err)
			// success
			return nil
		}
		fmt.Println(K8sClientError, "Unable to delete in Delete:", obj.GetName(), err)
		return err
	}

	err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
	if apierr.IsNotFound(err) {
		fmt.Println("Object", "'"+obj.GetName()+"'", "was deleted.")
		// success
		return nil
	}

	// fail
	return fmt.Errorf("'%s' still exists", obj.GetName())

}

// UntilSuccess will keep trying a K8s operation until it succeeds, or times out.
func UntilSuccess(k8sClient client.Client, f func(k8sClient client.Client) error) error {

	err := wait.PollImmediate(time.Second*1, time.Minute*2, func() (done bool, err error) {
		funcError := f(k8sClient)
		return funcError == nil, nil
	})

	return err
}

// WARNING: calling this function may lead to race conditions. Strongly consider using 'UntilSuccess' instead.
//
// For example of how to do that, see 'UpdateStatusWithFunction' in 'fixture/binding/fixture.go',
// and 'buildAndUpdateBindingStatus' in 'snapshotenvironmentbinding_test.go'
//
// UpdateStatus updates the status of a K8s resource using the provided object.
func UpdateStatus(obj client.Object, k8sClient client.Client) error {

	if err := k8sClient.Status().Update(context.Background(), obj); err != nil {
		fmt.Println(K8sClientError, "Error on Status Update : ", err)
		return err
	}

	return nil
}

func Update(obj client.Object, k8sClient client.Client) error {

	if err := k8sClient.Update(context.Background(), obj, &client.UpdateOptions{}); err != nil {
		fmt.Println(K8sClientError, "Error on updating ", err)
		return err
	}

	return nil
}

func UpdateWithoutConflict(obj client.Object, k8sClient client.Client, modify func(client.Object)) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return err
		}
		modify(obj)
		return k8sClient.Update(context.Background(), obj)
	})

	return err
}

// CreateSecret creates a secret with the given stringData.
func CreateSecret(namespace string, secretName string, stringData map[string]string, k8sClient client.Client) error {

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Immutable:  nil,
		Data:       nil,
		StringData: stringData,
		Type:       "",
	}

	return Create(secret, k8sClient)
}

// getOrCreateServiceAccountBearerToken returns a token if there is an existing token secret for a service account.
// If the token secret is missing, it creates a new secret and attach it to the service account
func CreateServiceAccountBearerToken(ctx context.Context, k8sClient client.Client, serviceAccountName string,
	serviceAccountNS string) (string, error) {

	tokenSecret, err := createServiceAccountTokenSecret(ctx, k8sClient, serviceAccountName, serviceAccountNS)
	if err != nil {
		return "", fmt.Errorf("failed to create a token secret for service account %s: %w", serviceAccountName, err)
	}

	if err := wait.PollImmediate(time.Second*1, time.Second*120, func() (bool, error) {

		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret); err != nil {
			fmt.Println("waiting for ServiceAccountTokenSecret to exist: ", err)
		}

		// Exit the loop if the token has been set by k8s, continue otherwise.
		_, exists := tokenSecret.Data["token"]
		return exists, nil

	}); err != nil {
		return "", fmt.Errorf("unable to create service account token secret: %w", err)
	}

	tokenSecretValue := tokenSecret.Data["token"]
	return string(tokenSecretValue), nil

}

func createServiceAccountTokenSecret(ctx context.Context, k8sClient client.Client, serviceAccountName, serviceAccountNS string) (*corev1.Secret, error) {

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: serviceAccountName,
			Namespace:    serviceAccountNS,
			Annotations: map[string]string{
				corev1.ServiceAccountNameKey: serviceAccountName,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	if err := k8sClient.Create(ctx, tokenSecret); err != nil {
		return nil, err
	}

	return tokenSecret, nil
}

const (
	ArgoCDManagerClusterRoleNamePrefix        = "argocd-manager-cluster-role-"
	ArgoCDManagerClusterRoleBindingNamePrefix = "argocd-manager-cluster-role-binding-"
)

var (
	ArgoCDManagerNamespacePolicyRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}
)

func CreateOrUpdateClusterRoleAndRoleBinding(ctx context.Context, uuid string, k8sClient client.Client,
	serviceAccountName string, serviceAccountNamespace string, policyRules []rbacv1.PolicyRule) error {

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleNamePrefix + uuid,
		},
	}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole); err != nil {

		clusterRole.Rules = policyRules
		if err := k8sClient.Create(ctx, clusterRole); err != nil {
			return fmt.Errorf("unable to create clusterrole: %w", err)
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleBindingNamePrefix + uuid,
		},
	}

	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     clusterRole.Name,
	}

	clusterRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      serviceAccountName,
		Namespace: serviceAccountNamespace,
	}}

	if err := k8sClient.Create(ctx, clusterRoleBinding); err != nil {
		return fmt.Errorf("unable to create clusterrole: %w", err)
	}

	return nil
}

// Generate generic fake kube config
func GenerateFakeKubeConfig() string {
	// This config has been sanitized of any real credentials.
	return `
apiVersion: v1
kind: Config
clusters:
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api.fake-e2e-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
  - cluster:
      insecure-skip-tls-verify: true
      server: https://api2.fake-e2e-test-data.origin-ci-int-gce.dev.rhcloud.com:6443
    name: api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
contexts:
  - context:
      cluster: api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
  - context:
      cluster: api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
      namespace: jgw
      user: kube:admin/api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    name: default/api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
current-context: default/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443/kube:admin
preferences: {}
users:
  - name: kube:admin/api-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      token: sha256~ABCdEF1gHiJKlMnoP-Q19qrTuv1_W9X2YZABCDefGH4
  - name: kube:admin/api2-fake-e2e-test-data-origin-ci-int-gce-dev-rhcloud-com:6443
    user:
      token: sha256~abcDef1gHIjkLmNOp-q19QRtUV1_w9x2yzabcdEFgh4
`
}

func GenerateKubeConfig(serverURL string, currentNamespace string, token string) string {

	return `
apiVersion: v1
kind: Config
clusters:
  - cluster:
      insecure-skip-tls-verify: true
      server: ` + serverURL + `
    name: cluster-name
contexts:
  - context:
      cluster: cluster-name
      namespace: ` + currentNamespace + `
      user: user-name
    name: context-name
current-context: context-name
preferences: {}
users:
  - name: user-name
    user:
      token: ` + token + `
`
}

// HasFinalizers matches on whether or not a K8s object has the given finalizer(s)s
func HasFinalizers(expectedFinalizers []string, k8sClient client.Client) matcher.GomegaMatcher {
	return WithTransform(func(k8sObj client.Object) bool {

		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObj), k8sObj); err != nil {
			fmt.Println(K8sClientError, err)
			return false
		}

		for _, expectedFinalizer := range expectedFinalizers {

			match := true

			for _, finalizer := range k8sObj.GetFinalizers() {

				if finalizer == expectedFinalizer {
					match = true
					break
				}
			}

			if !match {
				fmt.Println("Finalizer", expectedFinalizers, "not found on", k8sObj.GetName())
				return false
			}
		}

		return true
	}, BeTrue())
}

func HasAnnotation(key, value string, k8sClient client.Client) matcher.GomegaMatcher {
	return WithTransform(func(k8sObj client.Object) bool {

		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObj), k8sObj)
		if err != nil {
			fmt.Println(K8sClientError, err)
			return false
		}

		annotations := k8sObj.GetAnnotations()
		if annotations == nil {
			fmt.Printf("Annotation %s not found in %s\n", key, k8sObj.GetName())
			return false
		}

		v, found := annotations[key]
		if !found {
			fmt.Printf("Annotation %s not found in %s\n", key, k8sObj.GetName())
			return false
		}

		if v != value {
			fmt.Printf("Annotation value mismatch for %s: Expected: %s Actual %s\n", k8sObj.GetName(), value, v)
			return false
		}

		return true
	}, BeTrue())
}

func HasLabel(key, value string, k8sClient client.Client) matcher.GomegaMatcher {
	return WithTransform(func(k8sObj client.Object) bool {

		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObj), k8sObj)
		if err != nil {
			fmt.Println(K8sClientError, err)
			return false
		}

		labels := k8sObj.GetLabels()
		if labels == nil {
			fmt.Printf("Label %s not found in %s\n", key, k8sObj.GetName())
			return false
		}

		v, found := labels[key]
		if !found {
			fmt.Printf("Label %s not found in %s\n", key, k8sObj.GetName())
			return false
		}

		if v != value {
			fmt.Printf("Label value mismatch for %s: Expected: %s Actual %s\n", k8sObj.GetName(), value, v)
			return false
		}

		return true
	}, BeTrue())
}

// HasNonNilDeletionTimestamp checks whether the Kubernetes object has a non-nil deletion timestamp.
func HasNonNilDeletionTimestamp(k8sClient client.Client) matcher.GomegaMatcher {
	return WithTransform(func(k8sObj client.Object) bool {

		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObj), k8sObj)
		if err != nil {
			fmt.Println("HasNonNilDeletionTimestamp:", K8sClientError, err)
			return false
		}

		return k8sObj.GetDeletionTimestamp() != nil
	}, BeTrue())
}

func GenerateReadOnlyClusterRoleandBinding(user string) (rbacv1.ClusterRole, rbacv1.ClusterRoleBinding) {

	clusterRole := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "read-all-" + user,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"get", "list", "watch"},
			Resources: []string{"*"},
			APIGroups: []string{"*"},
		}},
	}

	clusterRoleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "read-all-" + user,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Namespace: user,
			Name:      user + "-sa",
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
	}

	return clusterRole, clusterRoleBinding
}

func CreateNamespaceScopedUserAccount(ctx context.Context, user string, createReadOnlyClusterRoleBinding bool,
	k8sClient client.Client, log logr.Logger) (string, error) {

	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: user,
		},
	}
	if err := k8sClient.Create(ctx, &ns); err != nil {
		return "", fmt.Errorf("error on creating namespace: %v", err)
	}

	saName := user + "-sa"

	sa, err := GetOrCreateServiceAccount(ctx, k8sClient, saName, ns.Name, log)
	if err != nil {
		return "", fmt.Errorf("error on get/create service account: %v", err)
	}

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-all",
			Namespace: ns.Name,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"*"},
			Resources: []string{"*"},
			APIGroups: []string{"*"},
		}},
	}
	if err := k8sClient.Create(ctx, &role); err != nil {
		return "", fmt.Errorf("error on creating role: %v", err)
	}

	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-all-binding",
			Namespace: ns.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      sa.Name,
			Namespace: sa.Namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
	}
	if err := k8sClient.Create(ctx, &roleBinding); err != nil {
		return "", fmt.Errorf("error on creating rolebinding: %v", err)
	}

	if createReadOnlyClusterRoleBinding {
		clusterRole, clusterRoleBinding := GenerateReadOnlyClusterRoleandBinding(user)

		if err := k8sClient.Create(ctx, &clusterRole); err != nil {
			return "", fmt.Errorf("error on creating role: %v", err)
		}

		if err := k8sClient.Create(ctx, &clusterRoleBinding); err != nil {
			return "", fmt.Errorf("error on creating role: %v", err)
		}
	}

	token, err := CreateServiceAccountBearerToken(ctx, k8sClient, sa.Name, ns.Name)
	if err != nil {
		return "", fmt.Errorf("error on getting bearer token: %v", err)
	}

	return token, nil
}

func GetOrCreateServiceAccount(ctx context.Context, k8sClient client.Client, serviceAccountName string, serviceAccountNS string,
	log logr.Logger) (*corev1.ServiceAccount, error) {

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: serviceAccountNS,
		},
	}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount); err != nil {
		if !apierr.IsNotFound(err) {
			return nil, fmt.Errorf("unable to retrieve service account '%s': %w", serviceAccount.Name, err)
		}
	} else {
		// Found it, so just return it
		return serviceAccount, nil
	}

	log = log.WithValues("serviceAccount", serviceAccountName, "namespace", serviceAccountNS)

	if err := k8sClient.Create(ctx, serviceAccount); err != nil {
		log.Error(err, "Unable to create ServiceAccount")
		return nil, fmt.Errorf("unable to create service account '%s': %w", serviceAccount.Name, err)
	}

	log.Info(fmt.Sprintf("ServiceAccount %s created in namespace %s", serviceAccountName, serviceAccountNS))

	return serviceAccount, nil
}
