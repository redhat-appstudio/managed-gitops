package appstudioredhatcom

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
)

// SpaceRequestReadyPredicate returns a predicate which filters out
// only SpaceRequests whose Ready Status are set to true
func DeploymentTargetDeletePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return hasDTDeletionTimestampChanged(e.ObjectOld, e.ObjectNew)
		},
	}
}

// hasDTDeletionTimestampChanged returns a boolean indicating whether the deletionTimeStamp is NonZero.
// If the objects passed to this function are not DeploymentTarget, the function will return false.
func hasDTDeletionTimestampChanged(objectOld, objectNew client.Object) bool {
	if newDeploymentTarget, ok := objectNew.(*applicationv1alpha1.DeploymentTarget); ok {
		return newDeploymentTarget.GetDeletionTimestamp() != nil
	}
	return false
}
