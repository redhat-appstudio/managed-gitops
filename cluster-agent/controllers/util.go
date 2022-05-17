package controllers

import (
	"context"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	WaitForArgoCDToPerformFinalizerDeleteTimeout    = time.Minute * 5
	WaitForArgoCDToPerformNonfinalizerDeleteTimeout = time.Minute * 2
)

const (
	argoCDResourcesFinalizer = "resources-finalizer.argocd.argoproj.io"
)

func DeleteArgoCDApplication(ctx context.Context, appFromList appv1.Application, eventClient client.Client, log logr.Logger) error {

	log = log.WithValues("name", appFromList.Name, "namespace", appFromList.Namespace, "uid", string(appFromList.UID))

	log.Info("Attempting to delete Argo CD Application CR " + appFromList.Name)

	app := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appFromList.Name,
			Namespace: appFromList.Namespace,
		},
	}

	if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

		if apierr.IsNotFound(err) {
			log.Info("unable to locate application which previously existed: " + appFromList.Name)
			return nil
		}

		log.Error(err, "unable to retrieve application which previously existed: "+appFromList.Name)
		return err
	}

	if app.DeletionTimestamp == nil {

		// Ensure finalizer is set
		{
			containsFinalizer := false
			for _, finalizer := range app.Finalizers {
				if finalizer == argoCDResourcesFinalizer {
					containsFinalizer = true
					break
				}
			}

			if !containsFinalizer {
				app.Finalizers = append(app.Finalizers, argoCDResourcesFinalizer)

				if err := eventClient.Update(ctx, app); err != nil {
					log.Error(err, "unable to update application with finalizer: "+app.Name)
					return err
				}
			}
		}

		// Tell K8s to start deleting the Application, which triggers Argo CD to delete children
		policy := metav1.DeletePropagationForeground
		if err := eventClient.Delete(ctx, app, &client.DeleteOptions{PropagationPolicy: &policy}); err != nil {
			log.Error(err, "unable to delete application with finalizer: "+app.Name)
			return err
		}
	}

	backoff := sharedutil.ExponentialBackoff{
		Factor: 2,
		Min:    time.Millisecond * 200,
		Max:    time.Second * 10,
		Jitter: true,
	}

	// Wait for Argo CD to delete the application.
	// We wait either (now+5 minutes), or (deletionTimestamp+5 minutes), whichever is soonest.
	success := false
	var expirationTime time.Time
	if app.DeletionTimestamp == nil {
		expirationTime = time.Now().Add(WaitForArgoCDToPerformFinalizerDeleteTimeout) // wait X minutes for Argo CD to delete
	} else {
		expirationTime = app.DeletionTimestamp.Time.Add(WaitForArgoCDToPerformFinalizerDeleteTimeout)
	}

	for {

		if time.Now().After(expirationTime) {
			log.V(sharedutil.LogLevel_Warn).Error(nil, "Argo CD application finalizer-based delete expired in deleteArgoCDApplication")
			break
		}

		if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

			if apierr.IsNotFound(err) {
				log.Info("The Argo CD application has been successfully deleted.")
				success = true
				// Success! The Application (and its resources) have been deleted.
				break
			} else {
				log.Error(err, "unable to retrieve application being deleted: "+app.Name)
			}

		}

		backoff.DelayOnFail(ctx)
	}

	// If the Argo CD was unable to delete the application properly, then just remove the finalizer and
	// wait for it to go away (up to 2 minutes)
	if !success {

		backoff.Reset()

		// Add up to X minutes (eg 2 minutes) to wait for the deletion
		expirationTime = expirationTime.Add(WaitForArgoCDToPerformNonfinalizerDeleteTimeout)

		for {

			if time.Now().After(expirationTime) {
				log.Error(nil, "Argo CD application non-finalizer-based delete expired in deleteArgoCDApplication")
				success = false
				break
			}

			if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

				if apierr.IsNotFound(err) {
					log.Info("The Argo CD application has been successfully deleted, after the finalizer was removed.")
					// Success! The Application (and its resources) have been deleted.
					success = true
					break
				} else {
					// A generic retrieve error occurred
					log.Error(err, "unable to retrieve Application in deleteArgoCDApplication")
					continue
				}
			} else {

				if len(app.Finalizers) != 0 {
					// If the application exists, and it has a finalizer, remove it finalizer and try again
					app.Finalizers = []string{}
					if err := eventClient.Update(ctx, app); err != nil {
						log.Error(err, "unable to remove finalizer from app: "+app.Name)
						continue
					}

				}
			}

			backoff.DelayOnFail(ctx)
		}
	}

	if !success {
		log.Info("Application was not successfully deleted: " + app.Name)
	} else {
		log.Info("Application was successfully deleted: " + app.Name)
	}

	return nil
}
