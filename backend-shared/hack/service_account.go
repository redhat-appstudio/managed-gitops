package hack

import (
	"context"
	"fmt"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ArgoCDManagerServiceAccount     = "argocd-manager"
	ArgoCDManagerClusterRole        = "argocd-manager-role"
	ArgoCDManagerClusterRoleBinding = "argocd-manager-role-binding"
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

func restrictServiceAccount(clientset kubernetes.Interface, ns string, namespace string) error {
	role := rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRole,
		},
		Rules: ArgoCDManagerNamespacePolicyRules,
	}
	_, err := clientset.RbacV1().Roles(namespace).Create(context.Background(), &role, metav1.CreateOptions{})
	if err != nil {
		if !apierr.IsAlreadyExists(err) {
			_, err := clientset.RbacV1().Roles(namespace).Update(context.Background(), &role, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	roleBinding := rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleBinding,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     ArgoCDManagerClusterRole,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      ArgoCDManagerServiceAccount,
			Namespace: ns,
		}},
	}
	_, err = clientset.RbacV1().RoleBindings(namespace).Create(context.Background(), &roleBinding, metav1.CreateOptions{})
	if err != nil {
		if !apierr.IsAlreadyExists(err) {
			_, err := clientset.RbacV1().RoleBindings(namespace).Update(context.Background(), &roleBinding, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func CreateServiceAccount(clientset kubernetes.Interface, serviceAccountName string, namespace string) error {
	serviceAccount := corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "v1",
			APIVersion: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(context.Background(), &serviceAccount, metav1.CreateOptions{})
	if err != nil {
		if !apierr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create service account %q in namespace %q: %v", serviceAccountName, namespace, err)
		}
		return fmt.Errorf("serviceAccount %q already exists in namespace %q", serviceAccountName, namespace)
	}

	log.Printf("serviceAccount %q created in namespace %q", serviceAccountName, namespace)
	return nil
}

func InstallServiceAccount(clientset kubernetes.Interface, ns string, namespaces []string) (string, error) {
	// ArgoCDManagerServiceAccount = "argocd-manager", is the name of the service account for managing a cluster

	err := CreateServiceAccount(clientset, "argocd-manager", ns)
	if err != nil {
		return "", err
	}
	for _, namespace := range namespaces {
		err := restrictServiceAccount(clientset, ns, namespace)
		if err != nil {
			return "", nil
		}
	}
	return GetServiceAccountBearerToken(clientset, ns, ArgoCDManagerServiceAccount)
}

// GetServiceAccountBearerToken will attempt to get the provided service account until it
// exists, iterate the secrets associated with it looking for one of type
// kubernetes.io/service-account-token, and return it's token if found.
func GetServiceAccountBearerToken(clientset kubernetes.Interface, ns string, sa string) (string, error) {
	var serviceAccount *corev1.ServiceAccount
	var secret *corev1.Secret
	var err error
	err = wait.Poll(500*time.Millisecond, 30*time.Second, func() (bool, error) {
		serviceAccount, err = clientset.CoreV1().ServiceAccounts(ns).Get(context.Background(), sa, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// Scan all secrets looking for one of the correct type:
		for _, oRef := range serviceAccount.Secrets {
			var getErr error
			secret, err = clientset.CoreV1().Secrets(ns).Get(context.Background(), oRef.Name, metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("failed to retrieve secret %q: %v", oRef.Name, getErr)
			}
			if secret.Type == corev1.SecretTypeServiceAccountToken {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to wait for service account secret: %v", err)
	}
	token, ok := secret.Data["token"]
	if !ok {
		return "", fmt.Errorf("secret %q for service account %q did not have a token", secret.Name, serviceAccount)
	}
	return string(token), nil
}

func generateClientFromClusterServiceAccount(configParam *rest.Config, bearerToken string) (client.Client, error) {

	newConfig := *configParam
	newConfig.BearerToken = bearerToken

	clientObj, err := client.New(&newConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return clientObj, nil
}
