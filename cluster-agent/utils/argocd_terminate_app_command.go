package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	argoio "github.com/argoproj/argo-cd/v2/util/io"
	"github.com/argoproj/gitops-engine/pkg/sync/common"
	"github.com/go-logr/logr"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file is loosely based on the 'argocd terminate-op' CLI command (https://github.com/argoproj/argo-cd/blob/0a46d37fc6af9fe0aa963bdd845e3d799aa0320d/cmd/argocd/commands/app.go#L2017)

// TerminateOperation calls the Argo CD GRPC API to terminates a synchronize operation on an Argo CD Application, if one is running.
func TerminateOperation(ctx context.Context, appName string, argocdNamespace corev1.Namespace,
	credentialService *CredentialService,
	k8sClient client.Client, expireDuration time.Duration, log logr.Logger) error {

	_, acdClient, err := credentialService.GetArgoCDLoginCredentials(ctx, argocdNamespace.Name,
		string(argocdNamespace.UID), false, k8sClient)

	if err != nil {
		return err
	}

	return terminateOperation(ctx, appName, argocdNamespace, acdClient, k8sClient, expireDuration, log)
}

func terminateOperation(ctx context.Context, appName string, argocdNamespace corev1.Namespace,
	acdClient apiclient.Client, k8sClient client.Client, expireDuration time.Duration, log logr.Logger) error {

	conn, appIf, err := acdClient.NewApplicationClient()
	if err != nil {
		return fmt.Errorf("unable to create application client for terminate operation: %v", err)
	}

	defer argoio.Close(conn)
	_, err = appIf.TerminateOperation(ctx, &applicationpkg.OperationTerminateRequest{Name: &appName})
	if err != nil {
		return err
	}

	expireTime := time.Now().Add(expireDuration)

	backoff := sharedutil.ExponentialBackoff{Factor: 2, Min: time.Duration(500 * time.Microsecond), Max: time.Duration(5 * time.Second), Jitter: true}
	for {

		if time.Now().After(expireTime) {
			return fmt.Errorf("application operation never terminated: %s", appName)
		}

		// Retrieve the corresponding Application, and wait for the operation phase to be complete.
		application := &appv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: argocdNamespace.Name,
			},
		}

		log := log.WithValues("appName", appName, "appNamespace", argocdNamespace.Name)

		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(application), application); err != nil {

			if apierr.IsNotFound(err) {
				log.Info("application no longer exists, so exiting terminate operation")
				return nil
			}
			log.Error(err, "unable to retrieve application from namespace")

		} else {

			if application.Status.OperationState != nil {
				operationPhase := application.Status.OperationState.Phase

				if operationPhase != "" && operationPhase != common.OperationRunning && operationPhase != common.OperationTerminating {
					log.Info("application operation is no longer running (or not running).")
					return nil
				}
			}
		}

		// An error occurred, or operation state is still not complete, so wait
		backoff.DelayOnFail(ctx)

	}

}
