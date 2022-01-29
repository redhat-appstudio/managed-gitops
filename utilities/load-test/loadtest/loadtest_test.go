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
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	// Check for an API (For now it is asume that the kubeconfig og each user lies in ~/.kube/config)
	// masterUrl ->  "master url address", within the kubeconfig, there is a server address that is referenced for masterUrl
	// namespace -> predefined to be argocd with the assumpution that argocd is configured within same namespace
	// podName -> If any specific pod details required replace "" with the podName
	userLocal, _ = user.Current()
	kubeconfig   = userLocal.HomeDir + "/.kube/config"
	masterUrl    = ""
	namespace    = "argocd"
	podName      = ""
)

func TestNGuestbook(t *testing.T) {
	// TEST 1: Deply 100 guestbook in different namespace
	t.Log("Testing for 100 Guestbook applications\n")
	numberOfAppsToCreate := 100
	t.Log("\n\nPods Memory Resource before creation:\n\n")
	memoryInit := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryInit)

	// Get Pod Restart Count
	PodC := PodRestartCount(namespace)

	for i := 1; i < numberOfAppsToCreate+1; i++ {
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
					Namespace: "guestbook",
					Server:    "https://kubernetes.default.svc",
				},
				Project: "default",
				SyncPolicy: &appv1.SyncPolicy{
					Automated: &appv1.SyncPolicyAutomated{},
				},
			},
		}

		_, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), guestbookApp, v1.CreateOptions{})
		if err != nil {
			t.Errorf("error, %v", err)
			return
		}
	}

	// wait.Poll will keep checking whether a (app variable).Status.Health.Status condition is met
	err := wait.Poll(1*time.Second, 2*time.Minute, func() (bool, error) {

		// list all Argo CD applications in the namespace
		appList, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			t.Errorf("error, %v", err)
		}
		// for each application, check if (app variable).Status.Health.Status
		for _, app := range appList.Items {

			// if status is empty, return false
			if app.Status.Health.Status == "" {
				return false, nil
			}
		}
		return true, nil
	})

	if err != nil {
		t.Errorf("Timeout Error, the application weren't ready within the defined timestamp, %v", err)
		return
	}

	// Check if the Pod is getting refreshed or not
	if reflect.DeepEqual(PodC, PodRestartCount(namespace)) != true {
		t.Errorf("error, %v", "The Pods are getting refreshed!")
		return
	}

	t.Log("\n\nPods Memory Resource after Creation:\n\n")
	memoryPost := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryPost)

	t.Log("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
	delta := PodMemoryDiff(memoryInit, memoryPost)
	PodInfoParse(delta)

	// To delete a application from the given (ex: argocd) namespace
	for i := 1; i < numberOfAppsToCreate+1; i++ {
		err_del := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "guestbook-"+fmt.Sprintf("%d", i), v1.DeleteOptions{})

		if err_del != nil {
			t.Errorf("error, %v", err_del)
			return
		}
	}
	t.Log("100 guestbook applications are created, deployed and deleted successfully!")
}

