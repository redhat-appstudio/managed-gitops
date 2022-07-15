package eventloop

import (
	"context"
	"fmt"
	"github.com/argoproj/argo-cd/v2/common"
	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errOperationIDNotFound   = "resource ID was nil while processing operation: \"%s\""
	errGenericDB             = "unable to retrieve database \"%s\" row from database"
	errRowNotFound           = "the \"%s\" row no longer exists in the database"
	errPrivateSecretNotFound = "Argo CD Private Repository secret doesn't exist: \"%s\""
	errPrivateSecretCreate   = "Unable to create Argo CD Repository secret: \"%s\""
	errGetPrivateSecret      = "unexpected error on retrieve Argo CD secret: \"%s\""
	errUpdatePrivateSecret   = "unable to update Argo CD Private Repository secret's \"%s\""
)

// processOperation_RepositoryCredentials processes the given operation as a RepositoryCredentials operation.
// It returns true if the operation should be retried, and false otherwise.
// It returns an error if there was an error processing the operation.
func processOperation_RepositoryCredentials(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation, dbQueries db.DatabaseQueries,
	argoCDNamespace corev1.Namespace, eventClient client.Client, l logr.Logger) (bool, error) {
	const retry, noRetry = true, false

	fmt.Println("")
	fmt.Println("HELLOOOOOOO WORLD !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Println("")

	if dbOperation.Resource_id == "" {
		return retry, fmt.Errorf(errOperationIDNotFound, crOperation.Name)
	}

	// 2) Retrieve the RepositoryCredentials CR from the database.
	dbRepositoryCredentials, err := dbQueries.GetRepositoryCredentialsByID(ctx, dbOperation.Resource_id)
	if err != nil {
		if db.IsResultNotFoundError(err) {
			l.Error(err, errRowNotFound, dbOperation.Resource_id)
			// TODO: Delete the corresponding repositoryCredentials CR, if it exists
			// TODO: Add a finalizer to the CR, to make sure it is deleted only when the corresponding ArgoCD secret has been deleted as well
			// // 1. Find the associated RepostitoryCredentials CR (How?)
			// // 2. Find the ArgoCD secret that is associated with the RepositoryCredentials CR.
			//argoCDSecret := &corev1.Secret{
			//	ObjectMeta: metav1.ObjectMeta{
			//		Name:      dbRepositoryCredentials.SecretObj,
			//		Namespace: argoCDNamespace.Name,
			//	},
			//}
			//if err := eventClient.Get(ctx, client.ObjectKeyFromObject(argoCDSecret), argoCDSecret); err != nil {
			//	if apierr.IsNotFound(err) {
			//		l.Error(err, "Argo CD Private Repository secret doesn't exist: "+argoCDSecret.Name)
			//		// No need to retry, as it's already been deleted.
			//		return noRetry, nil
			//	} else {
			//		l.Error(err, "unexpected error on retrieve Argo CD secret")
			//		// some other generic error
			//		return retry, err
			//	}
			//}
			return noRetry, err
		} else {
			// some other generic error
			l.Error(err, errGenericDB, dbOperation.Resource_id)
			return retry, err
		}
	}

	l = l.WithValues("repositoryCredentialsRow", dbRepositoryCredentials.RepositoryCredentialsID)

	// 3) Retrieve ArgoCD secret from the cluster.
	argoCDSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbRepositoryCredentials.SecretObj,
			Namespace: argoCDNamespace.Name,
		},
	}

	l = l.WithValues("ArgoCD Repository Secret", argoCDSecret.Name)

	if err = eventClient.Get(ctx, client.ObjectKeyFromObject(argoCDSecret), argoCDSecret); err != nil {
		if apierr.IsNotFound(err) {
			l.Error(err, errPrivateSecretNotFound, argoCDSecret.Name)
			l.Info("Creating Argo CD Private Repository secret", "secret", argoCDSecret.Name)

			repoCredToSecret(dbRepositoryCredentials, argoCDSecret)

			errCreateArgoCDSecret := eventClient.Create(ctx, argoCDSecret, &client.CreateOptions{})
			if errCreateArgoCDSecret != nil {
				l.Error(errCreateArgoCDSecret, errPrivateSecretCreate, argoCDSecret.Name)
			}

			return retry, errCreateArgoCDSecret

		} else {
			l.Error(err, errGetPrivateSecret, argoCDSecret.Name)
			return retry, err
		}
	}

	// 4. Check if the Argo CD secret has the correct data, and if not, update it with the data from the database.
	// https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories
	if !reflect.DeepEqual(argoCDSecret.Name, dbRepositoryCredentials.SecretObj) {
		l.Info("Updating Argo CD Private Repository secret", "secret name", argoCDSecret.Name)
		argoCDSecret.Name = dbRepositoryCredentials.SecretObj
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "name")
			return retry, err
		}
	}

	if argoCDSecret.StringData["url"] != "" {
		if !reflect.DeepEqual(argoCDSecret.StringData["url"], dbRepositoryCredentials.PrivateURL) {
			l.Info("Updating Argo CD Private Repository secret", "URL", dbRepositoryCredentials.PrivateURL)
			argoCDSecret.StringData["url"] = dbRepositoryCredentials.PrivateURL
			if err = eventClient.Update(ctx, argoCDSecret); err != nil {
				l.Error(err, errUpdatePrivateSecret, "URL")
				return retry, err
			}
		}
	}

	if argoCDSecret.StringData["password"] != "" {
		if !reflect.DeepEqual(argoCDSecret.StringData["password"], dbRepositoryCredentials.AuthPassword) {
			l.Info("Updating Argo CD Private Repository secret", "password", dbRepositoryCredentials.AuthPassword)
			argoCDSecret.StringData["password"] = dbRepositoryCredentials.AuthPassword
			if err = eventClient.Update(ctx, argoCDSecret); err != nil {
				l.Error(err, errUpdatePrivateSecret, "password")
				return retry, err
			}
		}
	}

	if argoCDSecret.StringData["username"] != "" {
		if !reflect.DeepEqual(argoCDSecret.StringData["username"], dbRepositoryCredentials.AuthUsername) {
			l.Info("Updating Argo CD Private Repository secret", "username", dbRepositoryCredentials.AuthPassword)
			argoCDSecret.StringData["username"] = dbRepositoryCredentials.AuthPassword
			if err = eventClient.Update(ctx, argoCDSecret); err != nil {
				l.Error(err, errUpdatePrivateSecret, "username")
				return retry, err
			}
		}
	}

	if argoCDSecret.StringData["sshPrivateKey"] != "" {
		if !reflect.DeepEqual(argoCDSecret.StringData["sshPrivateKey"], dbRepositoryCredentials.AuthSSHKey) {
			l.Info("Updating Argo CD Private Repository secret", "sshPrivateKey", dbRepositoryCredentials.AuthPassword)
			argoCDSecret.StringData["sshPrivateKey"] = dbRepositoryCredentials.AuthPassword
			if err = eventClient.Update(ctx, argoCDSecret); err != nil {
				l.Error(err, errUpdatePrivateSecret, "SSH PrivateKey")
				return retry, err
			}
		}
	}

	return noRetry, nil
}

