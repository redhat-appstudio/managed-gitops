package loadtest

import (
	"context"
	"fmt"
	"os/user"
	"reflect"
	"testing"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// type appResource struct{
// 	AppInfo *appv1.Application
// 	Count int
// }

var (
	// Check for an API (For now it is asume that the kubeconfig og each user lies in ~/.kube/config)
	userLocal, _ = user.Current()
	kubeconfig   = userLocal.HomeDir + "/.kube/config"
	master       = ""
	namespace    = "argocd"
	podName      = ""
)

func TestNGuestbook(t *testing.T) {
	// TEST 1: Deply 100 guestbook in different namespace
	fmt.Print("Testing for 100 Guestbook applications\n")

	fmt.Print("\n\nPods Memory Resource before creation:\n\n")
	memoryInit := Get_pod_info(kubeconfig, master, namespace, podName)
	PodInfoParse(memoryInit)

	// Get Pod Restart Count
	PodC := PodRestartcount(namespace)

	for i := 1; i < 101; i++ {
		guestbookApp := &appv1.Application{
			TypeMeta: v1.TypeMeta{
				Kind:       "Application",
				APIVersion: "argoproj.io/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "guestbook-" + fmt.Sprintf("%d", i),
				Namespace: "argocd",
			},
			Spec: appv1.ApplicationSpec{
				Source: appv1.ApplicationSource{
					Path:           "guestbook",
					TargetRevision: "HEAD",
					RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
				},
				Destination: appv1.ApplicationDestination{
					Namespace: "guestbook-" + fmt.Sprintf("%d", i),
					Server:    "https://kubernetes.default.svc",
				},
				Project: "default",
			},
		}

		_, err_create := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), guestbookApp, v1.CreateOptions{})
		if err_create != nil {
			t.Errorf("error, %v", err_create)
			return
		}
	}

	// Check if the Pod is getting refreshed or not
	if reflect.DeepEqual(PodC, PodRestartcount(namespace)) != true {
		t.Errorf("error, %v", "The Pods are getting refreshed!")
		return
	}

	fmt.Print("\n\nPods Memory Resource after Creation:\n\n")
	memoryPost := Get_pod_info(kubeconfig, master, namespace, podName)
	PodInfoParse(memoryPost)

	fmt.Print("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
	delta := PodMemoryDiff(memoryInit, memoryPost)
	PodInfoParse(delta)

	// less use of generic sleep statements in automated tests
	duration := time.Duration(5) * time.Second
	time.Sleep(duration)

	// To delete a application from the given (ex: argocd) namespace
	for i := 1; i < 101; i++ {
		err_del := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "guestbook-"+fmt.Sprintf("%d", i), v1.DeleteOptions{})

		if err_del != nil {
			t.Errorf("error, %v", err_del)
			return
		}
	}
	fmt.Println("100 guestbook applications are created, deployed and deleted successfully!")
}

// func TestHeavyApplication(t *testing.T) {

// 	// TEST 2: Deply Prometheus + External DNS + Cert Manager application

// 	fmt.Print("Testing for Prometheus + External DNS + Cert Manager applications\n")

// 	fmt.Print("\n\nPods Memory Resource before creation:\n\n")
// 	// Intial Memory Information
// 	memoryInit := Get_pod_info(kubeconfig, master, namespace, podName)
// 	PodInfoParse(memoryInit)

// 	// Get Pod Restart Count
// 	PodC := PodRestartcount(namespace)

// 	PrometheusApp := &appv1.Application{
// 		TypeMeta: v1.TypeMeta{
// 			Kind:       "Application",
// 			APIVersion: "argoproj.io/v1alpha1",
// 		},
// 		ObjectMeta: v1.ObjectMeta{
// 			Name:      "prometheus-app",
// 			Namespace: "argocd",
// 		},
// 		Spec: appv1.ApplicationSpec{
// 			Source: appv1.ApplicationSource{
// 				RepoURL: "https://github.com/jgwest/argocd-example-apps.git",
// 				Path:    "appset-examples/cluster-addon/prometheus-operator",
// 				Helm: &appv1.ApplicationSourceHelm{
// 					Version: "v2",
// 				},
// 			},
// 			Destination: appv1.ApplicationDestination{
// 				Namespace: "prometheus-app",
// 				Server:    "https://kubernetes.default.svc",
// 			},
// 			Project: "default",
// 		},
// 	}

