package eventloop

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ControllerEventLoop struct {
	eventLoopInputChannel chan controllerEventLoopEvent
}

func NewControllerEventLoop() *ControllerEventLoop {
	channel := make(chan controllerEventLoopEvent)

	res := &ControllerEventLoop{}
	res.eventLoopInputChannel = channel

	go controllerEventLoopRouter(channel)

	return res

}

type controllerEventLoopEvent struct {
	request ctrl.Request

	// TODO: DEBT - depending on how controller-runtime caching works, including this in the struct might be a bad idea:
	client client.Client
}

func (evl *ControllerEventLoop) EventReceived(req ctrl.Request, client client.Client) {

	event := controllerEventLoopEvent{request: req, client: client}
	evl.eventLoopInputChannel <- event
}

func controllerEventLoopRouter(input chan controllerEventLoopEvent) {

	ctx := context.Background()

	log := log.FromContext(ctx)

	// taskRetryLoop := newTaskRetryLoop()

	log.Info("controllerEventLoopRouter started")

	for {
		newEvent := <-input

		// mapKey := newEvent.request.Name + "-" + newEvent.request.Namespace

		task := processEventTask{
			event: controllerEventLoopEvent{
				request: newEvent.request,
				client:  newEvent.client,
			},
		}

		for {
			retry, err := task.performTask(ctx)

			if err != nil {
				log.Error(err, "")
			}

			if !retry {
				break
			}
		}
	}

}

type processEventTask struct {
	event controllerEventLoopEvent
}

func (task *processEventTask) performTask(taskContext context.Context) (bool, error) {

	log := log.FromContext(taskContext)

	eventClient := task.event.client

	operation := &operation.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      task.event.request.Name,
			Namespace: task.event.request.Namespace,
		},
	}

	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(operation), operation); err != nil {
		// TODO: Log me better
		log.Error(err, err.Error())

		if apierr.IsNotFound(err) {
			// work is complete
			return false, nil

		} else {
			// generic error
			return true, err
		}
	}

	dbQueries, err := db.NewProductionPostgresDBQueries(true)
	if err != nil {
		// TODO: Log me better
		return true, err
	}

	dbOperation := db.Operation{
		Operation_id: operation.Spec.OperationID,
	}
	if err := dbQueries.UncheckedGetOperationById(taskContext, &dbOperation); err != nil {

		// TODO: Log me better
		log.Error(err, err.Error())

		if db.IsResultNotFoundError(err) {
			// log as warning

			// no corresponding db operation, so no work to do
			return false, nil
		} else {
			// some other generic error
			return true, err
		}

	}

	dbGitopsEngineInstance := &db.GitopsEngineInstance{
		Gitopsengineinstance_id: dbOperation.Instance_id,
	}
	if err := dbQueries.UncheckedGetGitopsEngineInstanceById(taskContext, dbGitopsEngineInstance); err != nil {
		// TODO: Log me better
		log.Error(err, err.Error())

		if db.IsResultNotFoundError(err) {
			// log as warning

			// no corresponding db operation, so no work to do
			return false, nil
		} else {
			// some other generic error
			return true, err
		}
	}

	// TODO: DEBT: ensure that the engine cluster corresponds to this cluster-agent

	argoCDNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbGitopsEngineInstance.Namespace_name,
			Namespace: dbGitopsEngineInstance.Namespace_name,
		},
	}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(argoCDNamespace), argoCDNamespace); err != nil {

		// TODO: Log me better
		log.Error(err, err.Error())

		if db.IsResultNotFoundError(err) {
			// TODO: Log as severe
			// TODO: include todo reference to multi argocd instance story

			// no corresponding db operation, so no work to do
			return false, nil
		} else {
			// some other generic error
			return true, err
		}
	}

	// TODO: Verify that the UID matches

	if dbOperation.Resource_type == db.OperationResourceType_Application {
		err := processOperation_Application(taskContext, dbOperation, *operation, dbQueries, *argoCDNamespace, eventClient, log)
		log.Error(err, err.Error())

	} else {
		log.Error(nil, "SEVERE: unrecognized resource type: "+dbOperation.Resource_type)
		return false, nil
	}

	return false, nil
}

