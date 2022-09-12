package utils

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	argocdclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/argo"
	argoio "github.com/argoproj/argo-cd/v2/util/io"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/argoproj/gitops-engine/pkg/sync/common"
	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This contents of this file are loosely based on the 'argocd app sync' CLI command:
// https://github.com/argoproj/argo-cd/blob/0a46d37fc6af9fe0aa963bdd845e3d799aa0320d/cmd/argocd/commands/app.go#L1333

// AppSync will trigger a synchronize application on the given Argo CD appliatication, in the given namespace.
func AppSync(ctx context.Context, appName string, revision string, namespaceName string, k8sClient client.Client,
	credentialsService *CredentialService, skipTLSTest bool) error {

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespaceName,
			Name:      namespaceName,
		},
	}

	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace)
	if err != nil {
		return fmt.Errorf("unable to retrieve namespace in AppSync: %s, %v", namespaceName, err)
	}

	_, acdClient, err := credentialsService.GetArgoCDLoginCredentials(ctx, namespaceName, string(namespace.UID), false, k8sClient)
	if err != nil {
		return err
	}

	err = appSync(ctx, acdClient, appName, false, false, revision, false, "", false, false, 0, 0, 0, 0, 0)
	if err != nil {
		return err
	}

	return nil

}

func appSync(ctx context.Context, acdClient argocdclient.Client, appName string, dryRun bool, replace bool, revision string, prune bool,
	strategy string, force bool, async bool, timeout uint, retryLimit int64, retryBackoffDuration time.Duration,
	retryBackoffMaxDuration time.Duration, retryBackoffFactor int64) error {

	conn, appIf, err := acdClient.NewApplicationClient()
	if err != nil {
		return fmt.Errorf("unable to retrieve acd client: %v", err)
	}
	defer argoio.Close(conn)

	syncOptionsFactory := func() *applicationpkg.SyncOptions {
		syncOptions := applicationpkg.SyncOptions{}
		items := make([]string, 0)
		if replace {
			items = append(items, common.SyncOptionReplace)
		}

		if len(items) == 0 {
			// for prevent send even empty array if not need
			return nil
		}
		syncOptions.Items = items
		return &syncOptions
	}

	syncReq := applicationpkg.ApplicationSyncRequest{
		Name:        &appName,
		DryRun:      &dryRun,
		Revision:    &revision,
		Resources:   nil,
		Prune:       &prune,
		Manifests:   nil,
		Infos:       []*argoappv1.Info{},
		SyncOptions: syncOptionsFactory(),
	}

	switch strategy {
	case "apply":
		syncReq.Strategy = &argoappv1.SyncStrategy{Apply: &argoappv1.SyncStrategyApply{}}
		syncReq.Strategy.Apply.Force = force
	case "", "hook":
		syncReq.Strategy = &argoappv1.SyncStrategy{Hook: &argoappv1.SyncStrategyHook{}}
		syncReq.Strategy.Hook.Force = force
	default:
		return fmt.Errorf("unknown sync strategy: '%s'", strategy)
	}
	if retryLimit > 0 {
		syncReq.RetryStrategy = &argoappv1.RetryStrategy{
			Limit: retryLimit,
			Backoff: &argoappv1.Backoff{
				Duration:    retryBackoffDuration.String(),
				MaxDuration: retryBackoffMaxDuration.String(),
				Factor:      pointer.Int64Ptr(retryBackoffFactor),
			},
		}
	}
	_, err = appIf.Sync(ctx, &syncReq)
	if err != nil {
		return err
	}

	if !async {
		app, err := waitOnApplicationStatus(ctx, acdClient, appName, timeout, false, false, true, false, []argoappv1.SyncOperationResource{})
		if err != nil {
			return err
		}

		if !dryRun {
			if !app.Status.OperationState.Phase.Successful() {
				return fmt.Errorf("operation has completed with phase: %s", app.Status.OperationState.Phase)
			} else if /*len(selectedResources) == 0 &&*/ app.Status.Sync.Status != argoappv1.SyncStatusCodeSynced {
				// Only get resources to be pruned if sync was application-wide and final status is not synced
				pruningRequired := app.Status.OperationState.SyncResult.Resources.PruningRequired()
				if pruningRequired > 0 {
					return fmt.Errorf("%d resources require pruning", pruningRequired)
				}
			}
		}
	}

	return nil
}

