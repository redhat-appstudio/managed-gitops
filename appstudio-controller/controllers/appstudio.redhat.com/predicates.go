package appstudioredhatcom

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
)

// DTCPendingDynamicProvisioningBySandbox returns a predicate which filters out
// only DeploymentTargetClaims which have been marked for dynamic provisioning by a sandbox-provisioner
// and are in Pending phase
func DTCPendingDynamicProvisioningBySandbox() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return IsDeploymentClaimStatusPending(e.ObjectNew) && IsDeploymentTargetClaimMarkedForDynamicProvisioning(e.ObjectNew)
		},
	}
}

// IsDeploymentClaimStatusPending returns a boolean indicating whether the DeploymentTargetClaim status is pending.
// If the objects passed to this function are not DeploymentTargetClaim, the function will return false.
func IsDeploymentClaimStatusPending(objectNew client.Object) bool {
	if dtc, ok := objectNew.(*applicationv1alpha1.DeploymentTargetClaim); ok {
		return dtc.Status.Phase == applicationv1alpha1.DeploymentTargetClaimPhase_Pending
	}
	return false
}

// IsDeploymentTargetClaimMarkedForDynamicProvisioning returns a boolean indicating whether the DeploymentTargetClaim
// is marked for dynamic provisioning. If the objects passed to this function are not DeploymentTargetClaim, the function will return false.
func IsDeploymentTargetClaimMarkedForDynamicProvisioning(objectNew client.Object) bool {
	if dtc, ok := objectNew.(*applicationv1alpha1.DeploymentTargetClaim); ok {
		return isMarkedForDynamicProvisioning(*dtc)
	}
	return false
}
