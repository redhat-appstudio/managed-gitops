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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	"crypto/sha256"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	attributes "github.com/devfile/api/v2/pkg/attributes"
	"github.com/go-logr/logr"
	applicationv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	devfile "github.com/redhat-appstudio/application-service/pkg/devfile"
	gitopsdeploymentv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const deploymentSuffix = "-deployment"

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applications/finalizers,verbs=update

//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Detected AppStudio Application event:", "request", req)

	var asApplication applicationv1alpha1.Application

	if err := r.Client.Get(ctx, req.NamespacedName, &asApplication); err != nil {

		if apierrors.IsNotFound(err) {
			// A) Application has been deleted, so ensure that GitOps deployment is deleted.
			err := processDeleteGitOpsDeployment(ctx, req, r.Client, log)
			return ctrl.Result{}, err

		} else {
			log.Error(err, "unexpected error on retrieving AppStudio Application", "req", req)
			return ctrl.Result{}, err
		}
	}

	// Convert the app name to corresponding GitOpsDeployment name, ensuring that the GitOpsDeployment name fits within 64 chars
	gitopsDeplName := sanitizeAppNameWithSuffix(asApplication.Name, deploymentSuffix)

	gitopsDeployment := &gitopsdeploymentv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitopsDeplName,
			Namespace: asApplication.Namespace,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(gitopsDeployment), gitopsDeployment); err != nil {

		if apierrors.IsNotFound(err) {
			// B) GitOpsDeployment doesn't exist, but Application does, so create the GitOpsDeployment

			// Sanity check the application before we do anything more with it
			if err := validateApplication(asApplication); err != nil {
				return ctrl.Result{}, err
			}

			err := processCreateGitOpsDeployment(ctx, asApplication, r.Client, log)

			return ctrl.Result{}, err

		} else {
			log.Error(err, "unexpected error on retrieving GitOpsDeployment", "req", req)
			return ctrl.Result{}, err
		}
	}

	// C) GitOpsDeployment exists, and Application exists, so check if they differ. If so, update the old one.

	// Sanity check the application before we do anything more with it
	if err := validateApplication(asApplication); err != nil {
		return ctrl.Result{}, err
	}

	gopFromApplication, err := generateNewGitOpsDeploymentFromApplication(asApplication)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to convert Application to GitOpsDeployment: %v", err)
	}

	if reflect.DeepEqual(gopFromApplication.Spec, gitopsDeployment.Spec) {
		// D) Both exist, but there is no different, so no-op.
		log.V(sharedutil.LogLevel_Debug).Info("GitOpsDeployment '%s' is unchanged from Application, so did not require an update.", gitopsDeployment.Namespace+"/"+gitopsDeployment.Name)
		return ctrl.Result{}, nil
	}

	// Replace the old spec field with the new spec field
	gitopsDeployment.Spec = gopFromApplication.Spec

	if log.V(sharedutil.LogLevel_Debug).Enabled() {

		jsonStr, err := json.Marshal(gitopsDeployment.Spec)
		if err != nil {
			return ctrl.Result{}, err
		}
		log.V(sharedutil.LogLevel_Debug).
			Info("updating GitOpsDeployment '%s' with new spec: '%s'",
				gitopsDeployment.Namespace+"/"+gitopsDeployment.Name, string(jsonStr))

	} else {
		log.Info("updating GitOpsDeployment '%s' with new spec", gitopsDeployment.Namespace+"/"+gitopsDeployment.Name)
	}

	if err := r.Client.Update(ctx, gitopsDeployment); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// processDeleteGitOpsDeployment deletes the GitOpsDeployment that corresponds to req
func processDeleteGitOpsDeployment(ctx context.Context, req ctrl.Request, k8sClient client.Client, log logr.Logger) error {

	gitopsDeplName := sanitizeAppNameWithSuffix(req.Name, deploymentSuffix)

	gitopsDepl := &gitopsdeploymentv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitopsDeplName,
			Namespace: req.Namespace,
		},
	}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl); err != nil {

		if apierrors.IsNotFound(err) {
			// Application doesn't exist, and GitOpsDeployment also doesn't exist, so no work to do.
			return nil
		} else {
			return fmt.Errorf("unable to retrieve gitopsdepl '%s': %v", gitopsDeplName, err)
		}
	}

	if err := k8sClient.Delete(ctx, gitopsDepl); err != nil {
		return fmt.Errorf("unable to delete gitopsdepl '%s': %v", gitopsDeplName, err)
	}

	return nil

}

