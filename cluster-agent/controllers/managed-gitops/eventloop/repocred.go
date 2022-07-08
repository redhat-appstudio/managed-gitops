package eventloop

import (
	"context"
	"fmt"
	"github.com/argoproj/argo-cd/v2/common"
	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errOperationIDNotFound   = "resource ID was nil while processing operation"
	errGenericDB             = "unable to retrieve database row from database"
	errRowNotFound           = "row no longer exists in the database"
	errPrivateSecretNotFound = "Argo CD Private Repository secret doesn't exist"
	errPrivateSecretCreate   = "unable to create Argo CD Repository secret"
	errGetPrivateSecret      = "unexpected error on retrieve Argo CD secret"
	errUpdatePrivateSecret   = "unable to update Argo CD Private Repository secret"
	errDeletePrivateSecret   = "unable to delete Argo CD Private Repository secret"
	errLabelNotFound         = "SEVERE: invalid label requirement"
	errSecretLabelList       = "unable to complete Argo CD Secret list"
	errNumOfItemsInList      = "SEVERE: unexpected number of items in list"
)

// deleteArgoCDSecretLeftovers best effort attempt to clean up ArgoCD Secret leftovers.
func deleteArgoCDSecretLeftovers(ctx context.Context, argoCDNamespace corev1.Namespace, eventClient client.Client, l logr.Logger, databaseID string) (bool, error) {
	const retry, noRetry = true, false

	list := corev1.SecretList{}
	labelSelector := labels.NewSelector()
	req, err := labels.NewRequirement(controllers.RepoCredDatabaseIDLabel, selection.Equals, []string{databaseID})
	if err != nil {
		l.Error(err, errLabelNotFound)
		return noRetry, err
	}
	labelSelector = labelSelector.Add(*req)
	if err := eventClient.List(ctx, &list, &client.ListOptions{
		Namespace:     argoCDNamespace.Name,
		LabelSelector: labelSelector,
	}); err != nil {
		l.Error(err, errSecretLabelList)
		return retry, err
	}

	if len(list.Items) > 1 {
		// Sanity test: should really only ever be 0 or 1
		l.Error(nil, errNumOfItemsInList, "length", len(list.Items))
	}

	var firstDeletionErr error
	for _, item := range list.Items {

		l2 := l.WithValues("argoCDSecret", item.Name, "argoCDSecretNamespace", item.Namespace)

		l2.Info("Deleting Argo CD Secret that is missing a DB Entry")

		// Delete all Argo CD Secret with the corresponding database label (but, there should be only one)
		err := eventClient.Delete(ctx, &item)
		if err != nil {
			if apierr.IsNotFound(err) {
				l2.Info("Argo CD Secret was already deleted")
			} else {
				l2.Error(err, errDeletePrivateSecret, "Secret Name", item.Name)

				if firstDeletionErr == nil {
					firstDeletionErr = err
				}
			}
		} else {
			l2.Info("Deleted Argo CD Secret", "argoCDSecret", item.Name, "argoCDSecretNamespace", item.Namespace)
		}
	}

	if firstDeletionErr != nil {
		l.Error(firstDeletionErr, "Deletion of at least one Argo CD Secret failed. First error was: %v", firstDeletionErr)
		return retry, firstDeletionErr
	}

	return noRetry, nil
}

