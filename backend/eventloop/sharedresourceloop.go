package eventloop

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The goal of the shared resource event loop is to ensure that workspace-scoped resources are only
// created from a single thread, preventing concurrent goroutines from stepping on each others toes.
//
// This ensures that:
// - When multiple application goroutines are attempting to create workspace-scoped resources,
//   that no duplicates are created (eg multiple clusterusers for a single user, or multiple
//   managedenvs for a single workspace)
// - There are no race conditions on creation of workspace-scoped resources.
//
// Workspace scoped resources are:
// - managedenv
// - clusteraccess
// - clusteruser
// - gitopsengineinstance (for now)
type sharedResourceEventLoop struct {
	inputChannel chan sharedResourceLoopMessage
}

func (srEventLoop *sharedResourceEventLoop) getOrCreateClusterUserByNamespaceUID(ctx context.Context, workspaceClient client.Client, workspaceNamespace corev1.Namespace) (*db.ClusterUser, error) {

	responseChannel := make(chan interface{})

	msg := sharedResourceLoopMessage{
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUID,
		responseChannel:    responseChannel,
	}

	srEventLoop.inputChannel <- msg

	var rawResponse interface{}

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled in getOrCreateClusterUserByNamespaceUID")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDResponse)
	if !ok {
		return nil, fmt.Errorf("SEVERE: unexpected response type")
	}

	return response.clusterUser, response.err

}

func (srEventLoop *sharedResourceEventLoop) getGitopsEngineInstanceById(ctx context.Context, id string, workspaceClient client.Client, workspaceNamespace corev1.Namespace) (*db.GitopsEngineInstance, error) {

	responseChannel := make(chan interface{})

	msg := sharedResourceLoopMessage{
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_getGitopsEngineInstanceById,
		responseChannel:    responseChannel,
		payload: sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDRequest{
			gitopsEngineInstanceID: id,
		},
	}

	srEventLoop.inputChannel <- msg

	var rawResponse interface{}

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled in getGitOpsEngineInstanceById")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_getGitopsEngineInstanceByIdResponse)
	if !ok {
		return nil, fmt.Errorf("SEVERE: unexpected response type")
	}

	return response.gitopsEngineInstance, response.err

}

func (srEventLoop *sharedResourceEventLoop) getOrCreateSharedResources(ctx context.Context,
	workspaceClient client.Client, workspaceNamespace corev1.Namespace) (*db.ClusterUser,
	*db.ManagedEnvironment, *db.GitopsEngineInstance, *db.ClusterAccess, error) {

	responseChannel := make(chan interface{})

	msg := sharedResourceLoopMessage{
		workspaceClient:    workspaceClient,
		workspaceNamespace: workspaceNamespace,
		messageType:        sharedResourceLoopMessage_getOrCreateSharedResources,
		responseChannel:    responseChannel,
	}

	srEventLoop.inputChannel <- msg

	var rawResponse interface{}

	select {
	case rawResponse = <-responseChannel:
	case <-ctx.Done():
		return nil, nil, nil, nil, fmt.Errorf("context cancelled in getOrCreateSharedResources")
	}

	response, ok := rawResponse.(sharedResourceLoopMessage_getOrCreateSharedResourcesResponse)
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("SEVERE: unexpected response type")
	}

	return response.clusterUser, response.managedEnv, response.gitopsEngineInstance, response.clusterAccess, nil

}

func newSharedResourceLoop() *sharedResourceEventLoop {

	sharedResourceEventLoop := &sharedResourceEventLoop{
		inputChannel: make(chan sharedResourceLoopMessage),
	}

	go internalSharedResourceEventLoop(sharedResourceEventLoop.inputChannel)

	return sharedResourceEventLoop
}

type sharedResourceLoopMessageType string

const (
	sharedResourceLoopMessage_getOrCreateSharedResources           sharedResourceLoopMessageType = "getOrCreateSharedResources"
	sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUID sharedResourceLoopMessageType = "getOrCreateClusterUserByNamespaceUID"
	sharedResourceLoopMessage_getGitopsEngineInstanceById          sharedResourceLoopMessageType = "getGitopsEngineInstanceById"
)

type sharedResourceLoopMessage struct {
	workspaceClient    client.Client
	workspaceNamespace corev1.Namespace

	messageType     sharedResourceLoopMessageType
	responseChannel chan (interface{})

	// optional payload
	payload interface{}
}

type sharedResourceLoopMessage_getGitopsEngineInstanceByIdResponse struct {
	err                  error
	gitopsEngineInstance *db.GitopsEngineInstance
}

