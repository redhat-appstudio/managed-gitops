/*
Copyright 2022.

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

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationPromotionRunReconciler reconciles a ApplicationPromotionRun object
type ApplicationPromotionRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationpromotionruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationpromotionruns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationpromotionruns/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile

func (r *ApplicationPromotionRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	fmt.Println("ApplicationPromotionRun event: ", req)

	return ctrl.Result{}, nil
}

// Proof-of-concept/pseudocode Reconcile. Comment out the above Reconcile, and rename this to Reconcile, when starting working on this.
func (r *ApplicationPromotionRunReconciler) ReconcilePOC(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	promotionRun := &appstudioshared.ApplicationPromotionRun{}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(promotionRun), promotionRun); err != nil {
		if apierr.IsNotFound(err) {
			// Nothing more to do!
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, fmt.Errorf("unable to retrieve ApplicationPromotionRun: %v", err)
		}
	}

	if promotionRun.Status.State == appstudioshared.PromotionRunState_Complete {
		// Ignore promotion runs that have completed.
		return ctrl.Result{}, nil
	}

	if err := checkForExistingActivePromotions(ctx, *promotionRun, r.Client); err != nil {
		// TODO: GITOPSRVCE-157 - Add error to error occurred conditions field
		fmt.Println(err)
		return ctrl.Result{}, nil
	}

	// If this is a automated promotion, ignore it for now
	if promotionRun.Spec.AutomatedPromotion.InitialEnvironment != "" {
		// TODO: GITOPSRVCE-157 - Add error to error occurred conditions field
		fmt.Println("Note: Automated promotion not yet supported as of this writing.")
		return ctrl.Result{}, nil
	}

	if promotionRun.Spec.ManualPromotion.TargetEnvironment == "" {
		fmt.Println("Target environment has invalid value in " + promotionRun.Name)
		// TODO: GITOPSRVCE-157 - Update Error Occurred status field to indicate the target environment value is invalid
		return ctrl.Result{}, nil
	}

	// 1) Locate the binding that this PromotionRun is targetting
	binding, err := locateTargetManualBinding(ctx, *promotionRun, r.Client)
	if err != nil {
		return ctrl.Result{}, nil
	}

	if promotionRun.Status.State != appstudioshared.PromotionRunState_Active {
		promotionRun.Status.State = appstudioshared.PromotionRunState_Active

		sharedutil.LogAPIResourceChangeEvent(promotionRun.Namespace, promotionRun.Name, promotionRun, sharedutil.ResourceModified, log)
		if err := r.Client.Update(ctx, promotionRun); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update PromotionRun state: %v", err)
		}
		// TODO: GITOPSRVCE-157 - log this action as debug
	}

	// Verify: activebindings should not have a value which differs from the value specified in promotionrun.spec
	if len(promotionRun.Status.ActiveBindings) > 0 {
		for _, existingActiveBinding := range promotionRun.Status.ActiveBindings {
			if existingActiveBinding != binding.Name {

				fmt.Printf("The binding changed after the PromotionRun first start. "+
					"The .spec fields of the PromotionRun are immutable, and should not be changed "+
					"after being created. old-binding: %s, new-binding: %s\n", existingActiveBinding, binding.Name)

				// TODO: GITOPSRVCE-157 - Update error occurred status field

				return ctrl.Result{}, nil
			}
		}
	}

	// TODO: GITOPSRVCE-157 - Verify that the snapshot refered in binding.spec.snapshot actually exists, if not, throw error

	// 2) Set the Binding to target the expected snapshot, if not already done
	if binding.Spec.Snapshot != promotionRun.Spec.Snapshot || len(promotionRun.Status.ActiveBindings) == 0 {
		binding.Spec.Snapshot = promotionRun.Spec.Snapshot
		sharedutil.LogAPIResourceChangeEvent(promotionRun.Namespace, promotionRun.Name, promotionRun, sharedutil.ResourceModified, log)
		if err := r.Client.Update(ctx, promotionRun); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update Binding '%s' snapshot: %v", binding.Name, err)
		}
		// TODO: GITOPSRVCE-157 - log this action

		promotionRun.Status.ActiveBindings = []string{binding.Name}
		sharedutil.LogAPIResourceChangeEvent(promotionRun.Namespace, promotionRun.Name, promotionRun, sharedutil.ResourceModified, log)
		if err := r.Client.Update(ctx, promotionRun); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to update PromotionRun active binding: %v", err)
		}

		return ctrl.Result{}, nil
	}

	// 3) Wait for the environment binding to create all of the expected GitOpsDeployments
	if len(binding.Status.GitOpsDeployments) != len(binding.Spec.Components) {
		// TODO: GITOPSRVCE-157 - Update promotionRun.Status.EnvironmentStatus.DisplayName, indicating that we are waiting for all the gitopsdeployments
		return ctrl.Result{}, nil
	}

	gitopsRepositoryCommitsWithSnapshot := []string{} /* we need a mechanism to tell which gitops repository revision corresponds to which snapshot*/

	// 4) Wait for all the GitOpsDeployments of the binding to have the expected state
	waitingGitOpsDeployments := []string{}

	for _, gitopsDeploymentName := range binding.Status.GitOpsDeployments {

		gitopsDeployment := &apibackend.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gitopsDeploymentName.GitOpsDeployment,
				Namespace: binding.Namespace,
			},
		}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(gitopsDeployment), gitopsDeployment); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to retrieve gitopsdeployment '%s', %v", gitopsDeployment.Name, err)
		}

		// Must have status of Synced/Healthy
		if gitopsDeployment.Status.Sync.Status == apibackend.SyncStatusCodeSynced && gitopsDeployment.Status.Health.Status != apibackend.HeathStatusCodeHealthy {
			// TODO: GITOPSRVCE-157 - Update Status.EnvironmentStatus to indicate that the gitopsdeployment is not synced/healthy
			waitingGitOpsDeployments = append(waitingGitOpsDeployments, gitopsDeployment.Name)
			continue
		}

		// Argo CD must have deployed at least one of the commits that include the Snapshot container images
		match := false
		for _, snapshotCommit := range gitopsRepositoryCommitsWithSnapshot {
			if gitopsDeployment.Status.Sync.Revision == snapshotCommit {
				match = true
				break
			}
		}
		if !match {
			waitingGitOpsDeployments = append(waitingGitOpsDeployments, gitopsDeployment.Name)
			continue
		}
	}

	// TODO: GITOPSRVCE-157 - Implement a time limit on the spec of Promotion Run, and fail if the conditions aren't met in the timeframe.

	if len(waitingGitOpsDeployments) > 0 {
		fmt.Println("Waiting for GitOpsDeployments to have expected commit/sync/health:", waitingGitOpsDeployments)
		return ctrl.Result{RequeueAfter: time.Second * 15}, nil
	}

	// All the GitOpsDeployments are synced/healthy, and they are synced with a commit that includes the target snapshot.
	promotionRun.Status.ActiveBindings = []string{}
	promotionRun.Status.CompletionResult = appstudioshared.PromotionRunCompleteResult_Success
	promotionRun.Status.State = appstudioshared.PromotionRunState_Complete
	promotionRun.Status.ActiveBindings = []string{binding.Name}
	sharedutil.LogAPIResourceChangeEvent(promotionRun.Namespace, promotionRun.Name, promotionRun, sharedutil.ResourceModified, log)
	if err := r.Client.Update(ctx, promotionRun); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to update PromotionRun on successful completion: %v", err)
	}

	// TODO: GITOPSRVCE-157 - Update promotionRun.Status.EnvironmentStatus to indicate which GitOpsDeployments we are still waiting for

	// TODO: GITOPSRVCE-157 - Update promotionRun.Status.EnvironmentStatus

	return ctrl.Result{}, nil
}