// 	_, err_createPrometheus := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), PrometheusApp, v1.CreateOptions{})
// 	if err_createPrometheus != nil {
// 		t.Errorf("error, %v", err_createPrometheus)
// 		return
// 	}

// 	CertManagerApp := &appv1.Application{
// 		TypeMeta: v1.TypeMeta{
// 			Kind:       "Application",
// 			APIVersion: "argoproj.io/v1alpha1",
// 		},
// 		ObjectMeta: v1.ObjectMeta{
// 			Name:      "cert-manager-app",
// 			Namespace: "argocd",
// 		},
// 		Spec: appv1.ApplicationSpec{
// 			Source: appv1.ApplicationSource{
// 				RepoURL: "https://github.com/jgwest/argocd-example-apps.git",
// 				Path:    "cert-manager",
// 				Helm: &appv1.ApplicationSourceHelm{
// 					Version: "v2",
// 				},
// 			},
// 			Destination: appv1.ApplicationDestination{
// 				Namespace: "cert-manager-app",
// 				Server:    "https://kubernetes.default.svc",
// 			},
// 			Project: "default",
// 		},
// 	}

// 	_, err_createCertMan := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), CertManagerApp, v1.CreateOptions{})
// 	if err_createCertMan != nil {
// 		t.Errorf("error, %v", err_createCertMan)
// 		return
// 	}

// 	// ExternalDNSApp := &appv1.Application{
// 	// 	TypeMeta: v1.TypeMeta{
// 	// 		Kind:       "Application",
// 	// 		APIVersion: "argoproj.io/v1alpha1",
// 	// 	},
// 	// 	ObjectMeta: v1.ObjectMeta{
// 	// 		Name:      "external-dns-app",
// 	// 		Namespace: "argocd",
// 	// 	},
// 	// 	Spec: appv1.ApplicationSpec{
// 	// 		Source: appv1.ApplicationSource{
// 	// 			RepoURL: "https://github.com/jgwest/argocd-example-apps.git",
// 	// 			Path:    "appset-examples/cluster-addon/prometheus-operator",
// 	// 			Helm: &appv1.ApplicationSourceHelm{
// 	// 				Version: "v2",
// 	// 			},
// 	// 		},
// 	// 		Destination: appv1.ApplicationDestination{
// 	// 			Namespace: "external-dns-app",
// 	// 			Server:    "https://kubernetes.default.svc",
// 	// 		},
// 	// 		Project: "default",
// 	// 	},
// 	// }

// 	// _, err_createDNS := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), ExternalDNSApp, v1.CreateOptions{})
// 	// if err_createDNS != nil {
// 	// 	t.Errorf("error, %v", err_createDNS)
// 	// 	return
// 	// }

// 	// Check if the Pod is getting refreshed or not
// 	if reflect.DeepEqual(PodC, PodRestartcount(namespace)) != true {
// 		t.Errorf("error, %v", "The Pods are getting refreshed!")
// 		return
// 	}

// 	fmt.Print("\n\nPods Memory Resource after Creation:\n\n")
// 	memoryPost := Get_pod_info(kubeconfig, master, namespace, podName)
// 	PodInfoParse(memoryPost)

// 	fmt.Print("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
// 	delta := PodMemoryDiff(memoryInit, memoryPost)
// 	PodInfoParse(delta)

// 	// less use of generic sleep statements in automated tests
// 	duration := time.Duration(10) * time.Second
// 	time.Sleep(duration)

// 	// To delete a application from the given (ex: argocd) namespace
// 	err_delPrometheus := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "prometheus-app", v1.DeleteOptions{})
// 	if err_delPrometheus != nil {
// 		t.Errorf("error, %v", err_delPrometheus)
// 		return
// 	}
// 	// err_delExternalDNS := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "external-dns-app", v1.DeleteOptions{})
// 	// if err_delExternalDNS != nil {
// 	// 	t.Errorf("error, %v", err_delExternalDNS)
// 	// 	return
// 	// }