func TestHeavyApplication(t *testing.T) {

	// TEST 2: Deply Prometheus + External DNS + Cert Manager application

	t.Log("Testing for Prometheus + External DNS + Cert Manager applications\n")

	t.Log("\n\nPods Memory Resource before creation:\n\n")
	// Intial Memory Information
	memoryInit := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryInit)

	// Get Pod Restart Count
	PodC := PodRestartCount(namespace)

	PrometheusApp := &appv1.Application{
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
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{},
			},
		},
	}

	_, err_createPrometheus := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), PrometheusApp, v1.CreateOptions{})
	if err_createPrometheus != nil {
		t.Errorf("error, %v", err_createPrometheus)
		return
	}

	CertManagerApp := &appv1.Application{
		TypeMeta: v1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "cert-manager-app",
			Namespace: "argocd",
		},
		Spec: appv1.ApplicationSpec{
			Source: appv1.ApplicationSource{
				Path:    "cert-manager",
				RepoURL: "https://github.com/samyak-jn/gitops-sample-apps.git",
				Helm: &appv1.ApplicationSourceHelm{
					Version: "v2",
				},
			},
			Destination: appv1.ApplicationDestination{
				Namespace: "cert-manager",
				Server:    "https://kubernetes.default.svc",
			},
			Project: "default",
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{},
			},
		},
	}

	_, err_createCertMan := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), CertManagerApp, v1.CreateOptions{})
	if err_createCertMan != nil {
		t.Errorf("error, %v", err_createCertMan)
		return
	}

	ExternalDNSApp := &appv1.Application{
		TypeMeta: v1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "external-dns-app",
			Namespace: "argocd",
		},
		Spec: appv1.ApplicationSpec{
			Source: appv1.ApplicationSource{
				Path: "external-dns",
				Helm: &appv1.ApplicationSourceHelm{
					Version: "v2",
				},
				RepoURL: "https://github.com/samyak-jn/gitops-sample-apps.git",
			},
			Destination: appv1.ApplicationDestination{
				Namespace: "external-dns",
				Server:    "https://kubernetes.default.svc",
			},
			Project: "default",
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{},
			},
		},
	}

	_, err_createDNS := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), ExternalDNSApp, v1.CreateOptions{})
	if err_createDNS != nil {
		t.Errorf("error, %v", err_createDNS)
		return
	}

	// wait.Poll will keep checking whether a (app variable).Status.Health.Status condition is met
	err := wait.Poll(1*time.Second, 2*time.Minute, func() (bool, error) {

		// list all Argo CD applications in the namespace
		appList, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			t.Errorf("error, %v", err)
		}
		// for each application, check if (app variable).Status.Health.Status
		for _, app := range appList.Items {

			// if status is empty, return false
			if app.Status.Health.Status == "" {
				return false, nil
			}
		}
		return true, nil
	})

	if err != nil {
		t.Errorf("Timeout Error, the application weren't ready within the defined timestamp, %v", err)
		return
	}

	// Check if the Pod is getting refreshed or not
	if reflect.DeepEqual(PodC, PodRestartCount(namespace)) != true {
		t.Errorf("error, %v", "The Pods are getting refreshed!")
		return
	}

	t.Log("\n\nPods Memory Resource after Creation:\n\n")
	memoryPost := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryPost)

	t.Log("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
	delta := PodMemoryDiff(memoryInit, memoryPost)
	PodInfoParse(delta)

	// To delete a application from the given (ex: argocd) namespace
	err_Prometheus := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "prometheus-app", v1.DeleteOptions{})
	if err_Prometheus != nil {
		t.Errorf("error, %v", err_Prometheus)
		return
	}
	err_ExternalDNS := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "external-dns-app", v1.DeleteOptions{})
	if err_ExternalDNS != nil {
		t.Errorf("error, %v", err_ExternalDNS)
		return
	}

	err_CertManager := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "cert-manager-app", v1.DeleteOptions{})
	if err_CertManager != nil {
		t.Errorf("error, %v", err_CertManager)
		return
	}
	t.Log("Prometheus, External DNS and Cert Manager applications are created, deployed and deleted successfully!")
}

func TestAllApplication(t *testing.T) {

	//	TEST 3: Deply Prometheus (heavy application) + 20 Guestbook Application (small application)

	t.Log("Testing for Prometheus + 20 Guestbook Application\n")
	numberOfGuestbookAppsToCreate := 20
	t.Log("\n\nPods Memory Resource before creation:\n\n")
	// Intial Memory Information
	memoryInit := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryInit)

	// Get Pod Restart Count
	PodC := PodRestartCount(namespace)

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
			SyncPolicy: &appv1.SyncPolicy{
				Automated: &appv1.SyncPolicyAutomated{},
			},
		},
	}

	_, err_create := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), PromethusApp, v1.CreateOptions{})
	if err_create != nil {
		t.Errorf("error, %v", err_create)
		return
	}

	for i := 1; i < numberOfGuestbookAppsToCreate+1; i++ {
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
					Namespace: "guestbook",
					Server:    "https://kubernetes.default.svc",
				},
				Project: "default",
				SyncPolicy: &appv1.SyncPolicy{
					Automated: &appv1.SyncPolicyAutomated{},
				},
			},
		}

		_, err_create := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), guestbookApp, v1.CreateOptions{})
		if err_create != nil {
			t.Errorf("error, %v", err_create)
			return
		}
	}

	// wait.Poll will keep checking whether a (app variable).Status.Health.Status condition is met
	err := wait.Poll(1*time.Second, 2*time.Minute, func() (bool, error) {

		// list all Argo CD applications in the namespace
		appList, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			t.Errorf("error, %v", err)
		}
		// for each application, check if (app variable).Status.Health.Status
		for _, app := range appList.Items {

			// if status is empty, return false
			if app.Status.Health.Status == "" {
				return false, nil
			}
		}
		return true, nil
	})

	if err != nil {
		t.Errorf("Timeout Error, the application weren't ready within the defined timestamp, %v", err)
		return
	}

	// Check if the Pod is getting refreshed or not
	if reflect.DeepEqual(PodC, PodRestartCount(namespace)) != true {
		t.Errorf("error, %v", "The Pods are getting refreshed!")
		return
	}

	t.Log("\n\nPods Memory Resource after Creation:\n\n")
	memoryPost := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryPost)

	t.Log("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
	delta := PodMemoryDiff(memoryInit, memoryPost)
	PodInfoParse(delta)

	// To delete a application from the given (ex: argocd) namespace
	err_del := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "prometheus-app", v1.DeleteOptions{})
	// Deleting 10 guestbook applications
	for i := 1; i < numberOfGuestbookAppsToCreate+1; i++ {
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
	t.Log("Prometheus, along with 20 guestbook applications are created, deployed and deleted successfully!")
}

