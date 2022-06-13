package util

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ArgoCDManagerServiceAccountPrefix         = "argocd-manager-"
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

func getOrCreateServiceAccount(ctx context.Context, k8sClient client.Client, serviceAccountName string, serviceAccountNS string, log logr.Logger) (*corev1.ServiceAccount, error) {

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: serviceAccountNS,
		},
	}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount); err != nil {
		if !apierr.IsNotFound(err) {
			return nil, fmt.Errorf("unable to retrieve service account '%s': %v", serviceAccount.Name, err)
		}
	} else {
		// Found it, so just return it
		return serviceAccount, nil
	}

	if err := k8sClient.Create(ctx, serviceAccount); err != nil {
		return nil, fmt.Errorf("unable to create service account '%s': %v", serviceAccount.Name, err)
	}

	log.Info(fmt.Sprintf("serviceAccount %s created in namespace %s", serviceAccountName, serviceAccountNS))

	return serviceAccount, nil
}

func InstallServiceAccount(ctx context.Context, k8sClient client.Client, uuid string, serviceAccountNS string, log logr.Logger) (string, *corev1.ServiceAccount, error) {

	serviceAccountName := ArgoCDManagerServiceAccountPrefix + uuid

	sa, err := getOrCreateServiceAccount(ctx, k8sClient, serviceAccountName, serviceAccountNS, log)
	if err != nil {
		return "", nil, fmt.Errorf("unable to create service account: %v", serviceAccountName)
	}

	if err := createOrUpdateClusterRoleAndRoleBinding(ctx, uuid, k8sClient, serviceAccountName, serviceAccountNS); err != nil {
		return "", nil, fmt.Errorf("unable to create role and cluster role binding: %v", err)
	}

	token, err := getServiceAccountBearerToken(ctx, k8sClient, serviceAccountName, serviceAccountNS)
	if err != nil {
		return "", nil, err
	}

	return token, sa, nil
}

// GetServiceAccountBearerToken will attempt to get the provided service account until it
// exists, iterate the secrets associated with it looking for one of type
// kubernetes.io/service-account-token, and return it's token if found.
func getServiceAccountBearerToken(ctx context.Context, k8sClient client.Client, serviceAccountName string, serviceAccountNS string) (string, error) {

	var serviceAccount *corev1.ServiceAccount
	var secret *corev1.Secret
	if err := wait.Poll(500*time.Millisecond, 30*time.Second, func() (bool, error) {

		serviceAccount := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceAccountName,
				Namespace: serviceAccountNS,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount); err != nil {
			return false, err
		}

		// Scan all secrets looking for one of the correct type:
		for _, oRef := range serviceAccount.Secrets {
			var getErr error
			innerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      oRef.Name,
					Namespace: serviceAccountNS,
				},
			}

			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(innerSecret), innerSecret); err != nil {
				return false, fmt.Errorf("failed to retrieve secret %q: %v", oRef.Name, getErr)
			}
			if innerSecret.Type == corev1.SecretTypeServiceAccountToken {
				secret = innerSecret
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		return "", fmt.Errorf("failed to wait for service account secret: %v", err)
	}

	if secret == nil {
		return "", fmt.Errorf("unable to locate service account secret")
	}

	token, ok := secret.Data["token"]
	if !ok {
		return "", fmt.Errorf("secret %q for service account %q did not have a token", secret.Name, serviceAccount)
	}
	return string(token), nil
}

func createOrUpdateClusterRoleAndRoleBinding(ctx context.Context, uuid string, k8sClient client.Client,
	serviceAccountName string, serviceAccountNamespace string) error {

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleNamePrefix + uuid,
		},
		Rules: ArgoCDManagerNamespacePolicyRules,
	}
	if err := k8sClient.Create(ctx, clusterRole); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, clusterRole); err != nil {
				return fmt.Errorf("unable to update cluster role: %v", err)
			}
		} else {
			return fmt.Errorf("unable to create clusterrole: %v", err)
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleBindingNamePrefix + uuid,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      serviceAccountName,
			Namespace: serviceAccountNamespace,
		}},
	}
	if err := k8sClient.Create(ctx, clusterRoleBinding); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, clusterRoleBinding); err != nil {
				return fmt.Errorf("unable to update cluster role: %v", err)
			}
		} else {
			return fmt.Errorf("unable to create clusterrole: %v", err)
		}
	}

	return nil
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
