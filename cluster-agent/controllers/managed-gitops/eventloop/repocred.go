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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errOperationIDNotFound   = "resource ID was nil while processing operation: \"%s\""
	errGenericDB             = "unable to retrieve database row from database"
	errRowNotFound           = "row no longer exists in the database"
	errPrivateSecretNotFound = "Argo CD Private Repository secret doesn't exist:"
	errPrivateSecretCreate   = "Unable to create Argo CD Repository secret:"
	errGetPrivateSecret      = "unexpected error on retrieve Argo CD secret:"
	errUpdatePrivateSecret   = "unable to update Argo CD Private Repository secret's \"%s\""
)

// processOperation_RepositoryCredentials processes the given operation as a RepositoryCredentials operation.
// It returns true if the operation should be retried, and false otherwise.
// It returns an error if there was an error processing the operation.
func processOperation_RepositoryCredentials(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation, dbQueries db.DatabaseQueries,
	argoCDNamespace corev1.Namespace, eventClient client.Client, l logr.Logger) (bool, error) {
	const retry, noRetry = true, false

	if dbOperation.Resource_id == "" {
		return retry, fmt.Errorf(errOperationIDNotFound, crOperation.Name)
	}

	l = l.WithValues("operationRow", dbOperation.Operation_id)

	// 2) Retrieve the RepositoryCredentials CR from the database.
	dbRepositoryCredentials, err := dbQueries.GetRepositoryCredentialsByID(ctx, dbOperation.Resource_id)
	if err != nil {
		if db.IsResultNotFoundError(err) {
			l.Error(err, errRowNotFound, "resource-id", dbOperation.Resource_id)
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
			l.Error(err, errGenericDB, "resource-id", dbOperation.Resource_id)
			return retry, err
		}
	}

	l = l.WithValues("repositoryCredentialsRow", dbRepositoryCredentials.RepositoryCredentialsID)
	l.Info("Retrieved RepositoryCredentials CR from database")
	l.Info(fmt.Sprintf("%v", dbRepositoryCredentials))

	// 3) Retrieve ArgoCD secret from the cluster.
	argoCDSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbRepositoryCredentials.SecretObj,
			Namespace: argoCDNamespace.Name,
		},
	}

	l = l.WithValues("ArgoCD Repository Secret", argoCDSecret.Name)
	l.Info("Retrieving ArgoCD Repository Secret from cluster")

	if err = eventClient.Get(ctx, client.ObjectKeyFromObject(argoCDSecret), argoCDSecret); err != nil {
		if apierr.IsNotFound(err) {
			l.Info(errPrivateSecretNotFound, "Secret Name:", argoCDSecret.Name)
			l.Info("Creating Argo CD Private Repository secret", "Secret Name:", argoCDSecret.Name)

			repoCredToSecret(dbRepositoryCredentials, argoCDSecret)

			errCreateArgoCDSecret := eventClient.Create(ctx, argoCDSecret, &client.CreateOptions{})
			if errCreateArgoCDSecret != nil {
				l.Error(errCreateArgoCDSecret, errPrivateSecretCreate, "Secret name", argoCDSecret.Name)
				return retry, errCreateArgoCDSecret
			}

			// The problem with the secret is now resolved, so we can proceed with the operation.
			l.Info("Created Argo CD Private Repository secret successfully", "Secret name", argoCDSecret.Name)
			l.Info(fmt.Sprintf("%v", argoCDSecret))

		} else {
			l.Error(err, errGetPrivateSecret, "Secret Name:", argoCDSecret.Name)
			return retry, err
		}
	}

	l.Info("Retrieved ArgoCD Repository Secret from cluster. Checking if it needs to be updated")
	// 4. Check if the Argo CD secret has the correct data, and if not, update it with the data from the database.
	// https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories

	l.Info(fmt.Sprintf("Labels: %s", argoCDSecret.Labels))
	l.Info(fmt.Sprintf("Annotations: %s", argoCDSecret.Annotations))

	decodedSecret := secretToRepoCred(argoCDSecret)
	fmt.Println("Name:", decodedSecret.SecretObj)
	fmt.Println("URL", decodedSecret.PrivateURL)
	fmt.Println("Username", decodedSecret.AuthUsername)
	fmt.Println("Password", decodedSecret.AuthPassword)
	fmt.Println("SSH", decodedSecret.AuthSSHKey)

	l.Info("Checking if the Name of the Argo CD Private Repository secret needs to be updated")
	if decodedSecret.SecretObj != dbRepositoryCredentials.SecretObj {
		l.Info("Updating Argo CD Private Repository secret", "secret name", argoCDSecret.Name)
		argoCDSecret.Name = dbRepositoryCredentials.SecretObj
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "name")
			return retry, err
		}
	} else {
		l.Info("Re: No need the Name of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	}

	//argoCDSecret.Data["url"] = []byte("https://www.google.com")
	//_ = eventClient.Update(ctx, argoCDSecret)
	//decodedSecret = secretToRepoCred(argoCDSecret)

	l = l.WithValues("ArgoCD Repository Secret", argoCDSecret.Name)
	l.Info("Checking if the URL of the Argo CD Private Repository secret needs to be updated")
	if decodedSecret.PrivateURL != dbRepositoryCredentials.PrivateURL {
		l.Info("Re: Yes, updating Argo CD Private Repository secret URL", "From (Current)", string(argoCDSecret.Data["url"]), "to (Database)", dbRepositoryCredentials.PrivateURL)
		argoCDSecret.Data["url"] = []byte(dbRepositoryCredentials.PrivateURL)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "url")
			return retry, err
		}
	} else {
		l.Info("Re: No need the URL of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	}

	l.Info("Checking if the password of the Argo CD Private Repository secret needs to be updated")
	if decodedSecret.AuthPassword != dbRepositoryCredentials.AuthPassword {
		l.Info("Updating Argo CD Private Repository secret password", "secret name", argoCDSecret.Name)
		argoCDSecret.Data["password"] = []byte(dbRepositoryCredentials.AuthPassword)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "password")
			return retry, err
		}
	} else {
		l.Info("Re: No need the password of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	}

	l.Info("Checking if the username of the Argo CD Private Repository secret needs to be updated")
	if decodedSecret.AuthUsername != dbRepositoryCredentials.AuthUsername {
		l.Info("Updating Argo CD Private Repository secret", "secret name", argoCDSecret.Name)
		argoCDSecret.Data["username"] = []byte(dbRepositoryCredentials.AuthUsername)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "username")
			return retry, err
		}
	} else {
		l.Info("Re: No need the username of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	}

	l.Info("Checking if the SSH key of the Argo CD Private Repository secret needs to be updated")
	if decodedSecret.AuthSSHKey != dbRepositoryCredentials.AuthSSHKey {
		l.Info("Updating Argo CD Private Repository secret", "secret name", argoCDSecret.Name)
		argoCDSecret.Data["ssh"] = []byte(dbRepositoryCredentials.AuthSSHKey)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "ssh")
			return retry, err
		}
	} else {
		l.Info("Re: No need the SSH key of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	}

	return noRetry, nil
}

func repoCredToSecret(repoCred db.RepositoryCredentials, secret *corev1.Secret) {
	if secret.Data == nil {
		fmt.Printf("\n\nSecret is nil. Creating a map.\n\n")
		secret.Data = make(map[string][]byte)
	}

	fmt.Println("This is the repoCred", repoCred)
	fmt.Println("RepoCred name", repoCred.SecretObj)
	fmt.Println("RepoCred URL", repoCred.PrivateURL)
	fmt.Println("RepoCred username", repoCred.AuthUsername)
	fmt.Println("RepoCred password", repoCred.AuthPassword)
	fmt.Println("RepoCred SSH PrivateKey", repoCred.AuthSSHKey)
	fmt.Println("Common Label:", common.LabelValueSecretTypeRepository)

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
	if _, present := secret.Data[key]; present || value != "" {
		fmt.Printf("\n\nUpdating Secret's %s\n\n", key)
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

// secretToRepoCred converts a Secret to a RepositoryCredentials
// This is the reverse of repoCredToSecret and it's needed because by default the values of the secret are in bytes
// e.g. Secret.name: [116 101 115 116 45 102 97 107 101 45 115 101 99 114 101 116 45 111 98 106]
//		Secret password: [116 101 115 116 45 102 97 107 101 45 97 117 116 104 45 112 97 115 115 119 111 114 100]
// that is why we need this function. To typecast the bytes to string.
func secretToRepoCred(secret *corev1.Secret) (repoCred *db.RepositoryCredentials) {
	return &db.RepositoryCredentials{
		PrivateURL:   string(secret.Data["url"]),
		AuthUsername: string(secret.Data["username"]),
		AuthPassword: string(secret.Data["password"]),
		AuthSSHKey:   string(secret.Data["sshPrivateKey"]),
		SecretObj:    string(secret.Data["name"]),
	}
}