// checkForExistingActivePromotions ensures that there are no other promotions that are active on this application.
// - only one promotion should be active at a time on a single Application
func checkForExistingActivePromotions(ctx context.Context, reconciledPromotionRun appstudioshared.ApplicationPromotionRun, k8sClient client.Client) error {

	otherPromotionRuns := &appstudioshared.ApplicationPromotionRunList{}
	if err := k8sClient.List(ctx, otherPromotionRuns); err != nil {
		// log me
		return err
	}

	for _, otherPromotionRun := range otherPromotionRuns.Items {
		if otherPromotionRun.Status.State == appstudioshared.PromotionRunState_Complete {
			// Ignore completed promotions (these are no longer active)
			continue
		}

		if otherPromotionRun.Spec.Application != reconciledPromotionRun.Spec.Application {
			// Ignore promotions with applications that don't match the one we are reconciling
			continue
		}

		if otherPromotionRun.ObjectMeta.CreationTimestamp.Before(&reconciledPromotionRun.CreationTimestamp) {
			return fmt.Errorf("another PromotionRun is already active. Only one waiting/active PromotionRun should exist per Application: %s", otherPromotionRun.Name)
		}

	}

	return nil
}

func locateTargetManualBinding(ctx context.Context, promotionRun appstudioshared.ApplicationPromotionRun, k8sClient client.Client) (appstudioshared.ApplicationSnapshotEnvironmentBinding, error) {

	// Locate the corresponding binding

	bindingList := appstudioshared.ApplicationSnapshotEnvironmentBindingList{}
	if err := k8sClient.List(ctx, &bindingList); err != nil {
		return appstudioshared.ApplicationSnapshotEnvironmentBinding{}, fmt.Errorf("unable to list bindings: %v", err)
	}

	for _, binding := range bindingList.Items {

		if binding.Spec.Application == promotionRun.Spec.Application && binding.Spec.Environment == promotionRun.Spec.ManualPromotion.TargetEnvironment {
			return binding, nil
		}
	}

	return appstudioshared.ApplicationSnapshotEnvironmentBinding{},
		fmt.Errorf("unable to locate binding with application '%s' and target environment '%s'",
			promotionRun.Spec.Application, promotionRun.Spec.ManualPromotion.TargetEnvironment)

}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationPromotionRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appstudioshared.ApplicationPromotionRun{}).
		Owns(&appstudioshared.ApplicationSnapshotEnvironmentBinding{}).
		Complete(r)
}