// 	// err_delCertMan := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "cert-manager-app", v1.DeleteOptions{})
// 	// if err_delCertMan != nil {
// 	// 	t.Errorf("error, %v", err_delCertMan)
// 	// 	return
// 	// }
// 	fmt.Println("Prometheus, External DNS and Cert Manager applications are created, deployed and deleted successfully!")
// }

func TestAllApplication(t *testing.T) {

	// TEST 3: Deply Prometheus (heavy application) + 20 Guestbook Application (small application)

	fmt.Print("Testing for Prometheus + 20 Guestbook Application\n")

	fmt.Print("\n\nPods Memory Resource before creation:\n\n")
	// Intial Memory Information
	memoryInit := Get_pod_info(kubeconfig, master, namespace, podName)
	PodInfoParse(memoryInit)

	// Get Pod Restart Count
	PodC := PodRestartcount(namespace)

	PromethusApp := &appv1.Application{
		TypeMeta: v1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "prometheus-app",
			Namespace: "argocd",
		},
		Spec: appv1.ApplicationSpec{
			Source: appv1.ApplicationSource{
				RepoURL: "https://github.com/jgwest/argocd-example-apps.git",
				Path:    "appset-examples/cluster-addon/prometheus-operator",
				Helm: &appv1.ApplicationSourceHelm{
					Version: "v2",
				},
			},
			Destination: appv1.ApplicationDestination{
				Namespace: "prometheus-app",
				Server:    "https://kubernetes.default.svc",
			},
			Project: "default",
		},
	}

	_, err_create := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), PromethusApp, v1.CreateOptions{})
	if err_create != nil {
		t.Errorf("error, %v", err_create)
		return
	}

	for i := 1; i < 21; i++ {
		guestbookApp := &appv1.Application{
			TypeMeta: v1.TypeMeta{
				Kind:       "Application",
				APIVersion: "argoproj.io/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "guestbook-" + fmt.Sprintf("%d", i),
				Namespace: "argocd",
			},
			Spec: appv1.ApplicationSpec{
				Source: appv1.ApplicationSource{
					Path:           "guestbook",
					TargetRevision: "HEAD",
					RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
				},
				Destination: appv1.ApplicationDestination{
					Namespace: "guestbook-" + fmt.Sprintf("%d", i),
					Server:    "https://kubernetes.default.svc",
				},
				Project: "default",
			},
		}

		_, err_create := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), guestbookApp, v1.CreateOptions{})
		if err_create != nil {
			t.Errorf("error, %v", err_create)
			return
		}
	}

	// Check if the Pod is getting refreshed or not
	if reflect.DeepEqual(PodC, PodRestartcount(namespace)) != true {
		t.Errorf("error, %v", "The Pods are getting refreshed!")
		return
	}

	fmt.Print("\n\nPods Memory Resource after Creation:\n\n")
	memoryPost := Get_pod_info(kubeconfig, master, namespace, podName)
	PodInfoParse(memoryPost)

	fmt.Print("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
	delta := PodMemoryDiff(memoryInit, memoryPost)
	PodInfoParse(delta)

	// less use of generic sleep statements in automated tests
	duration := time.Duration(10) * time.Second
	time.Sleep(duration)

	// To delete a application from the given (ex: argocd) namespace
	err_del := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "prometheus-app", v1.DeleteOptions{})
	// Deleting 10 guestbook applications
	for i := 1; i < 21; i++ {
		err_del := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "guestbook-"+fmt.Sprintf("%d", i), v1.DeleteOptions{})

		if err_del != nil {
			t.Errorf("error, %v", err_del)
			return
		}
	}
	if err_del != nil {
		t.Errorf("error, %v", err_del)
		return
	}
	fmt.Println("Prometheus, along with 20 guestbook applications are created, deployed and deleted successfully!")
}
