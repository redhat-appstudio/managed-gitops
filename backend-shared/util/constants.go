package util

const (
	// Annotation to indicate that the binding controller completed the binding process
	AnnBindCompleted string = "dt.appstudio.redhat.com/bind-complete"

	// Annotation to indicate that the binding controller bind the DTC to a DT.
	// In practice it means that the controller set the DTC.spec.Target to the value of DT.Name
	AnnBoundByController string = "dt.appstudio.redhat.com/bound-by-controller"

	// Annotation to indicate that a DT should be dynamically provisioned for the
	// DTC by the provisioner whose name appear in the value
	AnnTargetProvisioner string = "provisioner.appstudio.redhat.com/dt-provisioner"

	// Finalizer added by the binding controller to handle the deletion of DeploymentTargetClaim.
	FinalizerBinder string = "binder.appstudio.redhat.com/finalizer"

	AnnBinderValueYes string = "yes"
)