func processOperation_Application(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation, dbQueries db.DatabaseQueries, argoCDNamespace corev1.Namespace, eventClient client.Client, log logr.Logger) error {

	// Sanity check
	if dbOperation.Resource_id == "" {
		return fmt.Errorf("resource id was nil while processing operation: " + crOperation.Name)
	}

	application := &db.Application{
		Application_id: dbOperation.Resource_id,
	}

	if err := dbQueries.UncheckedGetApplicationById(ctx, application); err != nil {

		if db.IsResultNotFoundError(err) {
			// The application db entry no longer exists, so delete the corresponding application cr

			// Find the Application that has the corresponding databaseId label
			list := appv1.ApplicationList{}
			labelSelector := labels.NewSelector()
			req, err := labels.NewRequirement("databaseId", selection.Equals, []string{application.Application_id})
			if err != nil {
				// TODO: log me
				return err
			}
			labelSelector = labelSelector.Add(*req)
			if err := eventClient.List(ctx, &list, &client.ListOptions{
				Namespace:     argoCDNamespace.Name,
				LabelSelector: labelSelector,
			}); err != nil {
				// TODO: log me
				return err
			}

			if len(list.Items) > 1 {
				// Sanity test: should really only ever be 0 or 1
				log.Error(nil, "unexpected number of items in list", "length", len(list.Items))
			}

			var firstDeletionErr error
			for _, item := range list.Items {
				// Delete all Argo CD applications with the corresponding database label (there should be only one)
				err := deleteArgoCDApplication(ctx, item, eventClient, log)
				if firstDeletionErr == nil {
					// TODO: Log me
					firstDeletionErr = err
				}
			}

			// TODO: update operation status

			// TODO: Log first deletion err

			return firstDeletionErr

		} else {
			// TODO: log generic err
			log.Error(err, err.Error())
			return err
		}
	}

	app := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      application.Name,
			Namespace: argoCDNamespace.Name,
		},
	}

	if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

		if apierr.IsNotFound(err) {
			// The Application CR doesn't exist, so we need to create it

			if err := json.Unmarshal([]byte(application.Spec_field), app); err != nil {
				log.Error(err, "SEVERE: unable to unmarshal application spec field: "+app.Name)
				// We return nil here, because there's likely nothing else that can be done to fix this.
				// Thus there is no need to keep retrying.
				return nil
			}

			if err := eventClient.Create(ctx, app, &client.CreateOptions{}); err != nil {
				log.Error(err, err.Error()) // TODO: Log me
				// This may or may not be salvageable depending on the error; we should figure out which
				// error messages mean unsalvageable, and not wait for them.
				return err
			}

			return nil

		} else {
			// TODO: log generic error
			log.Error(err, err.Error())
			return err
		}

	}

	// The application CR exists, and the database entry exists, so check if there is any
	// difference between them.

	specFieldApp := &appv1.Application{}

	if err := json.Unmarshal([]byte(application.Spec_field), specFieldApp); err != nil {
		log.Error(err, "SEVERE: unable to unmarshal application spec field: "+app.Name)
		// We return nil here, because there's likely nothing else that can be done to fix this.
		// Thus there is no need to keep retrying.
		return nil
	}

	var specDiff string
	if !reflect.DeepEqual(specFieldApp.Spec.Source, app.Spec.Source) {
		specDiff = "spec.source fields differ"
	} else if !reflect.DeepEqual(specFieldApp.Spec.Destination, app.Spec.Destination) {
		specDiff = "spec.destination fields differ"
	} else if specFieldApp.Spec.Project != app.Spec.Project {
		specDiff = "spec project fields differ"
	}

	if specDiff != "" {
		app.Spec.Destination = specFieldApp.Spec.Destination
		app.Spec.Source = specFieldApp.Spec.Source
		app.Spec.Project = specFieldApp.Spec.Project

		if err := eventClient.Update(ctx, app); err != nil {
			log.Error(err, "unable to update application after difference detected: "+app.Name)
			return err
		}

	} else {
		log.V(sharedutil.LogLevel_Debug).Info("no changes detected in application, so no update needed")
	}

	return nil
}

