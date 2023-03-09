/*
Copyright 2023.

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
	"time"

	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DeploymentTargetClaimReconciler reconciles a DeploymentTargetClaim object
type DeploymentTargetClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets/status,verbs=get;update;patch

const (
	annBindCompleted string = "dt.appstudio.redhat.com/bind-complete"

	annBoundByController string = "dt.appstudio.redhat.com/bound-by-controller"

	annTargetProvisioner string = "provisioner.appstudio.redhat.com/dt-provisioner"

	finalizerBinder string = "binder.appstudio.redhat.com/finalizer"

	annBinderValueYes string = "yes"

	// binderRequeueDuration indicates that the binder needs to reconcile after this duration.
	binderRequeueDuration time.Duration = 20 * time.Second
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DeploymentTargetClaim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *DeploymentTargetClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("request", req)

	dtc := applicationv1alpha1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc); err != nil {
		return ctrl.Result{}, err
	}

	// Add the deletion finalizer if it is absent.
	if addFinalizer(&dtc, finalizerBinder) {
		if err := r.Client.Update(ctx, &dtc); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer %s to DeploymentTargetClaim %s in namespace %s: %v", finalizerBinder, dtc.Name, dtc.Namespace, err)
		}
	}

	// Handle deletion if the DTC has a deletion timestamp set.
	if dtc.GetDeletionTimestamp() != nil {
		// If the DTC is bound set the status of the corresponding DT to Released
		if isDTCBound(dtc) {
			dt, err := getDTBoundByDTC(ctx, r.Client, &dtc)
			if err != nil && !apierr.IsNotFound(err) {
				return ctrl.Result{}, err
			}

			if dt != nil {
				err = updateDTStatusPhase(ctx, r.Client, dt, applicationv1alpha1.DeploymentTargetPhase_Released)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}

		if removeFinalizer(&dtc, finalizerBinder) {
			if err := r.Client.Update(ctx, &dtc); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer %s from DeploymentTargetClaim %s in namespace %s: %v", finalizerBinder, dtc.Name, dtc.Namespace, err)
			}
		}

		return ctrl.Result{}, nil
	}

	// If the binding is alredy done, we need to check if the DTC is still bound to a DT
	// and update the status accordingly
	if isBindingCompleted(dtc) {
		if err := handleBoundedDeploymentTargetClaim(ctx, r.Client, dtc); err != nil {
			log.Error(err, "failed to process bounded DeploymentTargetClaim")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// If the user doesn't set the DT, check if there is a matching DT available
	// or if it needs to be dynamically provisioned.
	if dtc.Spec.TargetName == "" {
		dt, err := findMatchingDTForDTC(ctx, r.Client, dtc)
		if err != nil {
			return ctrl.Result{}, err
		}

		// If a best match DT is available bind it to the current DTC.
		if dt != nil {
			err = bindDeploymentTargetCliamToTarget(ctx, r.Client, &dtc, dt, true)
			if err != nil {
				return ctrl.Result{}, err
			}

			if err := updateDTStatusPhase(ctx, r.Client, dt, applicationv1alpha1.DeploymentTargetPhase_Bound); err != nil {
				return ctrl.Result{}, err
			}

			if err := updateDTCStatusPhase(ctx, r.Client, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Bound); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		err = handleDynamicDTCProvisioning(ctx, r.Client, &dtc)
		if err != nil {
			log.Error(err, "failed to handle DeploymentTargetClaim for dynamic provsioning")
			return ctrl.Result{}, err
		}
		log.Info("Waiting for the DeploymentTarget to be dynamically created by the provisioner")

		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: binderRequeueDuration,
		}, nil
	}

	// Get the DT specified by the user in the DTC
	dt := applicationv1alpha1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dtc.Spec.TargetName,
			Namespace: dtc.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&dt), &dt); err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Waiting for DeploymentTarget to be created", "DeploymentTarget", dt.Name, "Namespace", dt.Namespace)

			// Update the DTC status as Pending and wait for DT to be created.
			if err := updateDTCStatusPhase(ctx, r.Client, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Pending); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: binderRequeueDuration,
			}, nil
		}
		return ctrl.Result{}, err

	}

	if dt.Spec.ClaimRef != "" {
		if dt.Spec.ClaimRef == dtc.Name {
			// Both DT and DTC refer each other. So bind them together.
			err := bindDeploymentTargetCliamToTarget(ctx, r.Client, &dtc, &dt, false)
			if err != nil {
				log.Error(err, "failed to bind DeploymentTargetClaim to the DeploymentTarget", "DeploymentTargetName", dt.Name, "Namespace", dt.Name)
				return ctrl.Result{}, err
			}
			log.Info("DeploymentTargetClaim bound to DeploymentTarget", "DeploymentTargetName", dt.Name, "Namespace", dt.Namespace)
		} else {
			// QUESTION: What should be the status here and should we return an error?
			alreadyClaimedErr := fmt.Errorf("DeploymentTargetClaim %s wants to claim DeploymentTarget %s that is already claimed in namespace %s", dtc.Name, dt.Name, dtc.Namespace)
			log.Error(alreadyClaimedErr, "invalid claim by DeploymentTargetClaim")

			// Update the DTC status to Pending since the DT is not available
			if err := updateDTCStatusPhase(ctx, r.Client, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Pending); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, alreadyClaimedErr
		}
	} else {
		// At this stage, DT isn't claimed by anyone. The current DTC can try to claim it.
		if err := doesDTMatchDTC(dt, dtc); err != nil {
			log.Error(err, "DeploymentTarget does not match the specified DeploymentTargetClaim")
			return ctrl.Result{}, err
		}

		err := bindDeploymentTargetCliamToTarget(ctx, r.Client, &dtc, &dt, false)
		if err != nil {
			log.Error(err, "failed to bind DeploymentTargetClaim to the DeploymentTarget", "DeploymentTargetName", dt.Name, "Namespace", dt.Name)
			return ctrl.Result{}, err
		}
		log.Info("DeploymentTargetClaim bound to DeploymentTarget", "DeploymentTargetName", dt.Name, "Namespace", dt.Namespace)
	}

	if err := updateDTStatusPhase(ctx, r.Client, &dt, applicationv1alpha1.DeploymentTargetPhase_Bound); err != nil {
		return ctrl.Result{}, err
	}

	// Update the status of DTC to bound.
	err := updateDTCStatusPhase(ctx, r.Client, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Bound)
	return ctrl.Result{}, err
}

// bindDeploymentTargetCliamToTarget binds the given DeploymentTarget to a DeploymentTargetClaim by
// setting the dtc.spec.targetName to the DT name and adding the "bound-by-controller" annotation to the DTC.
func bindDeploymentTargetCliamToTarget(ctx context.Context, k8sClient client.Client, dtc *applicationv1alpha1.DeploymentTargetClaim, dt *applicationv1alpha1.DeploymentTarget, isBoundByController bool) error {
	// Set the target name of DTC and update it as Bounded.
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dtc), dtc); err != nil {
			return nil
		}

		if dtc.Annotations == nil {
			dtc.Annotations = map[string]string{}
		}
		updated := false

		// Add bound-by-controller annotation if we are binding a DT that was
		// matched by the binding controller.
		if isBoundByController {
			dtc.Spec.TargetName = dt.Name
			val, found := dtc.Annotations[annBoundByController]
			if !found && val != annBinderValueYes {
				dtc.Annotations[annBoundByController] = annBinderValueYes
				updated = true
			}
		}

		// Add bind complete annotation
		val, found := dtc.Annotations[annBindCompleted]
		if !found && val != annBinderValueYes {
			dtc.Annotations[annBindCompleted] = annBinderValueYes
			updated = true
		}
		if updated {
			return k8sClient.Update(ctx, dtc)
		}
		return nil
	})
}

// handleBoundedDeploymentTargetClaim handles the DTCs that are already bounded i.e have the "bind-complete" annotation.
// It checks if the DTC is still bound to DTC and updates the status accordingly.
func handleBoundedDeploymentTargetClaim(ctx context.Context, k8sClient client.Client, dtc applicationv1alpha1.DeploymentTargetClaim) error {
	if !isBindingCompleted(dtc) {
		return nil
	}

	dt, err := getDTBoundByDTC(ctx, k8sClient, &dtc)
	if err != nil && !apierr.IsNotFound(err) {
		return fmt.Errorf("failed to get a DeploymentTarget for the given DeploymentTargetClaim %s", dtc.Name)
	}

	if dt == nil {
		// DeploymentTarget is not found for the DeploymentTargetClaim, so update the status as Lost
		err := updateDTCStatusPhase(ctx, k8sClient, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Lost)
		if err != nil {
			return err
		}

		return fmt.Errorf("DeploymentTarget not found for bounded DeploymentTargetClaim %s in namespace %s", dtc.Name, dtc.Namespace)
	}

	// At this stage, the DeploymentTarget exists, so update the status to Bound.
	if err := updateDTStatusPhase(ctx, k8sClient, dt, applicationv1alpha1.DeploymentTargetPhase_Bound); err != nil {
		return err
	}

	return updateDTCStatusPhase(ctx, k8sClient, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Bound)

}

// handleDynamicDTCProvisioning processes the DeploymentTargetClaim for dynamic provisioning.
func handleDynamicDTCProvisioning(ctx context.Context, k8sClient client.Client, dtc *applicationv1alpha1.DeploymentTargetClaim) error {
	// If DTC is not configured with a class name, update its status as Pending and return.
	if dtc.Spec.DeploymentTargetClassName == "" {
		return updateDTCStatusPhase(ctx, k8sClient, dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Pending)
	}

	// DTC is configured with a class name. So mark the DTC for dynamic provisioning.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dtc), dtc); err != nil {
			return nil
		}

		if dtc.Annotations == nil {
			dtc.Annotations = map[string]string{}
		}

		dtc.Annotations[annTargetProvisioner] = string(dtc.Spec.DeploymentTargetClassName)
		return k8sClient.Update(ctx, dtc)
	})
	if err != nil {
		return err
	}

	// set the DTC to Pending phase and wait for the Provisioner to create a DT
	return updateDTCStatusPhase(ctx, k8sClient, dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Pending)
}

// findMatchingDTForDTC tries to find a DT that matches the given DTC in a namespace.
func findMatchingDTForDTC(ctx context.Context, k8sClient client.Client, dtc applicationv1alpha1.DeploymentTargetClaim) (*applicationv1alpha1.DeploymentTarget, error) {
	dtList := applicationv1alpha1.DeploymentTargetList{}
	if err := k8sClient.List(ctx, &dtList, &client.ListOptions{Namespace: dtc.Namespace}); err != nil {
		return nil, err
	}

	var dt *applicationv1alpha1.DeploymentTarget
	for i, d := range dtList.Items {
		if doesDTMatchDTC(d, dtc) == nil {
			dt = &dtList.Items[i]
			break
		}
	}

	return dt, nil
}

// A DT matches a given DTC if it satisfies the below conditions
// 1. Both DT and DTC belong to the same class.
// 2. DT should be in Available phase and should not have a different claim ref.
// 3. DT should have the cluster credentials.
func doesDTMatchDTC(dt applicationv1alpha1.DeploymentTarget, dtc applicationv1alpha1.DeploymentTargetClaim) error {
	mismatchErr := fmt.Errorf("DeploymentTarget %s does not match DeploymentTargetClaim %s in namespace %s", dt.Name, dtc.Name, dtc.Namespace)
	if dt.Spec.DeploymentTargetClassName != dtc.Spec.DeploymentTargetClassName {
		return fmt.Errorf("%v: deploymentTargetClassName does not match", mismatchErr)
	}

	if dt.Status.Phase != applicationv1alpha1.DeploymentTargetPhase_Available {
		return fmt.Errorf("%v: DeploymentTarget is not in Available phase", mismatchErr)
	}

	if dt.Spec.KubernetesClusterCredentials == (applicationv1alpha1.DeploymentTargetKubernetesClusterCredentials{}) {
		return fmt.Errorf("%v: DeploymentTarget does not have cluster credentials", mismatchErr)
	}

	if err := checkForBindingConflict(&dtc, &dt); err != nil {
		return err
	}

	return nil
}

func updateDTCStatusPhase(ctx context.Context, k8sClient client.Client, dtc *applicationv1alpha1.DeploymentTargetClaim, targetPhase applicationv1alpha1.DeploymentTargetClaimPhase) error {
	if dtc.Status.Phase == targetPhase {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dtc), dtc); err != nil {
			return err
		}

		if dtc.Status.Phase == targetPhase {
			return nil
		}

		dtc.Status.Phase = targetPhase

		return k8sClient.Status().Update(ctx, dtc)
	})
}

func updateDTStatusPhase(ctx context.Context, k8sClient client.Client, dt *applicationv1alpha1.DeploymentTarget, targetPhase applicationv1alpha1.DeploymentTargetPhase) error {
	if dt.Status.Phase == targetPhase {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dt), dt); err != nil {
			return err
		}

		if dt.Status.Phase == targetPhase {
			return nil
		}

		dt.Status.Phase = targetPhase

		return k8sClient.Status().Update(ctx, dt)
	})
}

func isBindingCompleted(dtc applicationv1alpha1.DeploymentTargetClaim) bool {
	if dtc.Annotations == nil {
		return false
	}

	_, found := dtc.Annotations[annBindCompleted]
	return found
}

func isDTCBound(dtc applicationv1alpha1.DeploymentTargetClaim) bool {
	return isBindingCompleted(dtc) && dtc.Status.Phase == applicationv1alpha1.DeploymentTargetClaimPhase_Bound
}

func addFinalizer(obj client.Object, finalizer string) bool {
	finalizers := obj.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return false
		}
	}

	finalizers = append(finalizers, finalizer)
	obj.SetFinalizers(finalizers)
	return true
}

func removeFinalizer(obj client.Object, finalizer string) bool {
	finalizers := obj.GetFinalizers()
	for i, f := range finalizers {
		if f == finalizer {
			finalizers = append(finalizers[:i], finalizers[i+1:]...)
			obj.SetFinalizers(finalizers)
			return true
		}
	}
	return false
}

// getDTBoundByDTC will get the DT that is bound to a given DTC.
// It returns the DT targetted by DTC if DTC.Spec.TargetName is set.
// Else it will fetch the DT that is claiming the current DTC.
func getDTBoundByDTC(ctx context.Context, k8sClient client.Client, dtc *applicationv1alpha1.DeploymentTargetClaim) (*applicationv1alpha1.DeploymentTarget, error) {
	if dtc.Spec.TargetName != "" {
		dt := &applicationv1alpha1.DeploymentTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dtc.Spec.TargetName,
				Namespace: dtc.Namespace,
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dt), dt); err != nil {
			return nil, err
		}

		if err := checkForBindingConflict(dtc, dt); err != nil {
			return nil, err
		}

		return dt, nil
	}

	dtList := applicationv1alpha1.DeploymentTargetList{}
	if err := k8sClient.List(ctx, &dtList, &client.ListOptions{Namespace: dtc.Namespace}); err != nil {
		return nil, err
	}

	for i, dt := range dtList.Items {
		if dt.Spec.ClaimRef == dtc.Name {
			return &dtList.Items[i], nil
		}
	}

	return nil, nil
}

func checkForBindingConflict(dtc *applicationv1alpha1.DeploymentTargetClaim, dt *applicationv1alpha1.DeploymentTarget) error {
	if dtc.Spec.TargetName == dt.Name {
		if dt.Spec.ClaimRef != "" && dt.Spec.ClaimRef != dtc.Name {
			return fmt.Errorf("DeploymentTarget %s has a claim ref to a DeploymentTargetClaim %s with a different target", dt.Name, dtc.Name)
		}
	}

	if dt.Spec.ClaimRef == dtc.Name {
		if dtc.Spec.TargetName != "" && dtc.Spec.TargetName != dt.Name {
			return fmt.Errorf("DeploymentTargetClaim %s targets a DeploymenetTarget %s with a different claim ref", dtc.Name, dt.Name)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTargetClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r)
}