// processOperation_RepositoryCredentials processes the given operation as a RepositoryCredentials operation.
// It returns true if the operation should be retried, and false otherwise.
// It returns an error if there was an error processing the operation.
func processOperation_RepositoryCredentials(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation, dbQueries db.DatabaseQueries,
	argoCDNamespace corev1.Namespace, eventClient client.Client, l logr.Logger) (bool, error) {
	const retry, noRetry = true, false

	if dbOperation.Resource_id == "" {
		return retry, fmt.Errorf("%v: %v", errOperationIDNotFound, crOperation.Name)
	}

	l = l.WithValues("operationRow", dbOperation.Operation_id)

	// 2) Retrieve the RepositoryCredentials database row that corresponds to the operation
	dbRepositoryCredentials, err := dbQueries.GetRepositoryCredentialsByID(ctx, dbOperation.Resource_id)
	if err != nil {
		// If the db row is missing, try to delete the related leftovers (ArgoCD Secret)
		if db.IsResultNotFoundError(err) {
			l.Error(err, errRowNotFound, "resource-id", dbOperation.Resource_id)
			return deleteArgoCDSecretLeftovers(ctx, argoCDNamespace, eventClient, l, dbOperation.Resource_id)
		}

		// Something went wrong with the database connection, just retry
		l.Error(err, errGenericDB, "resource-id", dbOperation.Resource_id)
		return retry, err
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

	decodedSecret := secretToRepoCred(argoCDSecret)

	l.Info("Checking if the Name of the Argo CD Private Repository secret needs to be updated")
	if decodedSecret.SecretObj != dbRepositoryCredentials.SecretObj {
		l.Info("Updating Argo CD Private Repository secret name", "secret name", argoCDSecret.Name)
		argoCDSecret.Data["name"] = []byte(dbRepositoryCredentials.SecretObj)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "name")
			return retry, err
		}
	} else {
		l.Info("Re: No need the Name of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	}

	// Check if the secret has the correct Labels
	l.Info("Checking if the Label of the Argo CD Private Repository secret needs to be updated")
	secretLabels := getSecretLabels(argoCDSecret)
	var argoCDLabelFound, repoCredLabelFound = false, false
	for _, v := range secretLabels {
		if v == "argocd.argoproj.io/secret-type: repository" {
			argoCDLabelFound = true
		}
		if v == fmt.Sprintf("%s: %s", controllers.RepoCredDatabaseIDLabel, dbRepositoryCredentials.RepositoryCredentialsID) {
			repoCredLabelFound = true
		}
	}

	if argoCDLabelFound {
		l.Info("Re: No need the ArgoCD Label of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	} else {
		l.Info("Updating Argo CD Private Repository secret ArgoCD label", "secret name", argoCDSecret.Name)
		addSecretArgoCDMetadata(argoCDSecret, common.LabelValueSecretTypeRepository)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "label")
			return retry, err
		}
	}

	if repoCredLabelFound {
		l.Info("Re: No need the DatabaseID Label of the Argo CD Private Repository secret is the same with the respective RepositoryCredentials db entry")
	} else {
		l.Info("Updating Argo CD Private Repository secret DatabaseID label", "secret name", argoCDSecret.Name)
		addSecretRepoCredMetadata(argoCDSecret, dbRepositoryCredentials.RepositoryCredentialsID)
		if err = eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret, "label")
			return retry, err
		}
	}

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
		l.Info("Re: Yes, updating Argo CD Private Repository secret password", "From (Current)", string(argoCDSecret.Data["password"]), "to (Database)", dbRepositoryCredentials.AuthPassword)
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
		l.Info("Re: Yes, updating Argo CD Private Repository secret username", "From (Current)", string(argoCDSecret.Data["username"]), "to (Database)", dbRepositoryCredentials.AuthUsername)
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
		l.Info("Re: Yes, updating Argo CD Private Repository secret SSH key", "From (Current)", string(argoCDSecret.Data["ssh"]), "to (Database)", dbRepositoryCredentials.AuthSSHKey)
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

	updateSecretString(secret, "name", repoCred.SecretObj)
	updateSecretString(secret, "url", repoCred.PrivateURL)
	updateSecretString(secret, "username", repoCred.AuthUsername)
	updateSecretString(secret, "password", repoCred.AuthPassword)
	updateSecretString(secret, "sshPrivateKey", repoCred.AuthSSHKey)
	addSecretArgoCDMetadata(secret, common.LabelValueSecretTypeRepository) // adds the ArgoCD Label
	addSecretRepoCredMetadata(secret, repoCred.RepositoryCredentialsID)    // adds the DatabaseID Label

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

func addSecretArgoCDMetadata(secret *corev1.Secret, secretType string) {
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[common.AnnotationKeyManagedBy] = common.AnnotationValueManagedByArgoCD

	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[common.LabelKeySecretType] = secretType
}

// addSecretRepoCredMetadata adds the DatabaseID label to the ArgoCD secret, so we can find it later
func addSecretRepoCredMetadata(secret *corev1.Secret, secretType string) {
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[controllers.RepoCredDatabaseIDLabel] = secretType
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

func getSecretLabels(secret *corev1.Secret) []string {
	var s string
	var arr []string
	for key, val := range secret.Labels {
		s = fmt.Sprintf("%s=\"%s\"", key, val)
		arr = append(arr, s)
	}
	return arr
}