// ResourceDiff tracks the state of a resource when waiting on an application status.
type resourceState struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
	Status    string
	Health    string
	Hook      string
	Message   string
}

// key returns a unique-ish key for the resource.
func (rs *resourceState) key() string {
	return fmt.Sprintf("%s/%s/%s/%s", rs.Group, rs.Kind, rs.Namespace, rs.Name)
}

// merge merges the new state with any different contents from another resourceState.
// Blank fields in the receiver state will be updated to non-blank.
// Non-blank fields in the receiver state will never be updated to blank.
// Returns whether or not any keys were updated.
func (rs *resourceState) merge(newState *resourceState) bool {
	updated := false
	for _, field := range []string{"Status", "Health", "Hook", "Message"} {
		v := reflect.ValueOf(rs).Elem().FieldByName(field)
		currVal := v.String()
		newVal := reflect.ValueOf(newState).Elem().FieldByName(field).String()
		if newVal != "" && currVal != newVal {
			v.SetString(newVal)
			updated = true
		}
	}
	return updated
}

func waitOnApplicationStatus(parentContext context.Context, acdClient argocdclient.Client, appName string, timeout uint, watchSync bool,
	watchHealth bool, watchOperation bool, watchSuspended bool,
	selectedResources []argoappv1.SyncOperationResource) (*argoappv1.Application, error) {

	ctx, cancel := context.WithCancel(parentContext)
	defer cancel()

	// refresh controls whether or not we refresh the app before printing the final status.
	// We only want to do this when an operation is in progress, since operations are the only
	// time when the sync status lags behind when an operation completes
	refresh := false

	printFinalStatus := func(app *argoappv1.Application) (*argoappv1.Application, error) {
		if refresh {
			var err error
			conn, appClient, err := acdClient.NewApplicationClient()
			if err != nil {
				return nil, err
			}

			refreshType := string(argoappv1.RefreshTypeNormal)
			app, err = appClient.Get(context.Background(), &applicationpkg.ApplicationQuery{Name: &appName, Refresh: &refreshType})
			if err != nil {
				return nil, err
			}
			_ = conn.Close()
		}

		return app, nil
	}

	if timeout != 0 {
		time.AfterFunc(time.Duration(timeout)*time.Second, func() {
			cancel()
		})
	}

	prevStates := make(map[string]*resourceState)
	conn, appClient, err := acdClient.NewApplicationClient()
	if err != nil {
		return nil, err
	}
	defer argoio.Close(conn)
	app, err := appClient.Get(ctx, &applicationpkg.ApplicationQuery{Name: &appName})
	if err != nil {
		return nil, err
	}

	appEventCh := acdClient.WatchApplicationWithRetry(ctx, appName, app.ResourceVersion)
	for appEvent := range appEventCh {
		app = &appEvent.Application

		operationInProgress := false
		// consider the operation is in progress
		if app.Operation != nil {
			// if it just got requested
			operationInProgress = true
			if !app.Operation.DryRun() {
				refresh = true
			}
		} else if app.Status.OperationState != nil {
			if app.Status.OperationState.FinishedAt == nil {
				// if it is not finished yet
				operationInProgress = true
			} else if !app.Status.OperationState.Operation.DryRun() && (app.Status.ReconciledAt == nil || app.Status.ReconciledAt.Before(app.Status.OperationState.FinishedAt)) {
				// if it is just finished and we need to wait for controller to reconcile app once after syncing
				operationInProgress = true
			}
		}

		var selectedResourcesAreReady bool

		// If selected resources are included, wait only on those resources, otherwise wait on the application as a whole.
		if len(selectedResources) > 0 {
			selectedResourcesAreReady = true
			for _, state := range getResourceStates(app, selectedResources) {
				resourceIsReady := checkResourceStatus(watchSync, watchHealth, watchOperation, watchSuspended, state.Health, state.Status, appEvent.Application.Operation)
				if !resourceIsReady {
					selectedResourcesAreReady = false
					break
				}
			}
		} else {
			// Wait on the application as a whole
			selectedResourcesAreReady = checkResourceStatus(watchSync, watchHealth, watchOperation, watchSuspended, string(app.Status.Health.Status), string(app.Status.Sync.Status), appEvent.Application.Operation)
		}

		if selectedResourcesAreReady && (!operationInProgress || !watchOperation) {
			app, err := printFinalStatus(app)
			if err != nil {
				return nil, err
			}
			return app, nil
		}

		newStates := groupResourceStates(app, selectedResources)
		for _, newState := range newStates {
			stateKey := newState.key()
			if prevState, found := prevStates[stateKey]; found {
				if watchHealth && prevState.Health != string(health.HealthStatusUnknown) && prevState.Health != string(health.HealthStatusDegraded) && newState.Health == string(health.HealthStatusDegraded) {
					return nil, fmt.Errorf("application '%s' health state has transitioned from %s to %s", appName, prevState.Health, newState.Health)
				}
			} else {
				prevStates[stateKey] = newState
			}
		}
	}
	return nil, fmt.Errorf("timed out (%ds) waiting for app %q match desired state", timeout, appName)
}

