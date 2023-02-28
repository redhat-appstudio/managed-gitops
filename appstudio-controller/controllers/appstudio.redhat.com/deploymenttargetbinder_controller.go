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

	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
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

//+kubebuilder:rbac:groups=appstudio.redhat.com.redhat.com,resources=deploymenttargetclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com.redhat.com,resources=deploymenttargetclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com.redhat.com,resources=deploymenttargetclaims/finalizers,verbs=update

const (
	annBindCompleted string = "dt.appstudio.redhat.com/bind-complete"
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

	// If the binding is alredy done, we need to check if the DTC is still bound to a DT
	// and update the status accordingly
	if isBindingCompleted(dtc) {
		// Is targetName field optional?
		// verify if the DeploymentTarget specified by the DTC still exists.
		dt := applicationv1alpha1.DeploymentTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dtc.Spec.TargetName,
				Namespace: dtc.Namespace,
			},
		}

		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&dt), &dt); err != nil {
			return ctrl.Result{}, err
		}

		log.Info("DeploymentTarget exists for the DeploymentTargetClaim", "DeploymentTarget", dt.Name, "Namespace", dt.Namespace)

		err := updateDTCPhase(ctx, r.Client, &dtc, applicationv1alpha1.DeploymentTargetClaimPhase_Bound)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func updateDTCPhase(ctx context.Context, k8sClient client.Client, dtc *applicationv1alpha1.DeploymentTargetClaim, targetPhase applicationv1alpha1.DeploymentTargetClaimPhase) error {
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

func isBindingCompleted(dtc applicationv1alpha1.DeploymentTargetClaim) bool {
	if dtc.Annotations == nil {
		return false
	}

	_, found := dtc.Annotations[annBindCompleted]
	return found
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTargetClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r)
}
