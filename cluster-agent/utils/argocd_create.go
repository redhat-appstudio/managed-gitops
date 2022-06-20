package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	openshiftv1 "github.com/openshift/api/route/v1"
	v1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ArgoCDManagerServiceAccount     = "argocd-manager"
	ArgoCDManagerClusterRole        = "argocd-manager-role"
	ArgoCDManagerClusterRoleBinding = "argocd-manager-role-binding"
	// K8sClientError is a prefix that can/should be used when outputting errors from K8s client
	K8sClientError    = "Error from k8s client:"
	ClusterSecretName = "my-cluster"
)

func CreateNamespaceScopedArgoCD(ctx context.Context, name string, namespace string, k8sClient client.Client) error {
	policy := "g, system:authenticated, role:admin"
	scopes := "[groups]"

	// The values from manifests/staging-cluster-resources/argo-cd.yaml are conveeted in a Go struct.

	argoCDOperand := argocdoperator.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{"argoproj.io/finalizer"},
			Name:       name,
			Namespace:  namespace,
		},
		Spec: argocdoperator.ArgoCDSpec{

			ApplicationSet: &argocdoperator.ArgoCDApplicationSet{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			Controller: argocdoperator.ArgoCDApplicationControllerSpec{
				Processors: argocdoperator.ArgoCDApplicationControllerProcessorsSpec{},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				Sharding: argocdoperator.ArgoCDApplicationControllerShardSpec{},
			},
			Dex: argocdoperator.ArgoCDDexSpec{
				OpenShiftOAuth: false,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			Grafana: argocdoperator.ArgoCDGrafanaSpec{
				Enabled: false,
				Ingress: argocdoperator.ArgoCDIngressSpec{
					Enabled: false,
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				Route: argocdoperator.ArgoCDRouteSpec{
					Enabled: false,
				},
			},
			HA: argocdoperator.ArgoCDHASpec{
				Enabled: false,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			InitialSSHKnownHosts: argocdoperator.SSHHostsSpec{},
			Prometheus: argocdoperator.ArgoCDPrometheusSpec{
				Enabled: false,
				Ingress: argocdoperator.ArgoCDIngressSpec{
					Enabled: false,
				},
				Route: argocdoperator.ArgoCDRouteSpec{
					Enabled: false,
				},
			},
			RBAC: argocdoperator.ArgoCDRBACSpec{
				Policy: &policy,
				Scopes: &scopes,
			},
			Redis: argocdoperator.ArgoCDRedisSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			Repo: argocdoperator.ArgoCDRepoSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			Server: argocdoperator.ArgoCDServerSpec{
				Autoscale: argocdoperator.ArgoCDServerAutoscaleSpec{
					Enabled: false,
				},
				GRPC: argocdoperator.ArgoCDServerGRPCSpec{
					Ingress: argocdoperator.ArgoCDIngressSpec{
						Enabled: false,
					},
				},
				Ingress: argocdoperator.ArgoCDIngressSpec{
					Enabled: false,
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("125m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				Route: argocdoperator.ArgoCDRouteSpec{
					Enabled: true,
					TLS: &v1.TLSConfig{
						Termination: openshiftv1.TLSTerminationReencrypt,
					},
				},
				Service: argocdoperator.ArgoCDServerServiceSpec{
					Type: "",
				},
			},
			TLS: argocdoperator.ArgoCDTLSSpec{
				CA: argocdoperator.ArgoCDCASpec{},
			},
		},
	}

	// Creating namespace
	config := ctrl.GetConfigOrDie()
	config.Timeout = time.Duration(10 * time.Second)
	clientset := kubernetes.NewForConfigOrDie(config)

	namespaceToCreate := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	_, err := clientset.CoreV1().Namespaces().Create(context.Background(), namespaceToCreate, metav1.CreateOptions{})
	if err != nil {
		fmt.Println("Namespace could not be created")
		return err
	}

	errk8s := k8sClient.Create(ctx, &argoCDOperand)
	if errk8s != nil {
		fmt.Println(K8sClientError, "Error on creating ", argoCDOperand.GetName(), errk8s)
		return errk8s
	}

	// Wait for Argo CD to be installed by gitops operator. Use wait.Poll for ths

	err = wait.Poll(1*time.Second, 60*time.Second, func() (bool, error) {

		appProject := &appv1.AppProject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: namespace,
			},
		}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(appProject), appProject)
		if err != nil {
			if apierr.IsNotFound(err) {
				return false, nil
			} else {
				return false, err
			}
		} else {
			return true, nil
		}
	})

	if err != nil {
		fmt.Println("wait.Poll error : ", err)
		return err
	}

	return nil
}

