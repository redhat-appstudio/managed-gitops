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
	"os"
	"strings"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	log := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops).WithValues(
		logutil.Log_Component, logutil.Log_Component_Appstudio_Controller,
		logutil.Log_K8s_Request_Namespace, req.Namespace,
		logutil.Log_K8s_Request_Name, req.Name)

	spacerequest := codereadytoolchainv1alpha1.SpaceRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&spacerequest), &spacerequest); err != nil {
		// Don't requeue if the requested object is not found/deleted.
		if apierr.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if spacerequest.DeletionTimestamp != nil {
		// Skip processing SpaceRequests that are already in the process of being deleted.
		return ctrl.Result{}, nil
	}

	if dtcLabel, exists := spacerequest.Labels[DeploymentTargetClaimLabel]; !exists {
		log.Info("SpaceRequest is missing an '" + DeploymentTargetClaimLabel + "' label, so skipping.")
		return ctrl.Result{}, nil

	} else {
		// Determine if the SpaceRequest is orphaned: that is, the SpaceRequest refers to a DTC that no longer exists.

		spaceRequestDeleted, err := r.deleteSpaceRequestIfOrphaned(ctx, spacerequest, dtcLabel, log)
		if err != nil {
			return ctrl.Result{}, err
		}

		if spaceRequestDeleted { // No more work to do
			return ctrl.Result{}, nil
		}
	}

	if len(spacerequest.Status.NamespaceAccess) > 1 {
		// TODO: GITOPSRVCE-578: once we support more than one namespace, remove this check.
		log.Error(nil, "Sandbox provisioner does not currently support SpaceRequests with more than 1 namespace")
		return ctrl.Result{}, nil
	}

	if !doesSpaceRequestHaveReadyTrue(spacerequest) {
		log.Info("SpaceRequest is not ready yet")
		return ctrl.Result{}, nil
	}

	if len(spacerequest.Status.NamespaceAccess) == 0 {
		// No Namespaces yet
		log.Info("SpaceRequest is ready, but does not have a NamespaceAccess")
		return ctrl.Result{}, nil
	}

	var dt *applicationv1alpha1.DeploymentTarget
	var err error

	if dt, err = findMatchingDTForSpaceRequest(ctx, r.Client, spacerequest); err != nil {
		log.Error(err, "error while finding a dt that matches the SpaceRequest")
		return ctrl.Result{}, err
	}

	if dt == nil {
		dt, err = createDeploymentTargetForSpaceRequest(ctx, r.Client, spacerequest, log)
		if err != nil {
			log.Error(err, "failed to create the DeploymentTarget for a SpaceRequest")
			return ctrl.Result{}, err
		}

		log.Info("DeploymentTarget has been created for SpaceRequest", "SpaceRequest.Name", spacerequest.Name, "SpaceRequest.Namespace", spacerequest.Namespace)
		logutil.LogAPIResourceChangeEvent(dt.Namespace, dt.Name, dt, logutil.ResourceCreated, log)

	} else {
		log.Info("A DeploymentTarget for the SpaceRequest already exists, no work needed.", "DeploymentTarget.Name", dt.Name, "Namespace", dt.Namespace)
	}

	// If we found or created the DT, ensure the SpaceRequest references it via DeploymentTargetLabel
	if dt != nil {

		if val, exists := spacerequest.Labels[DeploymentTargetLabel]; !exists || val != dt.Name {

			spacerequest.Labels[DeploymentTargetLabel] = dt.Name

			if err := r.Client.Update(ctx, &spacerequest); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// deleteSpaceRequestIfOrphaned will delete a SpaceRequest if:
// - The dev sandbox DeploymentTargetClass exist, and it has Delete Policy
// - The DTC referenced by the SpaceRequest label does not exit
//
// Return true if the SpaceRequest was deleted, false otherwise
func (r *DevsandboxDeploymentReconciler) deleteSpaceRequestIfOrphaned(ctx context.Context, spacerequest codereadytoolchainv1alpha1.SpaceRequest, dtcLabelFromSpaceRequest string, log logr.Logger) (bool, error) {

	// Locate the Devsandbox DeploymentTargetClass
	var dtClassList applicationv1alpha1.DeploymentTargetClassList
	if err := r.Client.List(ctx, &dtClassList); err != nil {
		return false, err
	}

	var dtClassDevsandbox *applicationv1alpha1.DeploymentTargetClass

	for i, dtClass := range dtClassList.Items {

		if dtClass.Spec.Provisioner == applicationv1alpha1.Provisioner_Devsandbox {

			dtClassDevsandbox = &dtClassList.Items[i]
		}
	}

	if dtClassDevsandbox == nil {
		// Devsandbox provisioner could not be located, so we should not delete the SpaceRequest
		return false, nil
	}

	if dtClassDevsandbox.Spec.ReclaimPolicy != applicationv1alpha1.ReclaimPolicy_Delete {
		// The reclaim policy of Devsandbox provisioner is not delete, so we should not delete the SpaceRequest
		return false, nil
	}

	// Attempt to locate the DTC that references the SpaceRequest
	dtcReferencedByLabel := applicationv1alpha1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dtcLabelFromSpaceRequest,
			Namespace: spacerequest.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&dtcReferencedByLabel), &dtcReferencedByLabel); err != nil {
		if !apierr.IsNotFound(err) {
			return false, err
		}

		// If the DeploymentTargetClaim doesn't exist, delete the SpaceRequest
		if err := r.Client.Delete(ctx, &spacerequest); err != nil {
			log.Error(err, "unable to delete SpaceRequest that is no longer referenced by DTC")
			return false, err
		}

		log.Info("the DTC referenced by the SpaceRequest no longer exists, so deleted the SpaceRequest", "dtcLabel", dtcReferencedByLabel)

		logutil.LogAPIResourceChangeEvent(spacerequest.Namespace, spacerequest.Name, &spacerequest, logutil.ResourceDeleted, log)

		return true, nil

	}

	return false, nil
}

// doesSpaceRequestHaveReadyTrue checks if SpaceRequest condition is in Ready status.
func doesSpaceRequestHaveReadyTrue(spacerequest codereadytoolchainv1alpha1.SpaceRequest) bool {
	return condition.IsTrue(spacerequest.Status.Conditions, codereadytoolchainv1alpha1.ConditionReady)
}

// findMatchingDTForSpaceRequest tries to find a DT that matches the given SpaceRequest in a namespace.
func findMatchingDTForSpaceRequest(ctx context.Context, k8sClient client.Client, spacerequest codereadytoolchainv1alpha1.SpaceRequest) (*applicationv1alpha1.DeploymentTarget, error) {

	if HasLabel(&spacerequest, DeploymentTargetLabel) {

		dt := &applicationv1alpha1.DeploymentTarget{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: spacerequest.Namespace,
				Name:      spacerequest.Labels[DeploymentTargetLabel],
			},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dt), dt); err != nil {

			if !apierr.IsNotFound(err) {
				return nil, fmt.Errorf("unable to locate DT with name '%s' in Namespace '%s', from DeploymentTargetLabel of SpaceRequest '%s': %w", dt.Name, dt.Namespace, spacerequest.Name, err)
			}

			// On not found, continue to the rest of the function to attempt to find it using the DTC
		} else {
			// On success, return the DT
			return dt, nil
		}

	}

	// If it doesn't have a label, instead attempt to find it via the DTC label

	dtc, err := findMatchingDTCForSpaceRequest(ctx, k8sClient, spacerequest)

	if err != nil {
		return nil, err
	}

	if dtc == nil {
		return nil, nil
	}

	// 2) The DTC was found, so find the DT that reference that DTC in its claimref

	dtList := applicationv1alpha1.DeploymentTargetList{}
	opts := []client.ListOption{
		client.InNamespace(spacerequest.Namespace),
	}

	if err := k8sClient.List(ctx, &dtList, opts...); err != nil {
		return nil, err
	}

	for i, dt := range dtList.Items {
		if dtc.Name == dt.Spec.ClaimRef {

			matchingDT := &dtList.Items[i]
			return matchingDT, nil
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
func findMatchingDTCForSpaceRequest(ctx context.Context, k8sClient client.Client, spacerequest codereadytoolchainv1alpha1.SpaceRequest) (*applicationv1alpha1.DeploymentTargetClaim, error) {
	if !HasLabel(&spacerequest, DeploymentTargetClaimLabel) {
		return nil, fmt.Errorf("no '%s' label is set for spacerequest '%s' '%s'", DeploymentTargetClaimLabel, spacerequest.Name, spacerequest.Namespace)
	}
	dtc := &applicationv1alpha1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: spacerequest.Namespace,
			Name:      spacerequest.Labels[DeploymentTargetClaimLabel],
		},
	}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(dtc), dtc); err != nil {
		return nil, fmt.Errorf("unable to locate DTC with name '%s' in Namespace '%s': %w", dtc.Name, dtc.Namespace, err)
	}
	return dtc, nil
}