func deleteArgoCDApplication(ctx context.Context, appFromList appv1.Application, eventClient client.Client, log logr.Logger) error {

	log = log.WithValues("name", appFromList.Name, "namespace", appFromList.Namespace, "uid", string(appFromList.UID))

	app := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appFromList.Name,
			Namespace: appFromList.Namespace,
		},
	}

	if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

		if apierr.IsNotFound(err) {
			// TODO: log as warn
			log.Info("unable to locate application which previously existed: " + appFromList.Name)
			return nil
		}

		log.Error(err, "unable to retrieve application which previously existed: "+appFromList.Name)
		return err

	}

	// Ensure finalizer is set
	{
		containsFinalizer := false
		for _, finalizer := range app.Finalizers {
			// TODO: add to constant
			if finalizer == "resources-finalizer.argocd.argoproj.io" {
				containsFinalizer = true
				break
			}
		}

		if !containsFinalizer {
			// TODO: Add to constant (is there one from Argo CD we can use?)
			app.Finalizers = append(app.Finalizers, "resources-finalizer.argocd.argoproj.io")

			if err := eventClient.Update(ctx, app); err != nil {
				log.Error(err, "unable to update application with finalizer: "+app.Name)
				return err
			}
		}
	}

	if app.DeletionTimestamp == nil {

		// Tell K8s to start deleting the Application, which triggers Argo CD to delete children
		policy := metav1.DeletePropagationForeground
		if err := eventClient.Delete(ctx, app, &client.DeleteOptions{PropagationPolicy: &policy}); err != nil {
			log.Error(err, "unable to delete application with finalizer: "+app.Name)
			return err
		}
	}

	backoff := util.ExponentialBackoff{
		Factor: 2,
		Min:    time.Millisecond * 200,
		Max:    time.Second * 10,
		Jitter: true,
	}

	success := false
	// expired := false
	var expirationTime time.Time
	if app.DeletionTimestamp == nil {
		expirationTime = time.Now().Add(time.Minute * 5) // TODO: constant
	} else {
		expirationTime = app.DeletionTimestamp.Time.Add(time.Minute * 5) // TODO: constant
	}

	for {

		if time.Now().After(expirationTime) {
			// TODO: log me
			// expired = true
			break
		}

		if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

			if apierr.IsNotFound(err) {
				// TODO: log me
				success = true
				// Success! The Application (and its resources) have been deleted.
				break
			} else {
				log.Error(err, "unable to retrieve application being deleted: "+app.Name)
			}

		}

		backoff.DelayOnFail(ctx)
	}

	if !success {

		// Add up to 2 minutes to wait for the deletion
		expirationTime = expirationTime.Add(2 * time.Minute) // TODO: constant

		for {

			if time.Now().After(expirationTime) {
				// TODO: log me
				success = false
				break
			}

			if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

				if apierr.IsNotFound(err) {
					// Success! The Application (and its resources) have been deleted.
					success = true
					break
				} else {

					// Remove the finalizer
					app.Finalizers = []string{}
					if err := eventClient.Update(ctx, app); err != nil {
						log.Error(err, "unable to remove finalizer from app: "+app.Name)
						return err
					}
				}
			}
		}
	}

	if !success {
		log.Info("Application was not successfully deleted: " + app.Name)
	} else {
		log.Info("Application was successfully deleted: " + app.Name)
	}

	return nil
}