// processCreateGitOpsDeployment creates the GitOpsDeployment that corresponds to 'asApplication'
func processCreateGitOpsDeployment(ctx context.Context, asApplication applicationv1alpha1.Application, client client.Client, log logr.Logger) error {

	// Since the GitOpsDeployment doesn't exist, we create it.

	// Sanity check the application
	if err := validateApplication(asApplication); err != nil {
		return err
	}

	gitopsDepl, err := generateNewGitOpsDeploymentFromApplication(asApplication)
	if err != nil {
		return fmt.Errorf("unable to convert Application to GitOpsDeployment: %v", err)
	}

	log.Info("creating new GitOpsDeployment '" + gitopsDepl.Name + "' (" + string(gitopsDepl.UID) + ")")

	if err := client.Create(ctx, &gitopsDepl); err != nil {
		return fmt.Errorf("unable to create GitOpsDeployment '%s': %v", gitopsDepl.Name, err)
	}

	return nil
}

// Sanity test the application
func validateApplication(asApplication applicationv1alpha1.Application) error {

	if strings.TrimSpace(asApplication.Name) == "" {
		return fmt.Errorf("application resource has invalid name: '%s'", asApplication.Name)
	}

	if strings.TrimSpace(asApplication.Status.Devfile) == "" {
		return fmt.Errorf("application status' devfile field is empty")
	}

	_, _, _, err := getGitOpsRepoData(asApplication)
	if err != nil {
		return fmt.Errorf("unable to validate application: %v", err)
	}

	return nil

}

func getGitOpsRepoData(asApplication applicationv1alpha1.Application) (string, string, string, error) {

	curDevfile, err := devfile.ParseDevfileModel(asApplication.Status.Devfile)
	if err != nil {
		return "", "", "", fmt.Errorf("unable to parse devfile model: %v", err)
	}

	var getErr error

	// These strings are not defined as constants in the application service repo

	metadata := curDevfile.GetMetadata()

	// GitOps Repository is a required field
	gitopsURL := metadata.Attributes.GetString("gitOpsRepository.url", &getErr)
	if getErr != nil {
		return "", "", "", fmt.Errorf("unable to retrieve gitops url: %v", getErr)
	}
	if strings.TrimSpace(gitopsURL) == "" {
		return "", "", "", fmt.Errorf("gitops url is empty")
	}

	// Branch is not a required field
	branch := metadata.Attributes.GetString("gitOpsRepository.branch", &getErr)
	if getErr != nil {
		// Ignore KeyNotFoundErrors, but otherwise report the error and return
		if _, ok := (getErr).(*attributes.KeyNotFoundError); !ok {
			return "", "", "", fmt.Errorf("unable to retrieve gitops repo branch: %v", getErr)
		}
	}

	// Context is a required field
	context := metadata.Attributes.GetString("gitOpsRepository.context", &getErr)
	if getErr != nil {
		return "", "", "", fmt.Errorf("unable to retrieve gitops repo context: %v", getErr)
	}
	if strings.TrimSpace(context) == "" {
		return "", "", "", fmt.Errorf("gitops repo context is empty: %v", getErr)
	}

	return gitopsURL, branch, context, nil

}

// generateNewGitOpsDeploymentFromApplication converts the Application into a corresponding GitOpsDeployment, by
// matching their corresponding fields.
func generateNewGitOpsDeploymentFromApplication(asApplication applicationv1alpha1.Application) (gitopsdeploymentv1alpha1.GitOpsDeployment, error) {

	url, branch, context, err := getGitOpsRepoData(asApplication)
	if err != nil {
		return gitopsdeploymentv1alpha1.GitOpsDeployment{}, err
	}

	gitopsDeplName := sanitizeAppNameWithSuffix(asApplication.Name, "-deployment")

	res := gitopsdeploymentv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitopsDeplName,
			Namespace: asApplication.Namespace,
			Labels: map[string]string{
				// Add a label which contains a reference to the actual name of the parent Application resource
				"appstudio.application.name": asApplication.Name,
			},
			// When the parent is deleted, the child should be deleted too
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: asApplication.APIVersion,
				Kind:       asApplication.Kind,
				Name:       asApplication.Name,
				UID:        asApplication.UID,
			}},
		},
		Spec: gitopsdeploymentv1alpha1.GitOpsDeploymentSpec{
			Source: gitopsdeploymentv1alpha1.ApplicationSource{
				RepoURL:        url,
				Path:           context,
				TargetRevision: branch,
			},
			Destination: gitopsdeploymentv1alpha1.ApplicationDestination{},
			Type:        gitopsdeploymentv1alpha1.GitOpsDeploymentSpecType_Automated,
		},
	}

	return res, nil
}

// Ensure that the name of the GitOpsDeployment is always <= 64 characters
func sanitizeAppNameWithSuffix(appName string, suffix string) string {

	fullName := appName + suffix

	if len(fullName) < 64 {
		return fullName
	}

	sha256 := sha256.New()
	sha256.Write(([]byte)(appName))
	hashValBytes := (string)(sha256.Sum(nil))

	hashValStr := fmt.Sprintf("%x", hashValBytes)

	return hashValStr[0:32] + suffix
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&applicationv1alpha1.Application{}).
		Complete(r)
}
