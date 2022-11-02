package k8s

import (
	"context"
	"fmt"
	"time"

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
