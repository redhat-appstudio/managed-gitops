package k8s

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
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

func ExistByServiceName() matcher.GomegaMatcher {
	return WithTransform(func(k8sObject client.Object) bool {

		kubeClientSet, err := fixture.GetKubeClientSet()
		if err != nil {
			return false
		}

		_, err = kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-dex-server", metav1.GetOptions{})
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
		}

		_, err = kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-metrics", metav1.GetOptions{})
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
		}

		_, err = kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-redis", metav1.GetOptions{})
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
		}

		_, err = kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-repo-server", metav1.GetOptions{})
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
		}

		_, err = kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-server", metav1.GetOptions{})
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
		}

		_, err = kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-server-metrics", metav1.GetOptions{})
		if err != nil {
			fmt.Println(K8sClientError, "Object does not exists in ExistByServiceName:", k8sObject.GetName(), err)
		} else {
			fmt.Println("Object exists in ExistByServiceName:", k8sObject.GetName())
		}

		return err == nil
	}, BeTrue())
}

// (ctx context.Context, name string, namespace string, kubeClientSet *kubernetes.Clientset) error {
// 	err := wait.Poll(time.Second*1, time.Minute*2, func() (done bool, err error) {

// 		_, err1 := kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-dex-server", metav1.GetOptions{})
// 		_, err2 := kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-metrics", metav1.GetOptions{})
// 		_, err3 := kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-redis", metav1.GetOptions{})
// 		_, err4 := kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-repo-server", metav1.GetOptions{})
// 		_, err5 := kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-server", metav1.GetOptions{})
// 		_, err6 := kubeClientSet.CoreV1().Services(k8sObject.GetNamespace()).Get(context.Background(), k8sObject.GetName()+"-server-metrics", metav1.GetOptions{})
// 		if err1 != nil {
// 			if apierr.IsNotFound(err1) {
// 				return true, nil
// 			} else {
// 				return false, err1
// 			}
// 		}
// 		if err2 != nil {
// 			if apierr.IsNotFound(err2) {
// 				return true, nil
// 			} else {
// 				return false, err2
// 			}
// 		}
// 		if err3 != nil {
// 			if apierr.IsNotFound(err3) {
// 				return true, nil
// 			} else {
// 				return false, err3
// 			}
// 		}
// 		if err4 != nil {
// 			if apierr.IsNotFound(err4) {
// 				return true, nil
// 			} else {
// 				return false, err4
// 			}
// 		}
// 		if err5 != nil {
// 			if apierr.IsNotFound(err5) {
// 				return true, nil
// 			} else {
// 				return false, err5
// 			}
// 		}
// 		if err6 != nil {
// 			if apierr.IsNotFound(err6) {
// 				return true, nil
// 			} else {
// 				return false, err6
// 			}
// 		}

// 		return false, nil
// 	})

// 	if err != nil {
// 		fmt.Println("wait.Poll error for Services : ", err)
// 		return err
// 	}
// 	return nil
// }
