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
	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// environmentTierName is the tier that will be used for sandbox namespace-backed environments
	environmentTierName = "env"
	// deploymentTargetClaimLabel is the label indicating the DeploymentTargetClaim that's associated with the object
	deploymentTargetClaimLabel = "appstudio.openshift.io/dtc"
)

// SandboxProvisionerReconciler reconciles a DeploymentTargetClaim object in order to provision a Sandbox for it
type SandboxProvisionerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=deploymenttargetclass,verbs=get;list;watch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequest,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spacerequest/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DeploymentTargetClaim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *SandboxProvisionerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	dtc := applicationv1alpha1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc); err != nil {
		// Don't requeue if the requested object is not found/deleted.
		if apierr.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// If the DTC has a DeploymentTargetClass defined, check if the class is supposed to use the Sandbox provisioner.
	// If the DeploymentTargetClass is not defined, or it does not use the Sandbox provisioner, exit the Reconcile loop.
	if dtc.Spec.DeploymentTargetClassName != "" {
		dtcls, err := findMatchingDTClassForDTC(ctx, r.Client, dtc)
		if err != nil {
			log.Error(err, "failed when trying to find a DeploymentTargetClass that matches the DeploymentTargetClaim")
			return ctrl.Result{}, err
		}

		if dtcls == nil {
			log.Error(err, "failed to find a DeploymentTargetClass that matches the DeploymentTargetClaim")
			missingDTCLSErr := missingDTCLSErrWrap(dtc.Name, string(dtc.Spec.DeploymentTargetClassName))
			return ctrl.Result{}, missingDTCLSErr("the resource could not be found on the cluster")
		}

		if dtcls.Spec.Provisioner != applicationv1alpha1.Provisioner_Devsandbox {
			log.Info("the DeploymentTargetClass referenced by the DeploymentTargetClaim doesn't use the DevSandbox provisioner",
				"DTC.Name", dtc.Name, "DTCLS.Spec.Provisioner", dtcls.Spec.Provisioner)
			return ctrl.Result{}, nil
		}
	} else {
		log.Info("the DeploymentTargetClaim doesn't have a DeploymentTargetClass defined, can't determine which provisioner needs to be used")
		return ctrl.Result{}, nil
	}

	// Check if there is already a matching SpaceRequest for this DTC
	spaceRequest, err := findMatchingSpaceRequestForDTC(ctx, r.Client, dtc)
	if err != nil {
		log.Error(err, "error while finding a SpaceRequest that matches the DeploymentTargetClaim")
		return ctrl.Result{}, err
	}

	// If there is no existing SpaceRequest, create a new one
	if spaceRequest == nil {
		log.Info("No existing SpaceRequest for the DeploymentTargetClaim found, creating a new one")
		spaceRequest, err = createSpaceRequestForDTC(ctx, r.Client, dtc)
		if err != nil {
			log.Error(err, "failed to create a new SpaceRequest for the DeploymentTargetClaim")
			return ctrl.Result{}, err
		}
	}

	log.Info("A SpaceRequest for the DeploymentTargetClaim exists", "SpaceRequest.Name", spaceRequest.Name, "Namespace", spaceRequest.Namespace)

	return ctrl.Result{}, nil
}

func missingDTCLSErrWrap(dtcName, dtclsName string) func(string) error {
	return func(msg string) error {
		return fmt.Errorf("DeploymentTargetClaim %s does not have a matching DeploymentTargetClass %s: %s", dtcName, dtclsName, msg)
	}
}

// findMatchingDTForDTC tries to find a DT that matches the given DTC in a namespace.
func findMatchingDTClassForDTC(ctx context.Context, k8sClient client.Client, dtc applicationv1alpha1.DeploymentTargetClaim) (*applicationv1alpha1.DeploymentTargetClass, error) {
	dtList := applicationv1alpha1.DeploymentTargetClassList{}
	if err := k8sClient.List(ctx, &dtList); err != nil {
		return nil, err
	}

	var dtcl *applicationv1alpha1.DeploymentTargetClass
	for i, d := range dtList.Items {
		if d.Name == string(dtc.Spec.DeploymentTargetClassName) {
			dtcl = &dtList.Items[i]
			break
		}
	}

	return dtcl, nil
}

// findMatchingSpaceRequestForDTC tries to find a SpaceRequest that matches the given DTC in a namespace.
// The function will return only the SpaceRequest that matches the expected environment Tier name
func findMatchingSpaceRequestForDTC(ctx context.Context, k8sClient client.Client, dtc applicationv1alpha1.DeploymentTargetClaim) (*codereadytoolchainv1alpha1.SpaceRequest, error) {
	spaceRequestList := codereadytoolchainv1alpha1.SpaceRequestList{}

	opts := []client.ListOption{
		client.InNamespace(dtc.Namespace),
		client.MatchingLabels{
			deploymentTargetClaimLabel: dtc.Name,
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

// createSpaceRequestForDTC creates a new SpaceRequest for the given DTC. It sets the expected tierName and adds the
// label indicating that it's linked with the DTC
func createSpaceRequestForDTC(ctx context.Context, k8sClient client.Client, dtc applicationv1alpha1.DeploymentTargetClaim) (*codereadytoolchainv1alpha1.SpaceRequest, error) {
	newSpaceRequest := codereadytoolchainv1alpha1.SpaceRequest{
		Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{
			TierName:           environmentTierName,
			TargetClusterRoles: []string{},
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dtc.Name,
			Namespace:    dtc.Namespace,
			Labels: map[string]string{
				deploymentTargetClaimLabel: dtc.Name,
			},
		},
	}

	err := k8sClient.Create(ctx, &newSpaceRequest)
	if err != nil {
		return nil, err
	}

	return &newSpaceRequest, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SandboxProvisionerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&applicationv1alpha1.DeploymentTargetClaim{}).
		WithEventFilter(DTCPendingDynamicProvisioningBySandbox()).
		Complete(r)
}
