/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package appstudioredhatcom

import (
	"context"
	"fmt"
	"reflect"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EnvironmentReconciler reconciles a Environment object
type EnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=environments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=environments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=environments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = sharedutil.AddKCPClusterToContext(ctx, req.ClusterName)
	log := log.FromContext(ctx).WithValues("request", req)

	// The goal of this function is to ensure that if an Environment exists, and that Environment
	// has the 'kubernetesCredentials' field defined, that a corresponding
	// GitOpsDeploymentManagedEnvironment exists (and is up-to-date).
	environment := &appstudioshared.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(environment), environment); err != nil {

		if apierr.IsNotFound(err) {
			log.Info("Environment resource no longer exists")
			// A) The Environment resource could not be found: the owner reference on the GitOpsDeploymentManagedEnvironment
			// should ensure that it is cleaned up, so no more work is required.
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("unable to retrieve Environment: %v", err)
	}

	desiredManagedEnv, err := generateDesiredResource(ctx, *environment, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to generate expected GitOpsDeploymentManagedEnvironment resource: %v", err)
	}
	if desiredManagedEnv == nil {
		return ctrl.Result{}, nil
	}

	currentManagedEnv := generateEmptyManagedEnvironment(environment.Name, environment.Namespace)
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&currentManagedEnv), &currentManagedEnv); err != nil {

		if apierr.IsNotFound(err) {
			// B) The GitOpsDeploymentManagedEnvironment doesn't exist, so needs to be created.

			log.Info("Creating GitOpsDeploymentManagedEnvironment", "managedEnv", desiredManagedEnv.Name)
			if err := r.Client.Create(ctx, desiredManagedEnv); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to create new GitOpsDeploymentManagedEnvironment: %v", err)
			}
			sharedutil.LogAPIResourceChangeEvent(desiredManagedEnv.Namespace, desiredManagedEnv.Name, desiredManagedEnv, sharedutil.ResourceCreated, log)

			// Success: the resource has been created.
			return ctrl.Result{}, nil

		} else {
			// For any other error, return it
			return ctrl.Result{}, fmt.Errorf("unable to retrieve existing GitOpsDeploymentManagedEnvironment '%s': %v",
				currentManagedEnv.Name, err)
		}
	}

	// C) The GitOpsDeploymentManagedEnvironment already exists, so compare it with the desired state, and update it if different.
	if reflect.DeepEqual(currentManagedEnv.Spec, desiredManagedEnv.Spec) {
		// If the spec field is the same, no more work is needed.
		return ctrl.Result{}, nil
	}

	log.Info("Updating GitOpsDeploymentManagedEnvironment as a change was detected", "managedEnv", desiredManagedEnv.Name)

	// Update the current object to the desired state
	currentManagedEnv.Spec = desiredManagedEnv.Spec

	if err := r.Client.Update(ctx, &currentManagedEnv); err != nil {
		return ctrl.Result{},
			fmt.Errorf("unable to update existing GitOpsDeploymentManagedEnvironment '%s': %v", currentManagedEnv.Name, err)
	}
	sharedutil.LogAPIResourceChangeEvent(currentManagedEnv.Namespace, currentManagedEnv.Name, currentManagedEnv, sharedutil.ResourceModified, log)

	return ctrl.Result{}, nil
}

const (
	SnapshotEnvironmentBindingConditionErrorOccurred = "ErrorOccurred"
	SnapshotEnvironmentBindingReasonErrorOccurred    = "ErrorOccurred"
)

// Update Status.Condition field of snapshotEnvironmentBinding
func updateStatusConditionOfEnvironmentBinding(ctx context.Context, client client.Client, message string,
	binding *appstudioshared.SnapshotEnvironmentBinding, conditionType string,
	status metav1.ConditionStatus, reason string) error {
	// Check if condition with same type is already set, if Yes then check if content is same,
	// If content is not same update LastTransitionTime
	index := -1
	for i, Condition := range binding.Status.BindingConditions {
		if Condition.Type == conditionType {
			index = i
			break
		}
	}

	now := metav1.Now()

	if index == -1 {
		binding.Status.BindingConditions = append(binding.Status.BindingConditions,
			metav1.Condition{
				Type:               conditionType,
				Message:            message,
				LastTransitionTime: now,
				Status:             status,
				Reason:             reason,
			})
	} else {
		if binding.Status.BindingConditions[index].Message != message &&
			binding.Status.BindingConditions[index].Reason != reason &&
			binding.Status.BindingConditions[index].Status != status {
			binding.Status.BindingConditions[index].LastTransitionTime = now
		}
		binding.Status.BindingConditions[index].Reason = reason
		binding.Status.BindingConditions[index].Message = message
		binding.Status.BindingConditions[index].LastTransitionTime = now
		binding.Status.BindingConditions[index].Status = status

	}

	if err := client.Status().Update(ctx, binding); err != nil {
		return err
	}

	return nil
}

func generateDesiredResource(ctx context.Context, env appstudioshared.Environment, k8sClient client.Client) (*managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, error) {

	// Don't process the Environment configuration fields if they are empty
	if env.Spec.UnstableConfigurationFields == nil {
		return nil, nil
	}

	// 1) Retrieve the secret that the Environment is pointing to
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret,
			Namespace: env.Namespace,
		},
	}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		if apierr.IsNotFound(err) {
			return nil, fmt.Errorf("the secret '%s' referenced by the Environment resource was not found: %v", secret.Name, err)
		}
		return nil, err
	}

	// 2) Generate (but don't apply) the corresponding GitOpsDeploymentManagedEnvironment resource
	managedEnv := generateEmptyManagedEnvironment(env.Name, env.Namespace)
	managedEnv.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: managedgitopsv1alpha1.GroupVersion.Group + "/" + managedgitopsv1alpha1.GroupVersion.Version,
			Kind:       "Environment",
			Name:       env.Name,
			UID:        env.UID,
		},
	}
	managedEnv.Spec = managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
		APIURL:                   env.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.APIURL,
		ClusterCredentialsSecret: secret.Name,
	}

	return &managedEnv, nil
}

func generateEmptyManagedEnvironment(environmentName string, environmentNamespace string) managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment {
	res := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-environment-" + environmentName,
			Namespace: environmentNamespace,
		},
	}
	return res
}

// SetupWithManager sets up the controller with the Manager.
func (r *EnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appstudioshared.Environment{}).
		Complete(r)
}
