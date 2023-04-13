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

	"github.com/go-logr/logr"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// EnvironmentReconciler reconciles a Environment object
type EnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=environments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=environments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=environments/finalizers,verbs=update
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = sharedutil.AddKCPClusterToContext(ctx, req.ClusterName)
	log := log.FromContext(ctx).WithValues("request", req)

	rClient := sharedutil.IfEnabledSimulateUnreliableClient(r.Client)

	// If the Namespace is in the process of being deleted, don't handle any additional requests.
	if isNamespaceBeingDeleted, err := isRequestNamespaceBeingDeleted(ctx, req.Namespace,
		rClient, log); isNamespaceBeingDeleted || err != nil {
		return ctrl.Result{}, err
	}

	// The goal of this function is to ensure that if an Environment exists, and that Environment
	// has the 'kubernetesCredentials' field defined, that a corresponding
	// GitOpsDeploymentManagedEnvironment exists (and is up-to-date).
	environment := &appstudioshared.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}
	if err := rClient.Get(ctx, client.ObjectKeyFromObject(environment), environment); err != nil {

		if apierr.IsNotFound(err) {
			log.Info("Environment resource no longer exists")
			// A) The Environment resource could not be found: the owner reference on the GitOpsDeploymentManagedEnvironment
			// should ensure that it is cleaned up, so no more work is required.
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("unable to retrieve Environment: %v", err)
	}

	if environment.GetDeploymentTargetClaimName() != "" && environment.Spec.UnstableConfigurationFields != nil {
		log.Error(nil, "Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set")

		// Update Status.Conditions field of Environment.
		if err := updateStatusConditionOfEnvironment(ctx, rClient,
			"Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set", environment,
			EnvironmentConditionErrorOccurred, metav1.ConditionTrue, EnvironmentReasonErrorOccurred); err != nil {

			log.Error(err, "unable to update environment status condition.")
			return ctrl.Result{}, fmt.Errorf("unable to update environment status condition. %v", err)

		}
		return ctrl.Result{}, nil
	}

	// Update Status.Conditions field of Environment as false if error is resolved
	if err := updateConditionErrorAsResolved(ctx, rClient, "", environment, EnvironmentConditionErrorOccurred, metav1.ConditionFalse, EnvironmentReasonErrorOccurred); err != nil {
		return ctrl.Result{}, err
	}

	desiredManagedEnv, err := generateDesiredResource(ctx, *environment, rClient, log)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to generate expected GitOpsDeploymentManagedEnvironment resource: %v", err)
	}
	if desiredManagedEnv == nil {
		return ctrl.Result{}, nil
	}

	currentManagedEnv := generateEmptyManagedEnvironment(environment.Name, environment.Namespace)
	if err := rClient.Get(ctx, client.ObjectKeyFromObject(&currentManagedEnv), &currentManagedEnv); err != nil {

		if apierr.IsNotFound(err) {
			// B) The GitOpsDeploymentManagedEnvironment doesn't exist, so needs to be created.

			log.Info("Creating GitOpsDeploymentManagedEnvironment", "managedEnv", desiredManagedEnv.Name)
			if err := rClient.Create(ctx, desiredManagedEnv); err != nil {
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

	if err := rClient.Update(ctx, &currentManagedEnv); err != nil {
		return ctrl.Result{},
			fmt.Errorf("unable to update existing GitOpsDeploymentManagedEnvironment '%s': %v", currentManagedEnv.Name, err)
	}
	sharedutil.LogAPIResourceChangeEvent(currentManagedEnv.Namespace, currentManagedEnv.Name, currentManagedEnv, sharedutil.ResourceModified, log)

	return ctrl.Result{}, nil
}

const (
	SnapshotEnvironmentBindingConditionErrorOccurred = "ErrorOccurred"
	SnapshotEnvironmentBindingReasonErrorOccurred    = "ErrorOccurred"
	EnvironmentConditionErrorOccurred                = "ErrorOccurred"
	EnvironmentReasonErrorOccurred                   = "ErrorOccurred"
)

// Update Status.Condition field of Environment
func updateStatusConditionOfEnvironment(ctx context.Context, client client.Client, message string,
	environment *appstudioshared.Environment, conditionType string,
	status metav1.ConditionStatus, reason string) error {
	// Check if condition with same type is already set, if Yes then check if content is same,
	// If content is not same update LastTransitionTime
	index := -1
	for i, Condition := range environment.Status.Conditions {
		if Condition.Type == conditionType {
			index = i
			break
		}
	}

	now := metav1.Now()

	if index == -1 {
		environment.Status.Conditions = append(environment.Status.Conditions,
			metav1.Condition{
				Type:               conditionType,
				Message:            message,
				LastTransitionTime: now,
				Status:             status,
				Reason:             reason,
			})
	} else if environment.Status.Conditions[index].Message != message ||
		environment.Status.Conditions[index].Reason != reason ||
		environment.Status.Conditions[index].Status != status {
		environment.Status.Conditions[index].Reason = reason
		environment.Status.Conditions[index].Message = message
		environment.Status.Conditions[index].Status = status
		environment.Status.Conditions[index].LastTransitionTime = now
	}

	if err := client.Status().Update(ctx, environment); err != nil {
		return err
	}

	return nil
}

// Update status condition when error is resolved
func updateConditionErrorAsResolved(ctx context.Context, client client.Client, message string,
	environment *appstudioshared.Environment, conditionType string,
	status metav1.ConditionStatus, reason string) error {

	if HasCondition(&environment.Status.Conditions, EnvironmentConditionErrorOccurred) {
		reason = reason + "Resolved"
		// Check the condition and mark it as resolved, if it's resolved
		if cond, _ := FindCondition(&environment.Status.Conditions, EnvironmentConditionErrorOccurred); cond.Reason != reason {
			if err := updateStatusConditionOfEnvironment(ctx, client,
				"", environment, EnvironmentConditionErrorOccurred, metav1.ConditionFalse, reason); err != nil {

				return err
			}
		}
	}

	return nil
}

// HasCondition checks for the existence of a given Condition type
func HasCondition(conditions *[]metav1.Condition, conditionType string) bool {
	for _, condition := range *conditions {
		if condition.Type == conditionType {
			return true
		}
	}
	return false
}

// FindCondition finds the suitable Condition object by looking into the conditions list and returns true if already exists
// but, if none exists, it appends one and returns false
func FindCondition(conditions *[]metav1.Condition, conditionType string) (*metav1.Condition, bool) {
	for i, condition := range *conditions {
		if condition.Type == conditionType {
			return &(*conditions)[i], true
		}
	}

	// No such condition exists, so append it
	*conditions = append(*conditions, metav1.Condition{Type: conditionType})

	return &(*conditions)[len(*conditions)-1], false
}

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

func generateDesiredResource(ctx context.Context, env appstudioshared.Environment, k8sClient client.Client, log logr.Logger) (*managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, error) {
	var manageEnvDetails managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec
	// If the Environment has a reference to the DeploymentTargetClaim, use the credential secret
	// from the bounded DeploymentTarget.
	claimName := env.GetDeploymentTargetClaimName()
	if claimName != "" {
		log.Info("Environment is configured with a DeploymentTargetClaim")
		dtc := &appstudioshared.DeploymentTargetClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: env.Namespace,
			},
		}

		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dtc), dtc); err != nil {
			if apierr.IsNotFound(err) {
				log.Error(err, "DeploymentTargetClaim not found while generating the desired Environment resource")

				// Update Status.Conditions field of Environment.
				if cerr := updateStatusConditionOfEnvironment(ctx, k8sClient,
					"DeploymentTargetClaim not found while generating the desired Environment resource", &env,
					EnvironmentConditionErrorOccurred, metav1.ConditionTrue, EnvironmentReasonErrorOccurred); cerr != nil {

					log.Error(cerr, "unable to update environment status condition.")
					return nil, fmt.Errorf("unable to update environment status condition. %v", err)
				}

				return nil, nil
			}

			// Update Status.Conditions field of Environment.
			if err := updateStatusConditionOfEnvironment(ctx, k8sClient,
				"Unable to find DeploymentTarget for DeploymentTargetClaim", &env,
				EnvironmentConditionErrorOccurred, metav1.ConditionTrue, EnvironmentReasonErrorOccurred); err != nil {

				log.Error(err, "unable to update environment status condition.")
				return nil, fmt.Errorf("unable to update environment status condition. %v", err)
			}

			return nil, err
		}

		// Update Status.Conditions field of Environment as false if error is resolved
		if err := updateConditionErrorAsResolved(ctx, k8sClient, "", &env, EnvironmentConditionErrorOccurred, metav1.ConditionFalse, EnvironmentReasonErrorOccurred); err != nil {
			return nil, err
		}

		// If the DeploymentTargetClaim is not in bounded phase, return and wait
		// until it reaches bounded phase.
		if dtc.Status.Phase != appstudioshared.DeploymentTargetClaimPhase_Bound {
			log.Info("Waiting until the DeploymentTargetClaim associated with Environment reaches Bounded phase", "DeploymentTargetClaim", dtc.Name)
			return nil, nil
		}

		// If the DeploymentTargetClaim is bounded, find the corresponding DeploymentTarget.
		dt, err := getDTBoundByDTC(ctx, k8sClient, dtc)
		if err != nil {
			if apierr.IsNotFound(err) {
				log.Error(err, "DeploymentTarget not found for DeploymentTargetClaim", "DeploymentTargetClaim", dtc.Name)

				// Update Status.Conditions field of Environment.
				if err := updateStatusConditionOfEnvironment(ctx, k8sClient,
					"DeploymentTarget not found for DeploymentTargetClaim", &env,
					EnvironmentConditionErrorOccurred, metav1.ConditionTrue, EnvironmentReasonErrorOccurred); err != nil {

					log.Error(err, "unable to update environment status condition.")
					return nil, fmt.Errorf("unable to update environment status condition. %v", err)
				}

				return nil, nil
			}

			// Update Status.Conditions field of Environment.
			if err := updateStatusConditionOfEnvironment(ctx, k8sClient,
				"Unable to find the DeploymentTarget for DeploymentTargetClaim", &env,
				EnvironmentConditionErrorOccurred, metav1.ConditionTrue, EnvironmentReasonErrorOccurred); err != nil {

				log.Error(err, "unable to update environment status condition.")
				return nil, fmt.Errorf("unable to update environment status condition. %v", err)
			}

			return nil, err
		}

		if dt == nil {
			log.Error(nil, "DeploymentTarget not found for DeploymentTargetClaim", "DeploymentTargetClaim", dtc.Name)

			// Update Status.Conditions field of Environment.
			if err := updateStatusConditionOfEnvironment(ctx, k8sClient,
				"DeploymentTarget not found for DeploymentTargetClaim", &env,
				EnvironmentConditionErrorOccurred, metav1.ConditionTrue, EnvironmentReasonErrorOccurred); err != nil {

				log.Error(err, "unable to update environment status condition.")
				return nil, fmt.Errorf("unable to update environment status condition. %v", err)
			}

			return nil, nil
		}

		// Update Status.Conditions field of Environment as false if error is resolved
		if err = updateConditionErrorAsResolved(ctx, k8sClient, "", &env, EnvironmentConditionErrorOccurred, metav1.ConditionFalse, EnvironmentReasonErrorOccurred); err != nil {
			return nil, err
		}
		log.Info("Using the cluster credentials from the DeploymentTarget", "DeploymentTarget", dt.Name)
		manageEnvDetails = managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			APIURL:                     dt.Spec.KubernetesClusterCredentials.APIURL,
			ClusterCredentialsSecret:   dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret,
			AllowInsecureSkipTLSVerify: dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify,
		}

	} else if env.Spec.UnstableConfigurationFields != nil {
		log.Info("Using the cluster credentials specified in the Environment")
		manageEnvDetails = managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			APIURL:                     env.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.APIURL,
			ClusterCredentialsSecret:   env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret,
			AllowInsecureSkipTLSVerify: env.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify,
		}
	} else {
		// Don't process the Environment configuration fields if they are empty
		log.Info("Environment neither has cluster credentials nor DeploymentTargetClaim configured")
		return nil, nil
	}

	// 1) Retrieve the secret that the Environment is pointing to
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      manageEnvDetails.ClusterCredentialsSecret,
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
	managedEnv.Spec = manageEnvDetails

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
		Watches(
			&source.Kind{Type: &appstudioshared.DeploymentTargetClaim{}},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForDeploymentTargetClaim),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&source.Kind{Type: &appstudioshared.DeploymentTarget{}},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForDeploymentTarget),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}

