package k8s

import (
	"context"
	"fmt"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
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
func Create(obj client.Object) error {

	k8sClient, err := fixture.GetKubeClient()
	if err != nil {
		return err
	}

	if err := k8sClient.Create(context.Background(), obj); err != nil {
		fmt.Println(K8sClientError, "Error on creating ", obj.GetName(), err)
		return err
	}

	return nil
}

// Get the given K8s resource, returning an error on failure, or nil otherwise.
func Get(obj client.Object) error {

	k8sClient, err := fixture.GetKubeClient()
	if err != nil {
		return err
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
		fmt.Println(K8sClientError, "Unable to Get ", obj.GetName(), err)
		return err
	}

	return nil
}

// ExistByName checks if the given resource exists, when retrieving it by name/namespace.
// Does NOT check if the resource content matches.
func ExistByName() matcher.GomegaMatcher {

	return WithTransform(func(k8sObject client.Object) bool {

		k8sClient, err := fixture.GetKubeClient()
		if err != nil {
			return false
		}

		err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sObject), k8sObject)
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByName:", k8sObject.GetName())
		}
		return err == nil
	}, BeTrue())
}

// Delete deletes a K8s object from the namespace; it returns an error on failure, or nil otherwise.
func Delete(obj client.Object) error {

	k8sClient, err := fixture.GetKubeClient()
	if err != nil {
		return err
	}

	err = k8sClient.Delete(context.Background(), obj)
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

func UpdateStatus(obj client.Object) error {
	k8sClient, err := fixture.GetKubeClient()
	if err != nil {
		return err
	}

	if err = k8sClient.Status().Update(context.Background(), obj); err != nil {
		fmt.Println(K8sClientError, "Error on Status Update : ", err)
		return err
	}

	return nil
}

func Update(obj client.Object) error {
	k8sClient, err := fixture.GetKubeClient()
	if err != nil {
		return err
	}

	if err := k8sClient.Update(context.Background(), obj, &client.UpdateOptions{}); err != nil {
		fmt.Println(K8sClientError, "Error on updating ", err)
		return err
	}

	return nil
}
