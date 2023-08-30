package appstudioredhatcom

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
)

// dtcPendingDynamicProvisioningBySandbox returns a predicate which filters out
// only DeploymentTargetClaims which have been marked for dynamic provisioning by a sandbox-provisioner
// and are in Pending phase
func dtcPendingDynamicProvisioningBySandbox() predicate.Predicate {
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
			return isDeploymentClaimStatusPending(e.ObjectNew) && isDeploymentTargetClaimMarkedForDynamicProvisioning(e.ObjectNew)
		},
	}
}

// isDeploymentClaimStatusPending returns a boolean indicating whether the DeploymentTargetClaim status is pending.
// If the objects passed to this function are not DeploymentTargetClaim, the function will return false.
func isDeploymentClaimStatusPending(objectNew client.Object) bool {
	if dtc, ok := objectNew.(*applicationv1alpha1.DeploymentTargetClaim); ok {
		return dtc.Status.Phase == applicationv1alpha1.DeploymentTargetClaimPhase_Pending
	}
	return false
}

// isDeploymentTargetClaimMarkedForDynamicProvisioning returns a boolean indicating whether the DeploymentTargetClaim
// is marked for dynamic provisioning. If the objects passed to this function are not DeploymentTargetClaim, the function will return false.
func isDeploymentTargetClaimMarkedForDynamicProvisioning(objectNew client.Object) bool {
	if dtc, ok := objectNew.(*applicationv1alpha1.DeploymentTargetClaim); ok {
		return isMarkedForDynamicProvisioning(*dtc)
	}
	return false
}
