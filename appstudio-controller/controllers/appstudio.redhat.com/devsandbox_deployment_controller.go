/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
"github.com/codeready-toolchain/toolchain-common/pkg/condition"
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package appstudioredhatcom

import (
	"context"
	"fmt"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DevsandboxDeploymentReconciler reconciles a SpaceRequest object
type DevsandboxDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets,verbs=get;list;create;watch;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequests,verbs=get;list;watch;update;patch
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
func (r *DevsandboxDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dt *applicationv1alpha1.DeploymentTarget
	var err error

	log := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops)

	spacerequest := codereadytoolchainv1alpha1.SpaceRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	if err = r.Client.Get(ctx, client.ObjectKeyFromObject(&spacerequest), &spacerequest); err != nil {
		// Don't requeue if the requested object is not found/deleted.
		if apierr.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if len(spacerequest.Status.NamespaceAccess) > 1 {
		log.Error(nil, "Sandbox provisioner does not currently support SpaceRequests with more than 1 namespace")
		return ctrl.Result{}, nil
	}

	if !doesSpaceRequestHaveReadyTrue(&spacerequest) {
		return ctrl.Result{}, nil
	}
	if dt, err = findMatchingDTForSpaceRequest(ctx, r.Client, &spacerequest); err != nil {
		log.Error(err, "error while finding a dt that matches the SpaceRequest")
		return ctrl.Result{}, err
	}

	if dt == nil {
		dt, err = createDeploymentTargetForSpaceRequest(ctx, r.Client, &spacerequest)
		if err != nil {
			log.Error(err, "failed to create the DeploymentTarget for a SpaceRequest")
			return ctrl.Result{}, err
		}

		log.Info("DeploymentTarget has been created for SpaceRequest", "SpaceRequest.Name", spacerequest.Name, "SpaceRequest.Namespace", spacerequest.Namespace)
	}

	log.Info("A DeploymentTarget for the SpaceRequest exists", "DeploymentTarget.Name", dt.Name, "Namespace", dt.Namespace)

	return ctrl.Result{}, nil
}

// findMatchingDTForSpaceRequest tries to find a DT that matches the given SpaceRequest in a namespace.
func findMatchingDTForSpaceRequest(ctx context.Context, k8sClient client.Client, spacerequest *codereadytoolchainv1alpha1.SpaceRequest) (*applicationv1alpha1.DeploymentTarget, error) {
	if dtc, _ := findMatchingDTCForSpaceRequest(ctx, k8sClient, spacerequest); dtc != nil {
		dtList := applicationv1alpha1.DeploymentTargetList{}
		opts := []client.ListOption{
			client.InNamespace(spacerequest.Namespace),
		}

		if err := k8sClient.List(ctx, &dtList, opts...); err != nil {
			return nil, err
		}

		var dt *applicationv1alpha1.DeploymentTarget
		for i, d := range dtList.Items {
			if dtc.Name == string(d.Spec.ClaimRef) {
				dt = &dtList.Items[i]
				return dt, nil
			}
		}
	}
	return nil, nil
}

func HasAnnotation(object client.Object, annotation string) bool {
	_, found := object.GetAnnotations()[annotation]
	return found
}

func HasLabel(object client.Object, label string) bool {
	_, found := object.GetLabels()[label]
	return found
}

// findMatchingDTCForSpaceRequest tries to find the DTC that matches a given SpaceRequest.
func findMatchingDTCForSpaceRequest(ctx context.Context, k8sClient client.Client, spacerequest *codereadytoolchainv1alpha1.SpaceRequest) (*applicationv1alpha1.DeploymentTargetClaim, error) {
	if !HasLabel(spacerequest, deploymentTargetClaimLabel) {
		return nil, fmt.Errorf("no 'appstudio.openshift.io/dtc' label is set for spacerequest %s %s", spacerequest.Name, spacerequest.Namespace)
	}
	dtc := &applicationv1alpha1.DeploymentTargetClaim{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: spacerequest.Namespace,
		Name:      spacerequest.Labels[deploymentTargetClaimLabel],
	}, dtc)
	if err != nil {
		return nil, err
	}
	return dtc, nil
}

// newDeploymentTarget creates a new DeploymentTarget using the provided info.
func newDeploymentTarget(deploymentTargetClassName applicationv1alpha1.DeploymentTargetClassName, dtcNamespace string, namespace string, clusterAPIURL string, secretRef string, dtcName string) *applicationv1alpha1.DeploymentTarget {
	dtName := dtcName + "-dt"
	deploymentTarget := &applicationv1alpha1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dtName + "-",
			Namespace:    dtcNamespace,
		},
		Spec: applicationv1alpha1.DeploymentTargetSpec{
			DeploymentTargetClassName: deploymentTargetClassName,
			KubernetesClusterCredentials: applicationv1alpha1.DeploymentTargetKubernetesClusterCredentials{
				DefaultNamespace:         namespace,
				APIURL:                   clusterAPIURL,
				ClusterCredentialsSecret: secretRef,
			},
			ClaimRef: dtcName,
		},
	}

	return deploymentTarget
}

// createDeploymentTargetForSpaceRequest creates and returns a new DeploymentTarget
// If it's not possible to create it and set the SpaceRequest as the owner, an error will be returned
func createDeploymentTargetForSpaceRequest(ctx context.Context, client client.Client, spacerequest *codereadytoolchainv1alpha1.SpaceRequest) (*applicationv1alpha1.DeploymentTarget, error) {

	dtc, err := findMatchingDTCForSpaceRequest(ctx, client, spacerequest)
	if err != nil {
		return nil, err
	}

	deploymentTarget := newDeploymentTarget(
		dtc.Spec.DeploymentTargetClassName,
		dtc.Namespace,
		spacerequest.Status.NamespaceAccess[0].Name,
		spacerequest.Status.TargetClusterURL,
		spacerequest.Status.NamespaceAccess[0].SecretRef, dtc.Name)

	if HasAnnotation(dtc, applicationv1alpha1.AnnTargetProvisioner) {
		if deploymentTarget.Labels == nil {
			deploymentTarget.Labels = make(map[string]string)
		}
		deploymentTarget.Labels[applicationv1alpha1.AnnTargetProvisioner] = dtc.Annotations[applicationv1alpha1.AnnTargetProvisioner]
	}

	err = client.Create(ctx, deploymentTarget)
	if err != nil {
		return nil, err
	}

	deploymentTarget.Status.Phase = applicationv1alpha1.DeploymentTargetPhase_Available // set phrase to "Available"
	if err := client.Update(ctx, deploymentTarget); err != nil {
		return deploymentTarget, fmt.Errorf("failed to update DeploymentTarget %s in namespace %s to Available status", deploymentTarget.Name, deploymentTarget.Namespace)
	}

	return deploymentTarget, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DevsandboxDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&codereadytoolchainv1alpha1.SpaceRequest{}).
		WithEventFilter(predicate.Or(
			spaceRequestReadyPredicate())).
		Complete(r)
}
