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

func getOrCreateServiceAccount(ctx context.Context, k8sClient client.Client, serviceAccountName string, serviceAccountNS string,
	log logr.Logger) (*corev1.ServiceAccount, error) {

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

	log = log.WithValues("serviceAccount", serviceAccountName, "namespace", serviceAccountNS)

	if err := k8sClient.Create(ctx, serviceAccount); err != nil {
		log.Error(err, "Unable to create ServiceAccount")
		return nil, fmt.Errorf("unable to create service account '%s': %v", serviceAccount.Name, err)
	}
	LogAPIResourceChangeEvent(serviceAccount.Namespace, serviceAccount.Name, serviceAccount, ResourceCreated, log)

	log.Info(fmt.Sprintf("ServiceAccount %s created in namespace %s", serviceAccountName, serviceAccountNS))

	return serviceAccount, nil
}

// GenerateServiceAccountName encapsulates the logic of what name to use when creating a ServiceAccount for Argo CD to use.
func GenerateServiceAccountName(uuid string) string {
	return ArgoCDManagerServiceAccountPrefix + uuid
}

func InstallServiceAccount(ctx context.Context, k8sClient client.Client, uuid string, serviceAccountNS string, log logr.Logger) (string, *corev1.ServiceAccount, error) {

	serviceAccountName := GenerateServiceAccountName(uuid)

	sa, err := getOrCreateServiceAccount(ctx, k8sClient, serviceAccountName, serviceAccountNS, log)
	if err != nil {
		return "", nil, fmt.Errorf("unable to create or update service account: %v", serviceAccountName)
	}

	if err := createOrUpdateClusterRoleAndRoleBinding(ctx, uuid, k8sClient, serviceAccountName, serviceAccountNS, log); err != nil {
		return "", nil, fmt.Errorf("unable to create or update role and cluster role binding: %v", err)
	}

	token, err := getOrCreateServiceAccountBearerToken(ctx, k8sClient, serviceAccountName, serviceAccountNS, log)
	if err != nil {
		return "", nil, err
	}

	return token, sa, nil
}

// getOrCreateServiceAccountBearerToken returns a token if there is an existing token secret for a service account.
// If the token secret is missing, it creates a new secret and attach it to the service account
func getOrCreateServiceAccountBearerToken(ctx context.Context, k8sClient client.Client, serviceAccountName string,
	serviceAccountNS string, log logr.Logger) (string, error) {

	tokenSecret, err := createServiceAccountTokenSecret(ctx, k8sClient, serviceAccountName, serviceAccountNS, log)
	if err != nil {
		return "", fmt.Errorf("failed to create a token secret for service account %s: %w", serviceAccountName, err)
	}

	if err := wait.Poll(time.Second*1, time.Second*120, func() (bool, error) {

		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret); err != nil {
			log.Error(err, "unable to retrieve token secret for service account", "serviceAccountName", serviceAccountName)
		}

		// Exit the loop if the token has been set by k8s, continue otherwise.
		_, exists := tokenSecret.Data["token"]
		return exists, nil

	}); err != nil {
		return "", fmt.Errorf("unable to create service account token secret: %v", err)
	}

	tokenSecretValue := tokenSecret.Data["token"]
	return string(tokenSecretValue), nil

}

func getServiceAccountTokenSecret(ctx context.Context, k8sClient client.Client, serviceAccount *corev1.ServiceAccount) (*corev1.Secret, error) {
	secrets := &corev1.SecretList{}
	ns := serviceAccount.Namespace
	opts := []client.ListOption{
		client.InNamespace(ns),
	}

	err := k8sClient.List(ctx, secrets, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secrets in namespace: %s: %v", ns, err)
	}

	for _, oRef := range secrets.Items {
		var getErr error
		innerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oRef.Name,
				Namespace: serviceAccount.Namespace,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(innerSecret), innerSecret); err != nil {
			return nil, fmt.Errorf("failed to retrieve secret %q: %v", oRef.Name, getErr)
		}

		if innerSecret.Type == corev1.SecretTypeServiceAccountToken && innerSecret.Annotations["kubernetes.io/service-account.uid"] == string(serviceAccount.UID) {
			return innerSecret, nil
		}
	}

	return nil, nil
}

func createServiceAccountTokenSecret(ctx context.Context, k8sClient client.Client, serviceAccountName, serviceAccountNS string,
	log logr.Logger) (*corev1.Secret, error) {

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

	log = log.WithValues("name", tokenSecret.Name, "namespace", tokenSecret.Namespace)

	if err := k8sClient.Create(ctx, tokenSecret); err != nil {
		log.Error(err, "Unable to create ServiceAccountToken Secret")
		return nil, err
	}
	LogAPIResourceChangeEvent(tokenSecret.Namespace, tokenSecret.Name, tokenSecret, ResourceCreated, log)

	return tokenSecret, nil
}

func createOrUpdateClusterRoleAndRoleBinding(ctx context.Context, uuid string, k8sClient client.Client,
	serviceAccountName string, serviceAccountNamespace string, log logr.Logger) error {

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleNamePrefix + uuid,
		},
	}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole); err != nil {

		if !apierr.IsNotFound(err) {
			return fmt.Errorf("unable to get cluster role: %v", err)
		}

		log := log.WithValues("name", clusterRole.Name)

		clusterRole.Rules = ArgoCDManagerNamespacePolicyRules
		if err := k8sClient.Create(ctx, clusterRole); err != nil {
			log.Error(err, "Unable to create ClusterRole")
			return fmt.Errorf("unable to create clusterrole: %v", err)
		}
		LogAPIResourceChangeEvent(clusterRole.Namespace, clusterRole.Name, clusterRole, ResourceCreated, log)

	} else {
		log := log.WithValues("name", clusterRole.Name)

		clusterRole.Rules = ArgoCDManagerNamespacePolicyRules
		if err := k8sClient.Update(ctx, clusterRole); err != nil {
			log.Error(err, "Unable to update ClusterRole")
			return fmt.Errorf("unable to update cluster role: %v", err)
		}
		LogAPIResourceChangeEvent(clusterRole.Namespace, clusterRole.Name, clusterRole, ResourceModified, log)
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleBindingNamePrefix + uuid,
		},
	}
	update := true
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding); err != nil {
		if !apierr.IsNotFound(err) {
			return fmt.Errorf("unable to get cluster role binding: %v", err)
		}
		update = false
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

	log = log.WithValues("name", clusterRoleBinding.Name)

	if update {
		if err := k8sClient.Update(ctx, clusterRoleBinding); err != nil {
			log.Error(err, "Unable to update ClusterRoleBinding")
			return fmt.Errorf("unable to create clusterrole: %v", err)
		}
		LogAPIResourceChangeEvent(clusterRoleBinding.Namespace, clusterRoleBinding.Name, clusterRoleBinding, ResourceModified, log)
	} else {
		if err := k8sClient.Create(ctx, clusterRoleBinding); err != nil {
			log.Error(err, "Unable to create ClusterRoleBinding")
			return fmt.Errorf("unable to create clusterrole: %v", err)
		}
		LogAPIResourceChangeEvent(clusterRoleBinding.Namespace, clusterRoleBinding.Name, clusterRoleBinding, ResourceCreated, log)
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
