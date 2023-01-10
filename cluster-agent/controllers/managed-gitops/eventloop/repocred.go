package eventloop

import (
	"context"
	"fmt"

	"github.com/argoproj/argo-cd/v2/common"
	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errOperationIDNotFound = "resource ID was nil while processing operation"
	errGenericDB           = "unable to retrieve database row from database"
	errRowNotFound         = "row no longer exists in the database"
	// #nosec G101
	errPrivateSecretNotFound = "Argo CD Private Repository secret doesn't exist"
	errPrivateSecretCreate   = "unable to create Argo CD Repository secret"
	errGetPrivateSecret      = "unexpected error on retrieve Argo CD secret"
	errUpdatePrivateSecret   = "unable to update Argo CD Private Repository secret"
	errDeletePrivateSecret   = "unable to delete Argo CD Private Repository secret"
	// #nosec G101
	errSevereLabelNotFound = "SEVERE: invalid label requirement"
	// #nosec G101
	errSecretLabelList        = "unable to complete Argo CD Secret list"
	errSevereNumOfItemsInList = "SEVERE: unexpected number (more than one) of related ArgoCD secrets"
)

// deleteArgoCDSecretLeftovers best effort attempt to clean up ArgoCD Secret leftovers.
func deleteArgoCDSecretLeftovers(ctx context.Context, databaseID string, argoCDNamespace corev1.Namespace, eventClient client.Client, l logr.Logger) (bool, error) {
	const retry, noRetry = true, false
	list := corev1.SecretList{}
	labelSelector := labels.NewSelector()
	req, err := labels.NewRequirement(controllers.RepoCredDatabaseIDLabel, selection.Equals, []string{databaseID})
	if err != nil {
		l.Error(err, errSevereLabelNotFound)
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
		l.Error(nil, errSevereNumOfItemsInList, "length", len(list.Items))
	}

	var firstDeletionErr error
	for idx := range list.Items {

		item := list.Items[idx]

		l.Info("Deleting Argo CD Secret (leftover) that is missing a DB Entry", "secret", item.Name, "namespace", item.Namespace)

		// Delete all Argo CD Secret with the corresponding database label (but, there should be only one)
		// #nosec G601
		err := eventClient.Delete(ctx, &item)
		sharedutil.LogAPIResourceChangeEvent(item.Namespace, item.Name, item, sharedutil.ResourceDeleted, l)
		if err != nil {
			if apierr.IsNotFound(err) {
				l.Info("Argo CD Secret (leftover) was already previously deleted", "secret", item.Name, "namespace", item.Namespace)
			} else {
				l.Error(err, errDeletePrivateSecret, "secret", item.Name, "namespace", item.Namespace)

				if firstDeletionErr == nil {
					firstDeletionErr = err
				}
			}
		} else {
			l.Info("Argo CD Secret (leftover) has been successfully deleted", "secret", item.Name, "namespace", item.Namespace)
		}

	}

	if firstDeletionErr != nil {
		l.Error(firstDeletionErr, "Deletion of at least one Argo CD Secret (leftover) failed. First error was: %v", firstDeletionErr)
		return retry, firstDeletionErr
	}

	return noRetry, nil
}

