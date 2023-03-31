package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
			fmt.Println("waiting: ", err)
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