func repoCredToSecret(repoCred db.RepositoryCredentials, secret *corev1.Secret) {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	updateSecretString(secret, "name", repoCred.SecretObj)
	updateSecretString(secret, "url", repoCred.PrivateURL)
	updateSecretString(secret, "username", repoCred.AuthUsername)
	updateSecretString(secret, "password", repoCred.AuthPassword)
	updateSecretString(secret, "sshPrivateKey", repoCred.AuthSSHKey)
	addSecretMetadata(secret, common.LabelValueSecretTypeRepository)

	// Values Supported by ArgoCD but not yet part of GitOps Repository Credentials as part of the MVP
	// -----------------------------------------------------------------------------------------------
	//updateSecretString(secret, "project", "") not supported yet
	//updateSecretBool(secret, "enableOCI", repository.EnableOCI)
	//updateSecretString(secret, "tlsClientCertData", repository.TLSClientCertData)
	//updateSecretString(secret, "tlsClientCertKey", repository.TLSClientCertKey)
	//updateSecretString(secret, "type", repository.Type)
	//updateSecretString(secret, "githubAppPrivateKey", repository.GithubAppPrivateKey)
	//updateSecretInt(secret, "githubAppID", repository.GithubAppId)
	//updateSecretInt(secret, "githubAppInstallationID", repository.GithubAppInstallationId)
	//updateSecretString(secret, "githubAppEnterpriseBaseUrl", repository.GitHubAppEnterpriseBaseURL)
	//updateSecretBool(secret, "insecureIgnoreHostKey", repository.InsecureIgnoreHostKey)
	//updateSecretBool(secret, "insecure", repository.Insecure)
	//updateSecretBool(secret, "enableLfs", repository.EnableLFS)
	//updateSecretString(secret, "proxy", repository.Proxy)
}

func updateSecretString(secret *corev1.Secret, key, value string) {
	if _, present := secret.Data[key]; present || len(value) > 0 {
		secret.Data[key] = []byte(value)
	}
}

func addSecretMetadata(secret *corev1.Secret, secretType string) {
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[common.AnnotationKeyManagedBy] = common.AnnotationValueManagedByArgoCD

	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[common.LabelKeySecretType] = secretType
}