// findObjectsForDeploymentTargetClaim maps an incoming DTC event to the corresponding Environment request.
func (r *EnvironmentReconciler) findObjectsForDeploymentTargetClaim(dtc client.Object) []reconcile.Request {
	ctx := context.Background()
	handlerLog := log.FromContext(ctx)

	dtc, ok := dtc.(*appstudioshared.DeploymentTargetClaim)
	if !ok {
		handlerLog.Error(nil, "incompatible object in the Environment mapping function, expected a DeploymentTargetClaim")
		return []reconcile.Request{}
	}

	envList := &appstudioshared.EnvironmentList{}
	err := r.Client.List(context.Background(), envList, &client.ListOptions{Namespace: dtc.GetNamespace()})
	if err != nil {
		handlerLog.Error(err, "failed to list Environments in the Environment mapping function")
		return []reconcile.Request{}
	}

	envRequests := []reconcile.Request{}
	for i := 0; i < len(envList.Items); i++ {
		env := envList.Items[i]
		if env.GetDeploymentTargetClaimName() == dtc.GetName() {
			envRequests = append(envRequests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&env),
			})
		}
	}

	return envRequests
}

// findObjectsForDeploymentTarget maps an incoming DT event to the corresponding Environment request.
// We should reconcile Environments if the DT credentials get updated.
func (r *EnvironmentReconciler) findObjectsForDeploymentTarget(dt client.Object) []reconcile.Request {
	ctx := context.Background()
	handlerLog := log.FromContext(ctx)

	dtObj, ok := dt.(*appstudioshared.DeploymentTarget)
	if !ok {
		handlerLog.Error(nil, "incompatible object in the Environment mapping function, expected a DeploymentTarget")
		return []reconcile.Request{}
	}

	// 1. Find all DeploymentTargetClaims that are associated with this DeploymentTarget.
	dtcList := appstudioshared.DeploymentTargetClaimList{}
	err := r.List(ctx, &dtcList, &client.ListOptions{Namespace: dt.GetNamespace()})
	if err != nil {
		handlerLog.Error(err, "failed to list DeploymentTargetClaims in the mapping function")
		return []reconcile.Request{}
	}

	dtcs := []appstudioshared.DeploymentTargetClaim{}
	for _, d := range dtcList.Items {
		dtc := d
		// We only want to reconcile for DTs that have a corresponding DTC.
		if dtc.Spec.TargetName == dt.GetName() || dtObj.Spec.ClaimRef == dtc.Name {
			dtcs = append(dtcs, dtc)
		}
	}

	// 2. Find all Environments that are associated with this DeploymentTargetClaim.
	envList := &appstudioshared.EnvironmentList{}
	err = r.Client.List(context.Background(), envList, &client.ListOptions{Namespace: dt.GetNamespace()})
	if err != nil {
		handlerLog.Error(err, "failed to list Environments in the Environment mapping function")
		return []reconcile.Request{}
	}

	envRequests := []reconcile.Request{}
	for i := 0; i < len(envList.Items); i++ {
		env := envList.Items[i]
		for _, dtc := range dtcs {
			if env.GetDeploymentTargetClaimName() == dtc.GetName() {
				envRequests = append(envRequests, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(&env),
				})
			}
		}
	}

	return envRequests
}