func groupResourceStates(app *argoappv1.Application, selectedResources []argoappv1.SyncOperationResource) map[string]*resourceState {
	resStates := make(map[string]*resourceState)
	for _, result := range getResourceStates(app, selectedResources) {
		key := result.key()
		if prev, ok := resStates[key]; ok {
			prev.merge(result)
		} else {
			resStates[key] = result
		}
	}
	return resStates
}

func checkResourceStatus(watchSync bool, watchHealth bool, watchOperation bool, watchSuspended bool, healthStatus string, syncStatus string, operationStatus *argoappv1.Operation) bool {
	healthCheckPassed := true
	if watchSuspended && watchHealth {
		healthCheckPassed = healthStatus == string(health.HealthStatusHealthy) ||
			healthStatus == string(health.HealthStatusSuspended)
	} else if watchSuspended {
		healthCheckPassed = healthStatus == string(health.HealthStatusSuspended)
	} else if watchHealth {
		healthCheckPassed = healthStatus == string(health.HealthStatusHealthy)
	}

	synced := !watchSync || syncStatus == string(argoappv1.SyncStatusCodeSynced)
	operational := !watchOperation || operationStatus == nil
	return synced && healthCheckPassed && operational
}

func getResourceStates(app *argoappv1.Application, selectedResources []argoappv1.SyncOperationResource) []*resourceState {
	var states []*resourceState
	resourceByKey := make(map[kube.ResourceKey]argoappv1.ResourceStatus)
	for i := range app.Status.Resources {
		res := app.Status.Resources[i]
		resourceByKey[kube.NewResourceKey(res.Group, res.Kind, res.Namespace, res.Name)] = res
	}

	// print most resources info along with most recent operation results
	if app.Status.OperationState != nil && app.Status.OperationState.SyncResult != nil {
		for _, res := range app.Status.OperationState.SyncResult.Resources {
			sync := string(res.HookPhase)
			health := string(res.Status)
			key := kube.NewResourceKey(res.Group, res.Kind, res.Namespace, res.Name)
			if resource, ok := resourceByKey[key]; ok && res.HookType == "" {
				health = ""
				if resource.Health != nil {
					health = string(resource.Health.Status)
				}
				sync = string(resource.Status)
			}
			states = append(states, &resourceState{
				Group: res.Group, Kind: res.Kind, Namespace: res.Namespace, Name: res.Name, Status: sync, Health: health, Hook: string(res.HookType), Message: res.Message})
			delete(resourceByKey, kube.NewResourceKey(res.Group, res.Kind, res.Namespace, res.Name))
		}
	}
	resKeys := make([]kube.ResourceKey, 0)
	for k := range resourceByKey {
		resKeys = append(resKeys, k)
	}
	sort.Slice(resKeys, func(i, j int) bool {
		return resKeys[i].String() < resKeys[j].String()
	})
	// print rest of resources which were not part of most recent operation
	for _, resKey := range resKeys {
		res := resourceByKey[resKey]
		health := ""
		if res.Health != nil {
			health = string(res.Health.Status)
		}
		states = append(states, &resourceState{
			Group: res.Group, Kind: res.Kind, Namespace: res.Namespace, Name: res.Name, Status: string(res.Status), Health: health, Hook: "", Message: ""})
	}
	// filter out not selected resources
	if len(selectedResources) > 0 {
		for i := len(states) - 1; i >= 0; i-- {
			res := states[i]
			if !argo.ContainsSyncResource(res.Name, res.Namespace, schema.GroupVersionKind{Group: res.Group, Kind: res.Kind}, selectedResources) {
				states = append(states[:i], states[i+1:]...)
			}
		}
	}
	return states
}
