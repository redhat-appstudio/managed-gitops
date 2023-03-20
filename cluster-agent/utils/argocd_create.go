package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/settings"
	routev1 "github.com/openshift/api/route/v1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	/* #nosec */
	ArgoCDManagerSecretName             = "argocd-manager-secret"
	ArgoCDManagerServiceAccountName     = "argocd-manager"
	ArgoCDManagerClusterRoleName        = "argocd-manager-role"
	ArgoCDManagerClusterRoleBindingName = "argocd-manager-role-binding"
	KubeSystemNamespace                 = "kube-system"
	ClusterSecretName                   = "my-cluster"

	DefaultAppProject = "default"

	ArgoCDFinalizerName = "argoproj.io/finalizer"
)

type ClusterSecretTLSClientConfigJSON struct {
	Insecure bool `json:"insecure"`
}
type ClusterSecretConfigJSON struct {
	BearerToken     string                           `json:"bearerToken"`
	TLSClientConfig ClusterSecretTLSClientConfigJSON `json:"tlsClientConfig"`
}

// ReconcileNamespaceScopedArgoCD will create/update an ArgoCD operand within the specified namespace.
func ReconcileNamespaceScopedArgoCD(ctx context.Context, argocdCRName string, namespace string, k8sClient client.Client, log logr.Logger) error {
	policy := "g, system:authenticated, role:admin"
	scopes := "[groups]"

	resourceExclusions, err := yaml.Marshal([]settings.FilteredResource{
		{
			APIGroups: []string{"*.kcp.dev"},
			Clusters:  []string{"*"},
			Kinds:     []string{"*"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal resource exclusions: %v", err)
	}

	// The values from manifests/staging-cluster-resources/argo-cd.yaml are converted in a Go struct.

	expectedArgoCDOperand := &argocdoperator.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{ArgoCDFinalizerName},
			Name:       argocdCRName,
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
			Dex: &argocdoperator.ArgoCDDexSpec{
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
					TLS: &routev1.TLSConfig{
						Termination: routev1.TLSTerminationReencrypt,
					},
				},
				Service: argocdoperator.ArgoCDServerServiceSpec{
					Type: "",
				},
			},
			TLS: argocdoperator.ArgoCDTLSSpec{
				CA: argocdoperator.ArgoCDCASpec{},
			},
			ResourceExclusions: string(resourceExclusions),
		},
	}

	newArgoCDNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	// 1) Get the namespace, or create it if it doesn't already exist
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(newArgoCDNamespace), newArgoCDNamespace); err != nil {
		if apierr.IsNotFound(err) {
			// It doesn't exist, so create it
			if err := k8sClient.Create(ctx, newArgoCDNamespace); err != nil {
				return fmt.Errorf("while creating Argo CD instance, a namespace could not be created: %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(newArgoCDNamespace.Namespace, newArgoCDNamespace.Name, newArgoCDNamespace, sharedutil.ResourceCreated, log)
		} else {
			return fmt.Errorf("while creating Argo CD instance, an unexpected error on retrieving Namespace: %v", err)
		}
	}

	// 2) Retrieve the ArgoCD operand: if it doesn't exist, create it. If it does exist, update it.
	existingArgoCDOperand := &argocdoperator.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      argocdCRName,
			Namespace: newArgoCDNamespace.Name,
		},
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(existingArgoCDOperand), existingArgoCDOperand); err != nil {

		if apierr.IsNotFound(err) {
			// A) Operand doesn't exist, so create it
			if errk8s := k8sClient.Create(ctx, expectedArgoCDOperand); errk8s != nil {

				return fmt.Errorf("error on creating: %s, %v ", expectedArgoCDOperand.GetName(), errk8s)
			}
			sharedutil.LogAPIResourceChangeEvent(expectedArgoCDOperand.Namespace, expectedArgoCDOperand.Name, expectedArgoCDOperand,
				sharedutil.ResourceCreated, log)

		} else {
			log.Error(err, "unexpected error on retrieving ArgoCD operand")
			return fmt.Errorf("unexpected error on retrieving ArgoCD operand, %v", err)
		}

	} else {
		// B) The existing ArgoCD resource already exists, so make sure it is up to date
		if !reflect.DeepEqual(existingArgoCDOperand.Spec, expectedArgoCDOperand) {
			existingArgoCDOperand.Spec = expectedArgoCDOperand.Spec
			if err := k8sClient.Update(ctx, existingArgoCDOperand); err != nil {
				log.Error(err, "unexpected error on updating existing ArgoCD operand")
				return fmt.Errorf("unexpected error on updating existing ArgoCD operand, %v", err)
			}

			sharedutil.LogAPIResourceChangeEvent(existingArgoCDOperand.Namespace, existingArgoCDOperand.Name, existingArgoCDOperand,
				sharedutil.ResourceModified, log)
		}
	}

	// Wait for Argo CD to be installed by gitops operator.
	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {

		// 'default' AppProject will be created by Argo CD if Argo CD is successfully started.
		appProject := &appv1.AppProject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DefaultAppProject,
				Namespace: namespace,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(appProject), appProject); err != nil {
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

	return err

}

