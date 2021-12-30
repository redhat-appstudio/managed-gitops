package eventloop

import (
	"context"
	"fmt"
	"testing"

	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func genericTestSetup(t *testing.T) (*runtime.Scheme, *v1.Namespace, *v1.Namespace, *v1.Namespace) {
	scheme := runtime.NewScheme()

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	err := managedgitopsv1alpha1.AddToScheme(scheme)
	assert.Nil(t, err)
	err = operation.AddToScheme(scheme)
	assert.Nil(t, err)
	err = v1.AddToScheme(scheme)
	assert.Nil(t, err)

	argocdNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sharedutil.GitOpsEngineSingleInstanceNamespace,
			UID:       uuid.NewUUID(),
			Namespace: sharedutil.GitOpsEngineSingleInstanceNamespace,
		},
		Spec: v1.NamespaceSpec{},
	}

	kubesystemNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-system",
			UID:       uuid.NewUUID(),
			Namespace: "kube-system",
		},
		Spec: v1.NamespaceSpec{},
	}

	workspace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-user",
			UID:       uuid.NewUUID(),
			Namespace: "my-user",
		},
		Spec: v1.NamespaceSpec{},
	}

	return scheme, argocdNamespace, kubesystemNamespace, workspace

}

func TestWorkspaceEventLoopRunner_handleDeploymentModified(t *testing.T) {

	ctx := context.Background()

	scheme, argocdNamespace, kubesystemNamespace, workspace := genericTestSetup(t)

	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gitops-depl",
			Namespace: workspace.Name,
			UID:       uuid.NewUUID(),
		},
	}

	informer := sharedutil.ListEventReceiver{}

	k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
	k8sClient := &sharedutil.ProxyClient{
		InnerClient: k8sClientOuter,
		Informer:    &informer,
	}

	workspaceID := string(workspace.UID)

	dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
	assert.Nil(t, err)

	a := workspaceEventLoopRunner_Action{
		// When the code asks for a new k8s client, give it our fake client
		getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
			return k8sClient, nil
		},
		eventResourceName:       gitopsDepl.Name,
		eventResourceNamespace:  gitopsDepl.Namespace,
		workspaceClient:         k8sClient,
		log:                     log.FromContext(context.Background()),
		sharedResourceEventLoop: newSharedResourceLoop(),
	}

	// ------

	_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
	assert.Nil(t, err)

	// Verify that the database entries have been created -----------------------------------------

	var deplToAppMapping db.DeploymentToApplicationMapping
	{
		var appMappings []db.DeploymentToApplicationMapping

		err = dbQueries.UncheckedListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
		assert.Nil(t, err)

		if !assert.True(t, len(appMappings) == 1) {
			return
		}

		deplToAppMapping = appMappings[0]
		fmt.Println(deplToAppMapping)
	}

	clusterUser := db.ClusterUser{
		User_name: string(workspace.UID),
	}
	err = dbQueries.GetClusterUserByUsername(context.Background(), &clusterUser)
	if !assert.Nil(t, err) {
		return
	}

	application := db.Application{
		Application_id: deplToAppMapping.Application_id,
	}

	if err := dbQueries.GetApplicationById(context.Background(), &application, clusterUser.Clusteruser_id); err != nil {
		assert.Nil(t, err)
		return
	}

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: application.Engine_instance_inst_id,
	}

	err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance, clusterUser.Clusteruser_id)
	if !assert.Nil(t, err) {
		return
	}
	assert.Nil(t, err)

	managedEnvironment := db.ManagedEnvironment{
		Managedenvironment_id: application.Managed_environment_id,
	}
	err = dbQueries.GetManagedEnvironmentById(ctx, &managedEnvironment, clusterUser.Clusteruser_id)
	assert.Nil(t, err)

	// Delete the GitOpsDepl and verify that the corresponding DB entries are removed -------------

	err = k8sClient.Delete(ctx, gitopsDepl)
	if !assert.Nil(t, err) {
		return
	}

	_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
	assert.Nil(t, err)

	// Application should no longer exist
	err = dbQueries.UncheckedGetApplicationById(ctx, &application)
	assert.NotNil(t, err)
	assert.True(t, db.IsResultNotFoundError(err))

	// DeploymentToApplicationMapping should be removed, too
	var appMappings []db.DeploymentToApplicationMapping
	err = dbQueries.UncheckedListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
	assert.Nil(t, err)
	assert.True(t, len(appMappings) == 0)

	// GitopsEngine instance should still be reachable
	err = dbQueries.GetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance, clusterUser.Clusteruser_id)
	if !assert.Nil(t, err) {
		return
	}
	assert.Nil(t, err)

	operatorCreated := false
	operatorDeleted := false

	for idx, event := range informer.Events {

		if event.Action == sharedutil.Create && event.ObjectTypeOf() == "Operation" {
			operatorCreated = true
		}
		if event.Action == sharedutil.Delete && event.ObjectTypeOf() == "Operation" {
			operatorDeleted = true
		}

		fmt.Printf("%d) %v\n", idx, event)
	}

	assert.True(t, operatorCreated)
	assert.True(t, operatorDeleted)

}

func TestWorkspaceEventLoopRunner_handleSyncRunModified(t *testing.T) {
	ctx := context.Background()

	scheme, argocdNamespace, kubesystemNamespace, workspace := genericTestSetup(t)

	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gitops-depl",
			Namespace: workspace.Name,
			UID:       uuid.NewUUID(),
		},
	}

	gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gitops-depl-sync",
			Namespace: workspace.Name,
			UID:       uuid.NewUUID(),
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
			GitopsDeploymentName: gitopsDepl.Name,
			RevisionID:           "HEAD",
		},
	}

	informer := sharedutil.ListEventReceiver{}

	k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, gitopsDeplSyncRun, workspace, argocdNamespace, kubesystemNamespace).Build()
	k8sClient := &sharedutil.ProxyClient{
		InnerClient: k8sClientOuter,
		Informer:    &informer,
	}

	dbQueries, err := db.NewUnsafePostgresDBQueries(true, false)
	assert.Nil(t, err)

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	sharedResourceLoop := newSharedResourceLoop()

	a := workspaceEventLoopRunner_Action{
		// When the code asks for a new k8s client, give it our fake client
		getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
			return k8sClient, nil
		},
		eventResourceName:       gitopsDepl.Name,
		eventResourceNamespace:  gitopsDepl.Namespace,
		workspaceClient:         k8sClient,
		log:                     log.FromContext(context.Background()),
		sharedResourceEventLoop: sharedResourceLoop,
	}

	_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
	assert.Nil(t, err)

	a = workspaceEventLoopRunner_Action{
		// When the code asks for a new k8s client, give it our fake client
		getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
			return k8sClient, nil
		},
		eventResourceName:       gitopsDeplSyncRun.Name,
		eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
		workspaceClient:         k8sClient,
		log:                     log.FromContext(context.Background()),
		sharedResourceEventLoop: sharedResourceLoop,
	}

	_, err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
	assert.Nil(t, err)

}