type sharedResourceLoopMessage_getOrCreateSharedResourcesResponse struct {
	err                  error
	clusterUser          *db.ClusterUser
	managedEnv           *db.ManagedEnvironment
	gitopsEngineInstance *db.GitopsEngineInstance
	clusterAccess        *db.ClusterAccess
}

type sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDRequest struct {
	gitopsEngineInstanceID string
}

type sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDResponse struct {
	err         error
	clusterUser *db.ClusterUser
}

func internalSharedResourceEventLoop(inputChan chan sharedResourceLoopMessage) {

	ctx := context.Background()
	log := log.FromContext(ctx)
	dbQueries, err := db.NewProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: internalSharedResourceEventLoop exiting before startup")
		return
	}

	for {
		msg := <-inputChan

		_, err = sharedutil.CatchPanic(func() error {
			processSharedResourceMessage(ctx, msg, dbQueries, log)
			return nil
		})
		if err != nil {
			log.Error(err, "unexpected error from processMessage in internalSharedResourceEventLoop")
		}
	}
}

func processSharedResourceMessage(ctx context.Context, msg sharedResourceLoopMessage, dbQueries db.DatabaseQueries, log logr.Logger) {
	log.V(sharedutil.LogLevel_Debug).Info("sharedResourceEventLoop received message: "+string(msg.messageType), "workspace", msg.workspaceNamespace.UID)

	if msg.messageType == sharedResourceLoopMessage_getOrCreateSharedResources {
		clusterUser, managedEnv, gitopsEngineInstance, clusterAccess, err := internalProcessMessage_GetOrCreateSharedResources(ctx, msg.workspaceClient, msg.workspaceNamespace, dbQueries, log)

		response := sharedResourceLoopMessage_getOrCreateSharedResourcesResponse{
			err:                  err,
			clusterUser:          clusterUser,
			managedEnv:           managedEnv,
			gitopsEngineInstance: gitopsEngineInstance,
			clusterAccess:        clusterAccess,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else if msg.messageType == sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUID {

		clusterUser, err := internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx, msg.workspaceNamespace, dbQueries)

		response := sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDResponse{
			err:         err,
			clusterUser: clusterUser,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else if msg.messageType == sharedResourceLoopMessage_getGitopsEngineInstanceById {

		var gitopsEngineInstance *db.GitopsEngineInstance
		var err error

		payload, ok := (msg.payload).(sharedResourceLoopMessage_getOrCreateClusterUserByNamespaceUIDRequest)
		if ok {
			gitopsEngineInstance, err = internalProcessMessage_GetGitopsEngineInstanceById(ctx, payload.gitopsEngineInstanceID, dbQueries)
		} else {
			err = fmt.Errorf("SEVERE - unexpected cast in internalSharedResourceEventLoop")
			log.Error(err, err.Error())
		}

		response := sharedResourceLoopMessage_getGitopsEngineInstanceByIdResponse{
			err:                  err,
			gitopsEngineInstance: gitopsEngineInstance,
		}

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		go func() {
			msg.responseChannel <- response
		}()

	} else {
		log.Error(nil, "SEVERE: unrecognized sharedResourceLoopMessageType: "+string(msg.messageType))
	}

}

func internalProcessMessage_GetGitopsEngineInstanceById(ctx context.Context, id string, dbq db.DatabaseQueries) (*db.GitopsEngineInstance, error) {

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: id,
	}

	err := dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)

	return &gitopsEngineInstance, err
}

func internalProcessMessage_GetOrCreateClusterUserByNamespaceUID(ctx context.Context, workspaceNamespace corev1.Namespace, dbq db.DatabaseQueries) (*db.ClusterUser, error) {

	// TODO: GITOPSRVCE-19 - KCP support: for now, we assume that the namespace UID that the request occurred in is the user id.
	clusterUser := db.ClusterUser{User_name: string(workspaceNamespace.UID)}

	// TODO: GITOPSRVCE-41 - We are assuming that user namespace uid == username, which is messy. We should add a new field for unique user id, and username should be human readable and not used for security, etc.
	err := dbq.GetClusterUserByUsername(ctx, &clusterUser)
	if err != nil {
		if db.IsResultNotFoundError(err) {

			if err := dbq.CreateClusterUser(ctx, &clusterUser); err != nil {
				return nil, err
			}

		} else {
			return nil, err
		}
	}

	return &clusterUser, nil
}

// Ensure the user's workspace is configured, ensure a GitOpsEngineInstance exists that will target it, and ensure
// a cluster access exists the give the user permission to target them from the engine.
func internalProcessMessage_GetOrCreateSharedResources(ctx context.Context, workspaceClient client.Client,
	workspaceNamespace corev1.Namespace, dbQueries db.DatabaseQueries,
	log logr.Logger) (*db.ClusterUser, *db.ManagedEnvironment, *db.GitopsEngineInstance, *db.ClusterAccess, error) {

	clusterUser, err := internalGetOrCreateClusterUserByNamespaceUID(ctx, string(workspaceNamespace.UID), dbQueries)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("unable to retrieve cluster user in processMessage, '%s': %v", string(workspaceNamespace.UID), err)
	}

	managedEnv, err := dbutil.GetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspaceNamespace, dbQueries, log)
	if err != nil {
		log.Error(err, "unable to get or created managed env on deployment modified event")
		return nil, nil, nil, nil, err
	}

	engineInstance, err := internalDetermineGitOpsEngineInstanceForNewApplication(ctx, *clusterUser, *managedEnv, workspaceClient, dbQueries, log)
	if err != nil {
		log.Error(err, "unable to determine gitops engine instance")
		return nil, nil, nil, nil, err
	}

	// Create the cluster access object, to allow us to interact with the GitOpsEngine and ManagedEnvironment on the user's behalf
	ca := db.ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnv.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: engineInstance.Gitopsengineinstance_id,
	}

	if err := internalGetOrCreateClusterAccess(ctx, &ca, dbQueries); err != nil {
		log.Error(err, "unable to create cluster access")
		return nil, nil, nil, nil, err
	}

	return clusterUser, managedEnv, engineInstance, &ca, nil

}