func SetupArgoCD(ctx context.Context, apiHost string, argoCDNamespace string, k8sClient client.Client, log logr.Logger) error {

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerServiceAccountName,
			Namespace: KubeSystemNamespace,
		},
	}

	if err := k8sClient.Create(ctx, serviceAccount, &client.CreateOptions{}); err != nil {
		// If it already exists, then we update it
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, serviceAccount); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(serviceAccount.Namespace, serviceAccount.Name, serviceAccount, sharedutil.ResourceModified, log)

		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	sharedutil.LogAPIResourceChangeEvent(serviceAccount.Namespace, serviceAccount.Name, serviceAccount, sharedutil.ResourceCreated, log)

	log.Info(fmt.Sprintf("serviceAccount %q created in namespace %q", serviceAccount.Name, serviceAccount.Namespace))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerSecretName,
			Namespace: KubeSystemNamespace,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": serviceAccount.Name,
			},
		},
		Type: "kubernetes.io/service-account-token",
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, secret); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(secret.Namespace, secret.Name, secret, sharedutil.ResourceModified, log)

		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	sharedutil.LogAPIResourceChangeEvent(secret.Namespace, secret.Name, secret, sharedutil.ResourceCreated, log)
	log.Info(fmt.Sprintf("secret %q created in namespace %q", secret.Name, secret.Namespace))

	clusterRole := rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerClusterRoleName,
			Namespace: KubeSystemNamespace,
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

	if err := k8sClient.Create(ctx, &clusterRole); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, &clusterRole); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(clusterRole.Namespace, clusterRole.Name, clusterRole, sharedutil.ResourceModified, log)

		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	sharedutil.LogAPIResourceChangeEvent(clusterRole.Namespace, clusterRole.Name, clusterRole, sharedutil.ResourceCreated, log)
	log.Info(fmt.Sprintf("clusterRole %q created in namespace %q", clusterRole.Name, clusterRole.Namespace))

	clusterRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ArgoCDManagerClusterRoleBindingName,
			Namespace: KubeSystemNamespace,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ArgoCDManagerClusterRoleName,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ArgoCDManagerServiceAccountName,
				Namespace: KubeSystemNamespace,
			},
		},
	}

	if err := k8sClient.Create(ctx, &clusterRoleBinding); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, &clusterRoleBinding); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(clusterRoleBinding.Namespace, clusterRoleBinding.Name, clusterRoleBinding, sharedutil.ResourceModified, log)

		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	sharedutil.LogAPIResourceChangeEvent(clusterRoleBinding.Namespace, clusterRoleBinding.Name, clusterRoleBinding, sharedutil.ResourceCreated, log)
	log.Info(fmt.Sprintf("clusterRoleBinding %q created in namespace %q", clusterRoleBinding.Name, clusterRoleBinding.Namespace))

	// Wait for Secret to contain a bearer token
	err := wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ArgoCDManagerSecretName,
				Namespace: KubeSystemNamespace,
				Annotations: map[string]string{
					"kubernetes.io/service-account.name": serviceAccount.Name,
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
				return false, nil
			}
		} else {
			validToken := false
			secretToken, exists := secret.Data["token"]
			if !exists || string(secretToken) == "" {
				log.Info("Token byte stream is still empty")
				validToken = false
			} else {
				validToken = true
			}
			return validToken, nil
		}
	})
	if err != nil {
		return err
	}

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

	clusterSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster-secret",
			Namespace: argoCDNamespace,
			Labels:    map[string]string{"argocd.argoproj.io/secret-type": "cluster"},
		},
		StringData: map[string]string{
			"name":   ClusterSecretName,
			"server": apiHost,
			"config": string(jsonString),
		},
		Type: corev1.SecretType("Opaque"),
	}

	// Create, or update cluster secret if it already exists
	if err := k8sClient.Create(context.Background(), clusterSecret); err != nil {
		if apierr.IsAlreadyExists(err) {
			if err := k8sClient.Update(ctx, clusterSecret); err != nil {
				return fmt.Errorf("error on Update %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(clusterSecret.Namespace, clusterSecret.Name, clusterSecret, sharedutil.ResourceCreated, log)

		} else {
			return fmt.Errorf("error on Create %v", err)
		}
	}
	sharedutil.LogAPIResourceChangeEvent(clusterSecret.Namespace, clusterSecret.Name, clusterSecret, sharedutil.ResourceCreated, log)

	log.Info(fmt.Sprintf("cluster secret %q created ", clusterSecret.Name))
	return nil
}
