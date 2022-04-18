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

// Create creates the given K8s resource, returning an error on failure, or nil otherwise.
func Create(obj client.Object) error {

	client, err := fixture.GetKubeClient()
	if err != nil {
		return err
	}

	fmt.Println("Creating ", obj.GetName())

	if err := client.Create(context.Background(), obj); err != nil {
		fmt.Println("Error on creating ", obj.GetName(), err)
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
			fmt.Println("Object does not exists in ExistByName:", k8sObject.GetName(), err)
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
		fmt.Println("Unable to delete in Delete:", obj.GetName(), err)
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
