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

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DeploymentTargetReconciler reconciles a DeploymentTarget object
type DeploymentTargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	FinalizerDT = "dt.appstudio.redhat.com/finalizer"
)

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets,verbs=get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets/finalizers,verbs=update
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DeploymentTargetClaim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *DeploymentTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error

	log := log.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace, "component", "deploymentTargetreclaimer")

	dt := applicationv1alpha1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)

	if err != nil {
		if apierr.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Add the deletion finalizer if it is absent.
	if addFinalizer(&dt, FinalizerDT) {
		if err = r.Client.Update(ctx, &dt); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer %s to DeploymentTarget %s in namespace %s: %v", FinalizerDT, dt.Name, dt.Namespace, err)
		}
		log.Info("Added finalizer to DeploymentTarget", "finalizer", FinalizerDT)
	}

	// Handle deletion if the DT has a deletion timestamp set.
	if dt.GetDeletionTimestamp() != nil {
		var sr *codereadytoolchainv1alpha1.SpaceRequest
		if sr, err = findMatchingSpaceRequestForDT(ctx, r.Client, &dt); err != nil {
			if !apierr.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		if sr == nil {
			if removeFinalizer(&dt, FinalizerDT) {
				if err := r.Client.Update(ctx, &dt); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to remove finalizer %s from DeploymentTarget %s in namespace %s: %v", FinalizerDT, dt.Name, dt.Namespace, err)
				}
				log.Info("Removed finalizer from DeploymentTarget", "finalizer", FinalizerDT)
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
		if sr != nil {
			if addFinalizer(sr, codereadytoolchainv1alpha1.FinalizerName) {
				if err := r.Client.Update(ctx, sr); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to add finalizer %s for SpaceRequest %s in namespace %s: %v", codereadytoolchainv1alpha1.FinalizerName, sr.Name, sr.Namespace, err)
				}
				log.Info("Added finalizer for SpaceRequest", "finalizer", codereadytoolchainv1alpha1.FinalizerName)
			}
			// delete the SpaceRequest if it has not been deleted
			if condition.IsTrue(sr.Status.Conditions, codereadytoolchainv1alpha1.ConditionReady) {
				err = r.Client.Delete(ctx, sr)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
			var readyCond codereadytoolchainv1alpha1.Condition
			var found bool
			if readyCond, found = condition.FindConditionByType(sr.Status.Conditions, codereadytoolchainv1alpha1.ConditionReady); !found {
				return ctrl.Result{}, fmt.Errorf("failed to find ConditionReady for SpaceRequest %s from %s", sr.Name, sr.Namespace)
			}

			//SpaceRequest still exist after 2 minutes and with condition reason is UnableToTerminate
			if sr.GetDeletionTimestamp() != nil && time.Now().Add(-time.Minute*2).After(sr.GetDeletionTimestamp().Time) &&
				readyCond.Reason == "UnableToTerminate" {
				dt.Status.Phase = applicationv1alpha1.DeploymentTargetPhase_Failed
				if err := r.Client.Update(ctx, &dt); err != nil {
					return ctrl.Result{}, err
				}
				log.Info("The status of DT %s from %s is updated to Failed", dt.Name, dt.Namespace)
				return ctrl.Result{}, nil
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// findMatchingSpaceRequestForDT tries to find a SpaceRequest that matches the given DT in a namespace.
// The function will return only the SpaceRequest that matches the expected environment Tier name
func findMatchingSpaceRequestForDT(ctx context.Context, k8sClient client.Client, dt *applicationv1alpha1.DeploymentTarget) (*codereadytoolchainv1alpha1.SpaceRequest, error) {

	spaceRequestList := codereadytoolchainv1alpha1.SpaceRequestList{}

	opts := []client.ListOption{
		client.InNamespace(dt.Namespace),
		client.MatchingLabels{
			deploymentTargetClaimLabel: dt.Spec.ClaimRef,
		},
	}

	if err := k8sClient.List(ctx, &spaceRequestList, opts...); err != nil {
		return nil, err
	}

	if len(spaceRequestList.Items) > 0 {
		var spaceRequest *codereadytoolchainv1alpha1.SpaceRequest
		for i, s := range spaceRequestList.Items {
			if s.Spec.TierName == environmentTierName {
				spaceRequest = &spaceRequestList.Items[i]
				break
			}
		}
		return spaceRequest, nil
	}

	return nil, nil
}

// findMatchingDTClassForDT tries to find a DTCLS that matches the given DT in a namespace.
func findMatchingDTClassForDT(ctx context.Context, k8sClient client.Client, dt applicationv1alpha1.DeploymentTarget) (*applicationv1alpha1.DeploymentTargetClass, error) {
	dtclsList := applicationv1alpha1.DeploymentTargetClassList{}
	if err := k8sClient.List(ctx, &dtclsList); err != nil {
		return nil, err
	}

	var dtcls *applicationv1alpha1.DeploymentTargetClass
	for i, d := range dtclsList.Items {
		if d.Name == string(dt.Spec.DeploymentTargetClassName) {
			dtcls = &dtclsList.Items[i]
			break
		}
	}

	return dtcls, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	manager := ctrl.NewControllerManagedBy(mgr).
		For(&applicationv1alpha1.DeploymentTarget{}).
		WithEventFilter(predicate.Or(
			DeploymentTargetDeletePredicate()))

	return manager.Complete(r)
}