// processOperation_RepositoryCredentials processes the given operation as a RepositoryCredentials operation.
// It returns true if the operation should be retried, and false otherwise.
// It returns an error if there was an error processing the operation.
func processOperation_RepositoryCredentials(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation,
	opConfig operationConfig) (bool, error) {

	const retry, noRetry = true, false

	if dbOperation.Resource_id == "" {
		return retry, fmt.Errorf("%v: %v", errOperationIDNotFound, crOperation.Name)
	}

	l := opConfig.log.WithValues("operationRow", dbOperation.Operation_id)

	// 2) Retrieve the RepositoryCredentials database row that corresponds to the operation
	dbRepositoryCredentials, err := opConfig.dbQueries.GetRepositoryCredentialsByID(ctx, dbOperation.Resource_id)
	if err != nil {
		// If the db row is missing, try to delete the related leftovers (ArgoCD Secret)
		if db.IsResultNotFoundError(err) {
			l.Error(err, errRowNotFound, "resource-id", dbOperation.Resource_id)
			return deleteArgoCDSecretLeftovers(ctx, dbOperation.Resource_id, opConfig.argoCDNamespace, opConfig.eventClient, l)
		}

		// Something went wrong with the database connection, just retry
		l.Error(err, errGenericDB, "resource-id", dbOperation.Resource_id)
		return retry, err
	}

	l = l.WithValues("repositoryCredentialsRow", dbRepositoryCredentials.RepositoryCredentialsID)
	l.Info("Retrieved RepositoryCredentials DB row")

	// 3) Retrieve ArgoCD secret from the cluster.
	argoCDSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbRepositoryCredentials.SecretObj,
			Namespace: opConfig.argoCDNamespace.Name,
		},
	}

	l = l.WithValues("secret", argoCDSecret.Name, "namespace", argoCDSecret.Namespace)

	if err = opConfig.eventClient.Get(ctx, client.ObjectKeyFromObject(argoCDSecret), argoCDSecret); err != nil {
		if apierr.IsNotFound(err) {
			l.Info(errPrivateSecretNotFound)
			convertRepoCredToSecret(dbRepositoryCredentials, argoCDSecret)
			errCreateArgoCDSecret := opConfig.eventClient.Create(ctx, argoCDSecret, &client.CreateOptions{})
			if errCreateArgoCDSecret != nil {
				l.Error(errCreateArgoCDSecret, errPrivateSecretCreate)
				return retry, errCreateArgoCDSecret
			}
			sharedutil.LogAPIResourceChangeEvent(argoCDSecret.Namespace, argoCDSecret.Name, argoCDSecret, sharedutil.ResourceCreated, l)

			// The problem with the secret is now resolved, so we can proceed with the operation.
			l.Info("Argo CD Private Repository secret has been successfully created",
				"URL", string(argoCDSecret.Data["url"]),
				"username", string(argoCDSecret.Data["username"]),
				"SSH Key (length)", len(string(argoCDSecret.Data["ssh"])))
		} else {
			l.Error(err, errGetPrivateSecret)
			return retry, err
		}
	} else {
		l.Info("A corresponding Argo CD Private Repository secret has already been existing",
			"URL", string(argoCDSecret.Data["url"]),
			"username", string(argoCDSecret.Data["username"]),
			"SSH Key (length)", len(string(argoCDSecret.Data["ssh"])))
	}

	// 4. Check if the Argo CD secret has the correct name, and if not, update it with the name from the database.
	// https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#repositories

	decodedSecret := secretToRepoCred(argoCDSecret) // helpful function to decode []byte to string for easier comparison
	isUpdateNeeded := compareClusterResourceWithDatabaseRow(dbRepositoryCredentials, argoCDSecret, l, decodedSecret)

	if isUpdateNeeded {
		l.Info("Syncing with database...")
		if err = opConfig.eventClient.Update(ctx, argoCDSecret); err != nil {
			l.Error(err, errUpdatePrivateSecret)
			return retry, err
		}
		sharedutil.LogAPIResourceChangeEvent(argoCDSecret.Namespace, argoCDSecret.Name, argoCDSecret, sharedutil.ResourceModified, l)

	}

	return noRetry, nil
}