// Whenever a new Argo CD Application needs to be created, we need to find an Argo CD instance
// that is available to use it. In the future, when we have multiple instances, there would
// be an algorithm that intelligently places applications -> instances, to ensure that
// there are no Argo CD instances that are overloaded (have too many users).
//
// However, at the moment we are using a single shared Argo CD instnace, so we will
// just return that.
//
// This logic would be improved by https://issues.redhat.com/browse/GITOPSRVCE-73 (and others)
func internalDetermineGitOpsEngineInstanceForNewApplication(ctx context.Context, user db.ClusterUser, managedEnv db.ManagedEnvironment,
	k8sClient client.Client, dbq db.DatabaseQueries, log logr.Logger) (*db.GitopsEngineInstance, error) {

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: dbutil.GetGitOpsEngineSingleInstanceNamespace(), Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace()}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return nil, fmt.Errorf("unable to retrieve gitopsengine namespace in determineGitOpsEngineInstanceForNewApplication")
	}

	kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", Namespace: "kube-system"}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubeSystemNamespace), kubeSystemNamespace); err != nil {
		return nil, fmt.Errorf("unable to retrieve kube-system namespace in determineGitOpsEngineInstanceForNewApplication")
	}

	gitopsEngineInstance, _, err := dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, string(kubeSystemNamespace.UID), dbq, log)
	if err != nil {
		return nil, fmt.Errorf("unable to get or create engine instance for new application: %v", err)
	}

	// When we support multiple Argo CD instance, the algorithm would be:
	//
	// algorithm input:
	// - user
	// - managed environment
	//
	// output:
	// - gitops engine instance
	//
	// In a way that ensures that applications are balanced between instances.
	// Preliminary thoughts: https://docs.google.com/document/d/15E8d5frNuTFEdCHMlNSk0LQr6DI7BtiypxIC2AnW-OQ/edit#

	return gitopsEngineInstance, nil
}

func internalGetOrCreateClusterAccess(ctx context.Context, ca *db.ClusterAccess, dbq db.DatabaseQueries) error {

	if err := dbq.GetClusterAccessByPrimaryKey(ctx, ca); err != nil {

		if !db.IsResultNotFoundError(err) {
			return err
		}
	} else {
		return nil
	}

	if err := dbq.CreateClusterAccess(ctx, ca); err != nil {
		return err
	}

	return nil
}

func internalGetOrCreateClusterUserByNamespaceUID(ctx context.Context, namespaceUID string, dbq db.DatabaseQueries) (*db.ClusterUser, error) {

	// TODO: GITOPSRVCE-19 - KCP support: for now, we assume that the namespace UID that the request occurred in is the user id.
	clusterUser := db.ClusterUser{User_name: namespaceUID}

	err := dbq.GetClusterUserByUsername(ctx, &clusterUser)
	if err != nil {
		if db.IsResultNotFoundError(err) {

			if err := dbq.CreateClusterUser(ctx, &clusterUser); err != nil {
				return nil, err
			}

		} else {
			return nil, err
		}
	}

	return &clusterUser, nil
}