func TestNResource(t *testing.T) {
	numberOfAppsToCreate := 100

	cleanup := func(numberOfAppsToCreate int) {
		// To delete a application from the given (ex: argocd) namespace and delete the test-namespace created
		for i := 1; i < numberOfAppsToCreate+1; i++ {
			err_del := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Delete(context.TODO(), "test-app-"+fmt.Sprintf("%d", i), v1.DeleteOptions{})
			if err_del != nil {
				t.Errorf("error, %v", err_del)
			}
			_ = GetE2EFixtureK8sClient().KubeClientset.CoreV1().Namespaces().Delete(context.TODO(), "test-namespace-"+fmt.Sprintf("%d", i), v1.DeleteOptions{})
		}
	}
	defer cleanup(numberOfAppsToCreate)

	// TEST: Deply 100 sample application in different namespace
	t.Log("Testing for 100 applications that uses lightweight non-Pod-based resources\n")

	t.Log("\n\nPods Memory Resource before creation:\n\n")
	memoryInit := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryInit)

	// Get Pod Restart Count
	PodC := PodRestartCount(namespace)

	for i := 1; i < numberOfAppsToCreate+1; i++ {
		applicationTest := &appv1.Application{
			TypeMeta: v1.TypeMeta{
				Kind:       "Application",
				APIVersion: "argoproj.io/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app-" + fmt.Sprintf("%d", i),
				Namespace: "argocd",
			},
			Spec: appv1.ApplicationSpec{
				Source: appv1.ApplicationSource{
					Path:           "sample-app",
					TargetRevision: "HEAD",
					RepoURL:        "https://github.com/samyak-jn/gitops-service-sample-k8s-resources.git",
				},
				Destination: appv1.ApplicationDestination{
					Namespace: "test-namespace-" + fmt.Sprintf("%d", i),
					Server:    "https://kubernetes.default.svc",
				},
				Project: "default",
				SyncPolicy: &appv1.SyncPolicy{
					Automated:   &appv1.SyncPolicyAutomated{},
					SyncOptions: []string{"CreateNamespace=true"},
				},
			},
		}
		_, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).Create(context.TODO(), applicationTest, v1.CreateOptions{})
		if err != nil {
			t.Errorf("error, %v", err)
			return
		}
	}

	// wait.Poll will keep checking whether a (app variable).Status.Health.Status condition is met
	err := wait.Poll(1*time.Second, 2*time.Minute, func() (bool, error) {

		// list all Argo CD applications in the namespace
		appList, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			t.Errorf("error, %v", err)
		}
		// for each application, check if (app variable).Status.Health.Status
		for _, app := range appList.Items {
			// if status is empty, return false
			if app.Status.Sync.Status != "Synced" && app.Status.Health.Status != "Healthy" {
				return false, nil
			}
		}
		return true, nil
	})

	if err != nil {
		t.Errorf("Timeout Error, the application weren't ready within the defined timestamp, %v", err)
		return
	}

	// Check if the Pod is getting refreshed or not
	if reflect.DeepEqual(PodC, PodRestartCount(namespace)) != true {
		t.Errorf("error, %v", "The Pods are getting refreshed!")
		return
	}

	t.Log("\n\nPods Memory Resource after Creation:\n\n")
	memoryPost := GetPodInfo(kubeconfig, masterUrl, namespace, podName)
	PodInfoParse(memoryPost)

	t.Log("\n\nDifference in the Pod Memory Usage (in Ki)\n\n")
	delta := PodMemoryDiff(memoryInit, memoryPost)
	PodInfoParse(delta)

	t.Log("100 applications are created, deployed and deleted successfully!")
}