func compareClusterResourceWithDatabaseRow(dbRepositoryCredentials db.RepositoryCredentials, argoCDSecret *corev1.Secret, l logr.Logger, decodedSecret *db.RepositoryCredentials) bool {
	labelDatabaseIDPrivateRepoSecret := fmt.Sprintf("%s: %s", controllers.RepoCredDatabaseIDLabel, dbRepositoryCredentials.RepositoryCredentialsID)
	labelArgoCDPrivateRepoSecret := fmt.Sprintf("%s: %s", common.LabelKeySecretType, common.LabelValueSecretTypeRepository)
	annotationArgoCDPrivateRepoSecret := fmt.Sprintf("%s: %s", common.AnnotationKeyManagedBy, common.AnnotationValueManagedByArgoCD)
	var argoCDLabelFound, repoCredLabelFound, repoCredAnnotationFound bool

	if keyValue, isKeyExists := argoCDSecret.Labels[common.LabelKeySecretType]; isKeyExists && keyValue == common.LabelValueSecretTypeRepository {
		argoCDLabelFound = true
	}

	if keyValue, isKeyExists := argoCDSecret.Annotations[common.AnnotationKeyManagedBy]; isKeyExists && keyValue == common.AnnotationValueManagedByArgoCD {
		repoCredAnnotationFound = true
	}

	if keyValue, isKeyExists := argoCDSecret.Labels[controllers.RepoCredDatabaseIDLabel]; isKeyExists && keyValue == dbRepositoryCredentials.RepositoryCredentialsID {
		repoCredLabelFound = true
	}

	var isArgoCDLabelUpdateNeeded bool
	if !argoCDLabelFound {
		l.Info("Secret is missing ArgoCD label! Syncing with database...", "AddLabel", labelArgoCDPrivateRepoSecret)
		addSecretArgoCDMetadata(argoCDSecret, common.LabelValueSecretTypeRepository)
		isArgoCDLabelUpdateNeeded = true
	}

	var isRepoCredLabelUpdateNeeded bool
	if !repoCredLabelFound {
		l.Info("Secret is missing DatabaseID label! Syncing with database...", "AddLabel", labelDatabaseIDPrivateRepoSecret)
		addSecretRepoCredMetadata(argoCDSecret, dbRepositoryCredentials.RepositoryCredentialsID)
		isRepoCredLabelUpdateNeeded = true
	}

	var isRepoCredAnnotationUpdateNeeded bool
	if !repoCredAnnotationFound {
		l.Info("Secret is missing ArgoCD annotation! Syncing with database...", "AddAnnotation", annotationArgoCDPrivateRepoSecret)
		addSecretArgoCDAnnotation(argoCDSecret)
		isRepoCredAnnotationUpdateNeeded = true
	}

	var isSecretNameUpdateNeeded bool
	if decodedSecret.SecretObj != dbRepositoryCredentials.SecretObj {
		l.Info("Secret has wrong name! Syncing with database...", "UpdateFrom", decodedSecret.SecretObj, "UpdateTo", dbRepositoryCredentials.SecretObj)
		argoCDSecret.Data["name"] = []byte(dbRepositoryCredentials.SecretObj)
		isSecretNameUpdateNeeded = true
	}

	var isPrivateURLUpdateNeeded bool
	if decodedSecret.PrivateURL != dbRepositoryCredentials.PrivateURL {
		l.Info("Secret has wrong URL! Syncing with database...", "UpdateFrom", string(argoCDSecret.Data["url"]), "UpdateTo", dbRepositoryCredentials.PrivateURL)
		argoCDSecret.Data["url"] = []byte(dbRepositoryCredentials.PrivateURL)
		isPrivateURLUpdateNeeded = true
	}

	var isPasswordUpdateNeeded bool
	if decodedSecret.AuthPassword != dbRepositoryCredentials.AuthPassword {
		l.Info("Secret has wrong Password! Syncing with database...")
		argoCDSecret.Data["password"] = []byte(dbRepositoryCredentials.AuthPassword)
		isPasswordUpdateNeeded = true
	}

	var isUsernameUpdateNeeded bool
	if decodedSecret.AuthUsername != dbRepositoryCredentials.AuthUsername {
		l.Info("Secret has wrong Username! Syncing with database...", "UpdateFrom", decodedSecret.AuthUsername, "UpdateTo", dbRepositoryCredentials.AuthUsername)
		argoCDSecret.Data["username"] = []byte(dbRepositoryCredentials.AuthUsername)
		isUsernameUpdateNeeded = true
	}

	var isSSHKeyUpdateNeeded bool
	if decodedSecret.AuthSSHKey != dbRepositoryCredentials.AuthSSHKey {
		l.Info("Secret has wrong SSH key! Syncing with database...", "UpdateFrom (len)", len(decodedSecret.AuthSSHKey), "UpdateTo (len)", len(dbRepositoryCredentials.AuthSSHKey))
		argoCDSecret.Data["ssh"] = []byte(dbRepositoryCredentials.AuthSSHKey)
		isSSHKeyUpdateNeeded = true
	}

	// If any of the above steps have been performed, then we need to update the cluster secret resource.
	isUpdateNeeded := isArgoCDLabelUpdateNeeded || isRepoCredLabelUpdateNeeded || isRepoCredAnnotationUpdateNeeded ||
		isPrivateURLUpdateNeeded || isPasswordUpdateNeeded || isUsernameUpdateNeeded || isSSHKeyUpdateNeeded ||
		isSecretNameUpdateNeeded

	return isUpdateNeeded
}

func convertRepoCredToSecret(repoCred db.RepositoryCredentials, secret *corev1.Secret) {
	if secret.Data == nil {
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
		secret.Data[key] = []byte(value)
	}
}

func addSecretArgoCDAnnotation(secret *corev1.Secret) {
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[common.AnnotationKeyManagedBy] = common.AnnotationValueManagedByArgoCD
}

func addSecretArgoCDMetadata(secret *corev1.Secret, secretType string) {
	addSecretArgoCDAnnotation(secret) // Add the annotation if it is not already present

	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[common.LabelKeySecretType] = secretType
}

// addSecretRepoCredMetadata
func addSecretRepoCredMetadata(secret *corev1.Secret, secretType string) {
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[controllers.RepoCredDatabaseIDLabel] = secretType
}

// secretToRepoCred converts a Secret to a RepositoryCredentials
// This is the reverse of convertRepoCredToSecret and it's needed because by default the values of the secret are in bytes
// e.g. Secret.name: [116 101 115 116 45 102 97 107 101 45 115 101 99 114 101 116 45 111 98 106]
//
//	Secret password: [116 101 115 116 45 102 97 107 101 45 97 117 116 104 45 112 97 115 115 119 111 114 100]
//
// that is why we need this function. To typecast the bytes to string.
func secretToRepoCred(secret *corev1.Secret) (repoCred *db.RepositoryCredentials) {
	return &db.RepositoryCredentials{
		PrivateURL:   string(secret.Data["url"]),
		AuthUsername: string(secret.Data["username"]),
		AuthPassword: string(secret.Data["password"]),
		AuthSSHKey:   string(secret.Data["sshPrivateKey"]),
		SecretObj:    secret.Name,
	}
}
