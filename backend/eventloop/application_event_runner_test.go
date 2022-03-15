package eventloop

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1/mocks"
	condition "github.com/redhat-appstudio/managed-gitops/backend/condition/mocks"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	testStructs "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1/mocks/structs"
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
			Name:      dbutil.GetGitOpsEngineSingleInstanceNamespace(),
			UID:       uuid.NewUUID(),
			Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
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

func TestApplicationEventLoopRunner_handleDeploymentModified(t *testing.T) {

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

	a := applicationEventLoopRunner_Action{
		// When the code asks for a new k8s client, give it our fake client
		getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
			return k8sClient, nil
		},
		eventResourceName:           gitopsDepl.Name,
		eventResourceNamespace:      gitopsDepl.Namespace,
		workspaceClient:             k8sClient,
		log:                         log.FromContext(context.Background()),
		sharedResourceEventLoop:     newSharedResourceLoop(),
		workspaceID:                 workspaceID,
		testOnlySkipCreateOperation: true,
	}

	// ------

	_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
	assert.Nil(t, err)

	// Verify that the database entries have been created -----------------------------------------

	var deplToAppMapping db.DeploymentToApplicationMapping
	{
		var appMappings []db.DeploymentToApplicationMapping

		err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
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

	if err := dbQueries.CheckedGetApplicationById(context.Background(), &application, clusterUser.Clusteruser_id); err != nil {
		assert.Nil(t, err)
		return
	}

	gitopsEngineInstance := db.GitopsEngineInstance{
		Gitopsengineinstance_id: application.Engine_instance_inst_id,
	}

	err = dbQueries.CheckedGetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance, clusterUser.Clusteruser_id)
	if !assert.Nil(t, err) {
		return
	}
	assert.Nil(t, err)

	managedEnvironment := db.ManagedEnvironment{
		Managedenvironment_id: application.Managed_environment_id,
	}
	err = dbQueries.CheckedGetManagedEnvironmentById(ctx, &managedEnvironment, clusterUser.Clusteruser_id)
	assert.Nil(t, err)

	// Delete the GitOpsDepl and verify that the corresponding DB entries are removed -------------

	err = k8sClient.Delete(ctx, gitopsDepl)
	if !assert.Nil(t, err) {
		return
	}

	_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
	assert.Nil(t, err)

	// Application should no longer exist
	err = dbQueries.GetApplicationById(ctx, &application)
	assert.NotNil(t, err)
	assert.True(t, db.IsResultNotFoundError(err))

	// DeploymentToApplicationMapping should be removed, too
	var appMappings []db.DeploymentToApplicationMapping
	err = dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(context.Background(), gitopsDepl.Name, gitopsDepl.Namespace, workspaceID, &appMappings)
	assert.Nil(t, err)
	assert.True(t, len(appMappings) == 0)

	// GitopsEngine instance should still be reachable
	err = dbQueries.CheckedGetGitopsEngineInstanceById(context.Background(), &gitopsEngineInstance, clusterUser.Clusteruser_id)
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

func TestApplicationEventLoopRunner_handleSyncRunModified(t *testing.T) {
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

	a := applicationEventLoopRunner_Action{
		// When the code asks for a new k8s client, give it our fake client
		getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
			return k8sClient, nil
		},
		eventResourceName:           gitopsDepl.Name,
		eventResourceNamespace:      gitopsDepl.Namespace,
		workspaceClient:             k8sClient,
		log:                         log.FromContext(context.Background()),
		sharedResourceEventLoop:     sharedResourceLoop,
		workspaceID:                 string(workspace.UID),
		testOnlySkipCreateOperation: true,
	}

	_, _, _, err = a.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
	assert.Nil(t, err)

	a = applicationEventLoopRunner_Action{
		// When the code asks for a new k8s client, give it our fake client
		getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
			return k8sClient, nil
		},
		eventResourceName:       gitopsDeplSyncRun.Name,
		eventResourceNamespace:  gitopsDeplSyncRun.Namespace,
		workspaceClient:         k8sClient,
		log:                     log.FromContext(context.Background()),
		sharedResourceEventLoop: sharedResourceLoop,
		workspaceID:             a.workspaceID,
	}

	_, err = a.applicationEventRunner_handleSyncRunModified(ctx, dbQueries)
	assert.Nil(t, err)

}

var _ = Describe("GitOpsDeployment Conditions", func() {
	var (
		adapter          *gitOpsDeploymentAdapter
		mockCtrl         *gomock.Controller
		mockClient       *mocks.MockClient
		mockStatusWriter *mocks.MockStatusWriter
		gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment
		mockConditions   *condition.MockConditions
		ctx              context.Context
	)

	BeforeEach(func() {
		gitopsDeployment = testStructs.NewGitOpsDeploymentBuilder().Initialized().GetGitopsDeployment()
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockConditions = condition.NewMockConditions(mockCtrl)
		mockStatusWriter = mocks.NewMockStatusWriter(mockCtrl)
	})
	JustBeforeEach(func() {
		adapter = newGitOpsDeploymentAdapter(gitopsDeployment, log.Log.WithName("Test Logger"), mockClient, mockConditions, ctx)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("setGitopsDeploymentCondition()", func() {
		var (
			err           = errors.New("fake reconcile")
			reason        = managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileError")
			conditionType = managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred
		)
		Context("when no conditions defined before and the err is nil", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(false)
			})
			It("It returns nil ", func() {
				errTemp := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(errTemp).To(BeNil())
			})
		})
		Context("when the err comes from reconcileHandler", func() {
			It("should update the CR", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(gomock.Any(), conditionType, managedgitopsv1alpha1.GitOpsConditionStatusTrue, reason, err.Error()).Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, err)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Context("when the err has been resolved", func() {
			BeforeEach(func() {
				mockConditions.EXPECT().HasCondition(gomock.Any(), conditionType).Return(true)
				mockConditions.EXPECT().FindCondition(gomock.Any(), conditionType).Return(&managedgitopsv1alpha1.GitOpsDeploymentCondition{}, true)
			})
			It("It should update the CR condition status as resolved", func() {
				matcher := testStructs.NewGitopsDeploymentMatcher()
				conditions := &gitopsDeployment.Status.Conditions
				*conditions = append(*conditions, managedgitopsv1alpha1.GitOpsDeploymentCondition{})
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().Update(gomock.Any(), matcher, gomock.Any())
				mockConditions.EXPECT().SetCondition(conditions, conditionType, managedgitopsv1alpha1.GitOpsConditionStatusFalse, managedgitopsv1alpha1.GitOpsDeploymentReasonType("ReconcileErrorResolved"), "").Times(1)
				err := adapter.setGitOpsDeploymentCondition(conditionType, reason, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