// createDeploymentTargetForSpaceRequest creates and returns a new DeploymentTarget
// If it's not possible to create it and set the SpaceRequest as the owner, an error will be returned
func createDeploymentTargetForSpaceRequest(ctx context.Context, k8sClient client.Client, spacerequest codereadytoolchainv1alpha1.SpaceRequest, log logr.Logger) (*applicationv1alpha1.DeploymentTarget, error) {

	dtc, err := findMatchingDTCForSpaceRequest(ctx, k8sClient, spacerequest)
	if err != nil {
		return nil, fmt.Errorf("unable to locate matching DTC for SpaceRequest: %w", err)
	}

	deploymentTarget := &applicationv1alpha1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dtc.Name + "-dt",
			Namespace: dtc.Namespace,
		},
		Spec: applicationv1alpha1.DeploymentTargetSpec{
			DeploymentTargetClassName: dtc.Spec.DeploymentTargetClassName,
			KubernetesClusterCredentials: applicationv1alpha1.DeploymentTargetKubernetesClusterCredentials{
				DefaultNamespace:         spacerequest.Status.NamespaceAccess[0].Name,
				APIURL:                   spacerequest.Status.TargetClusterURL,
				ClusterCredentialsSecret: spacerequest.Status.NamespaceAccess[0].SecretRef,
			},
			ClaimRef: dtc.Name,
		},
	}

	// Set AllowInsecureSkipTLSVerify field of DT to True, if it is a dev cluster.
	if strings.EqualFold(os.Getenv("DEV_ONLY_IGNORE_SELFSIGNED_CERT_IN_DEPLOYMENT_TARGET"), "true") {
		deploymentTarget.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify = true
	}

	if HasAnnotation(dtc, applicationv1alpha1.AnnTargetProvisioner) {
		if deploymentTarget.Labels == nil {
			deploymentTarget.Labels = make(map[string]string)
		}
		deploymentTarget.Labels[applicationv1alpha1.AnnTargetProvisioner] = dtc.Annotations[applicationv1alpha1.AnnTargetProvisioner]
	}

	if deploymentTarget.Annotations == nil {
		deploymentTarget.Annotations = make(map[string]string)
	}
	deploymentTarget.Annotations[applicationv1alpha1.AnnDynamicallyProvisioned] = string(applicationv1alpha1.Provisioner_Devsandbox)

	// Create the DeploymentTarget
	if err := k8sClient.Create(ctx, deploymentTarget); err != nil {
		return nil, err
	}
	logutil.LogAPIResourceChangeEvent(deploymentTarget.Namespace, deploymentTarget.Name, deploymentTarget, logutil.ResourceCreated, log)

	deploymentTarget.Status.Phase = applicationv1alpha1.DeploymentTargetPhase_Available // set phrase to "Available"
	if err := k8sClient.Status().Update(ctx, deploymentTarget); err != nil {
		return deploymentTarget, fmt.Errorf("failed to update DeploymentTarget %s in namespace %s to Available status", deploymentTarget.Name, deploymentTarget.Namespace)
	}

	return deploymentTarget, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DevsandboxDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&codereadytoolchainv1alpha1.SpaceRequest{}).
		Complete(r)
}