func SetupArgoCD(argoCDNamespace string, k8sClient client.Client, kubeClientSet *kubernetes.Clientset) error {

	policy := metav1.DeletePropagationForeground

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-manager",
			Namespace: "kube-system",
		},
	}
	// Delete the resources, if it exists

	_, errOnGet := kubeClientSet.CoreV1().ServiceAccounts(serviceAccount.Namespace).Get(context.Background(), serviceAccount.Name, metav1.GetOptions{})
	if errOnGet == nil {
		errOnDelete := kubeClientSet.CoreV1().ServiceAccounts(serviceAccount.Namespace).Delete(context.Background(), serviceAccount.Name, metav1.DeleteOptions{PropagationPolicy: &policy})
		if errOnDelete != nil {
			return fmt.Errorf("error on DELETE %v", errOnDelete)
		}

	}

	if err := k8sClient.Create(context.Background(), serviceAccount); err != nil {
		if !apierr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create service account %q in namespace %q: %v", serviceAccount.Name, serviceAccount.Namespace, err)
		}
		return fmt.Errorf("serviceAccount %q already exists in namespace %q", serviceAccount.Name, serviceAccount.Namespace)
	}

	log.Printf("serviceAccount %q created in namespace %q", serviceAccount.Name, serviceAccount.Namespace)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-manager-secret",
			Namespace: "kube-system",
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": "argocd-manager",
			},
		},
		Type: "kubernetes.io/service-account-token",
	}

	if err := k8sClient.Create(context.Background(), secret); err != nil {
		if !apierr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create secret %q in namespace %q: %v", secret.Name, secret.Namespace, err)
		}
		return fmt.Errorf("secret %q already exists in namespace %q", secret.Name, secret.Namespace)
	}

	log.Printf("secret %q created in namespace %q", secret.Name, secret.Namespace)

	clusterRole := rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-manager-role",
			Namespace: "kube-system",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups:       []string{"*"},
				Resources:       []string{"*"},
				Verbs:           []string{"*"},
				NonResourceURLs: []string{},
			},
		},
	}

	// Delete the resources, if they exists

	_, errOnGet = kubeClientSet.RbacV1().ClusterRoles().Get(context.Background(), clusterRole.Name, metav1.GetOptions{})
	if errOnGet == nil {
		errOnDelete := kubeClientSet.RbacV1().ClusterRoles().Delete(context.Background(), clusterRole.Name, metav1.DeleteOptions{PropagationPolicy: &policy})
		if errOnDelete != nil {
			return fmt.Errorf("Error on DELETE %v", errOnDelete)
		}

	}
	if err := k8sClient.Create(context.Background(), &clusterRole); err != nil {
		if !apierr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create clusterRole %q in namespace %q: %v", clusterRole.Name, clusterRole.Namespace, err)
		}
		return fmt.Errorf("clusterRole %q already exists in namespace %q", clusterRole.Name, clusterRole.Namespace)
	}
	log.Printf("clusterRole %q created in namespace %q", clusterRole.Name, clusterRole.Namespace)

	clusterRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-manager-role-binding",
			Namespace: "kube-system",
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "argocd-manager-role",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "argocd-manager",
				Namespace: "kube-system",
			},
		},
	}

	// debug : cluster role binding doesn't respond to the below code, need to delete manually using kubectl.
	_, errOnGet = kubeClientSet.RbacV1().ClusterRoleBindings().Get(context.Background(), clusterRoleBinding.Name, metav1.GetOptions{})
	if errOnGet == nil {
		errOnDelete := kubeClientSet.RbacV1().ClusterRoleBindings().Delete(context.Background(), clusterRoleBinding.Name, metav1.DeleteOptions{PropagationPolicy: &policy})
		if errOnDelete != nil {
			return fmt.Errorf("Error on DELETE %v", errOnDelete)
		}
	}
	if err := k8sClient.Create(context.Background(), &clusterRoleBinding); err != nil {
		if !apierr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create clusterRoleBinding %q in namespace %q: %v", clusterRoleBinding.Name, clusterRoleBinding.Namespace, err)
		}
		return fmt.Errorf("clusterRoleBinding %q already exists in namespace %q", clusterRoleBinding.Name, clusterRoleBinding.Namespace)
	}
	log.Printf("clusterRoleBinding %q created in namespace %q", clusterRoleBinding.Name, clusterRoleBinding.Namespace)

	// 	apiVersion: v1
	// kind: Secret
	// metadata:
	//   name: my-cluster-secret
	//   labels:
	//     argocd.argoproj.io/secret-type: cluster
	// type: Opaque
	// stringData:
	//   name: my-cluster
	//   server: $API_URL
	//   config: |
	//     {
	//       "bearerToken": "$SECRET_TOKEN",
	//       "tlsClientConfig": {
	//         "insecure": true
	//       }
	//     }

	secretResource, secretErr := kubeClientSet.CoreV1().Secrets(secret.Namespace).Get(context.Background(), secret.Name, metav1.GetOptions{})
	if secretErr != nil {
		return secretErr
	}
	// token := secret.Data["token"]  not sure whether this will work
	token := secretResource.Data["token"]
	// no need to decode token, it is unmarshalled from base64

	// decodedToken, err := base64.StdEncoding.DecodeString(string(token))
	// if err != nil {
	// 	fmt.Printf("Error decoding Base64 encoded data %v", err)
	// }

	type ClusterSecretTLSClientConfigJSON struct {
		Insecure bool `json:"insecure"`
	}
	type ClusterSecretConfigJSON struct {
		BearerToken     string                           `json:"bearerToken"`
		TLSClientConfig ClusterSecretTLSClientConfigJSON `json:"tlsClientConfig"`
	}

	clusterSecretConfigJSON := ClusterSecretConfigJSON{
		BearerToken: string(token),
		TLSClientConfig: ClusterSecretTLSClientConfigJSON{
			Insecure: true,
		},
	}

	jsonString, err := json.Marshal(clusterSecretConfigJSON)
	if err != nil {
		return fmt.Errorf("SEVERE: unable to marshal JSON")
	}

	config, _ := ctrl.GetConfig()

	// fmt.Printf("HOST %v \n", config.Host)
	// fmt.Printf("TOKEN %v \n", string(token))

	clusterSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster-secret",
			Namespace: argoCDNamespace,
			Labels:    map[string]string{"argocd.argoproj.io/secret-type": "cluster"},
		},
		StringData: map[string]string{
			"name":   ClusterSecretName,
			"server": config.Host,
			"config": string(jsonString),
		},
		Type: corev1.SecretType("Opaque"),
	}
	//delete cluster secret if it already exists to create a new one
	_, errOnGet = kubeClientSet.CoreV1().Secrets(clusterSecret.Namespace).Get(context.Background(), clusterSecret.Name, metav1.GetOptions{})
	if errOnGet == nil {
		errOnDelete := kubeClientSet.CoreV1().Secrets(clusterSecret.Namespace).Delete(context.Background(), clusterSecret.Name, metav1.DeleteOptions{PropagationPolicy: &policy})
		if errOnDelete != nil {
			return fmt.Errorf("error on DELETE %v", errOnDelete)
		}

	}
	if err := k8sClient.Create(context.Background(), clusterSecret); err != nil {
		if !apierr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create cluster secret %q : %v", clusterSecret.Name, err)
		}
		return fmt.Errorf("cluster secret %q already exists", clusterSecret.Name)
	}

	log.Printf("cluster secret %q created ", clusterSecret.Name)
	return nil
}
