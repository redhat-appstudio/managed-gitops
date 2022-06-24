package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	openshiftv1 "github.com/openshift/api/route/v1"
	v1 "github.com/openshift/api/route/v1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ArgoCDManagerServiceAccount     = "argocd-manager"
	ArgoCDManagerClusterRole        = "argocd-manager-role"
	ArgoCDManagerClusterRoleBinding = "argocd-manager-role-binding"
	ArgoCDNamespace                 = "kube-system"
	// K8sClientError is a prefix that can/should be used when outputting errors from K8s client
	K8sClientError    = "Error from k8s client:"
	ClusterSecretName = "my-cluster"
)

type ClusterSecretTLSClientConfigJSON struct {
	Insecure bool `json:"insecure"`
}
type ClusterSecretConfigJSON struct {
	BearerToken     string                           `json:"bearerToken"`
	TLSClientConfig ClusterSecretTLSClientConfigJSON `json:"tlsClientConfig"`
}

func CreateNamespaceScopedArgoCD(ctx context.Context, name string, namespace string, k8sClient client.Client, log logr.Logger) error {
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
	config, err := ctrl.GetConfig()
	if err != nil {
		return err
	}
	config.Timeout = time.Duration(10 * time.Second)

	namespaceToCreate := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err = k8sClient.Create(ctx, namespaceToCreate)
	if err != nil {
		return fmt.Errorf("Namespace could not be created: %v", err)
	}

	errk8s := k8sClient.Create(ctx, &argoCDOperand)
	if errk8s != nil {
		return fmt.Errorf("Error on creating: %s, %v ", argoCDOperand.GetName(), errk8s)
	}

	// Wait for Argo CD to be installed by gitops operator. Use wait.Poll for ths

	err = wait.Poll(1*time.Second, 3*time.Minute, func() (bool, error) {

		appProject := &appv1.AppProject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: namespace,
			},
		}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(appProject), appProject)
		if err != nil {
			if apierr.IsNotFound(err) {
				log.V(sharedutil.LogLevel_Debug).Info("Waiting for AppProject to exist in namespace " + namespace)
				return false, nil
			} else {
				log.Error(err, "unable to retrieve AppProject")
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

func SetupArgoCD(ctx context.Context, argoCDNamespace string, k8sClient client.Client, log logr.Logger) error {

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerServiceAccount,
			Namespace: ArgoCDNamespace,
		},
	}

	err := k8sClient.Create(context.Background(), serviceAccount, &client.CreateOptions{})
	if err != nil {
		// If it already exists, then we update it
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(context.Background(), serviceAccount); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	log.Info(fmt.Sprintf("serviceAccount %q created in namespace %q", serviceAccount.Name, serviceAccount.Namespace))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-manager-secret",
			Namespace: ArgoCDNamespace,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": "argocd-manager",
			},
		},
		Type: "kubernetes.io/service-account-token",
	}

	if err := k8sClient.Create(context.Background(), secret); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(context.Background(), secret); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}

	log.Info(fmt.Sprintf("secret %q created in namespace %q", secret.Name, secret.Namespace))

	clusterRole := rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerClusterRole,
			Namespace: ArgoCDNamespace,
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

	if err := k8sClient.Create(context.Background(), &clusterRole); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(context.Background(), &clusterRole); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	log.Info(fmt.Sprintf("clusterRole %q created in namespace %q", clusterRole.Name, clusterRole.Namespace))

	clusterRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerClusterRoleBinding,
			Namespace: ArgoCDNamespace,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ArgoCDManagerClusterRole,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ArgoCDManagerServiceAccount,
				Namespace: ArgoCDNamespace,
			},
		},
	}

	if err := k8sClient.Create(context.Background(), &clusterRoleBinding); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(context.Background(), &clusterRoleBinding); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	log.Info(fmt.Sprintf("clusterRoleBinding %q created in namespace %q", clusterRoleBinding.Name, clusterRoleBinding.Namespace))

	err = wait.Poll(1*time.Second, 3*time.Minute, func() (bool, error) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argocd-manager-secret",
				Namespace: ArgoCDNamespace,
				Annotations: map[string]string{
					"kubernetes.io/service-account.name": "argocd-manager",
				},
			},
			Type: "kubernetes.io/service-account-token",
		}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		if err != nil {
			if apierr.IsNotFound(err) {
				log.V(sharedutil.LogLevel_Debug).Info("Waiting for Secret to exist in namespace " + secret.Namespace)
				return false, nil
			} else {
				log.Error(err, "unable to retrieve Secret")
				return false, err
			}
		} else {
			if string(secret.Data["token"]) == "" {
				log.Error(err, "Token byte stream is empty")
			}
			return true, nil
		}
	})
	if err != nil {
		fmt.Println("wait.Poll error : ", err)
		return err
	}

	// err = k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	// if err != nil {
	// 	return err
	// }

	// token := secret.Data["token"]  not sure whether this will work
	token := secret.Data["token"]

	// no need to decode token, it is unmarshalled from base64

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

	if err := k8sClient.Create(context.Background(), clusterSecret); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(context.Background(), clusterSecret); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}

	log.Info(fmt.Sprintf("cluster secret %q created ", clusterSecret.Name))
	return nil
}
