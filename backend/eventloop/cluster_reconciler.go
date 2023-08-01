package eventloop

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Interval in Minutes to reconcile ClusterReconciler.
	orphanedResourcesCleanUpInterval = 30 * time.Minute
)

type ClusterReconciler struct {
	discoveryClient discovery.DiscoveryInterface

	client client.Client
}

func NewClusterReconciler(client client.Client, discoveryClient discovery.DiscoveryInterface) *ClusterReconciler {
	return &ClusterReconciler{
		discoveryClient: discoveryClient,
		client:          client,
	}
}

func (c *ClusterReconciler) Start() {
	go func() {
		<-time.NewTimer(orphanedResourcesCleanUpInterval).C

		ctx := context.Background()
		log := log.FromContext(ctx).
			WithName(logutil.LogLogger_managed_gitops).
			WithValues("component", "cluster-reconciler").
			WithValues(sharedutil.JobKey, sharedutil.JobKeyValue)

		_, _ = sharedutil.CatchPanic(func() error {
			c.cleanOrphanedResources(ctx, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		c.Start()
	}()
}

// A k8s resource is considered to be orphaned when:
// 1. It was previously managed by Argo CD i.e has label "app.kubernetes.io/instance".
// 2. It has a deletiontimestamp set.
// 3. It doesn't have a corresponding GitOpsDeployment resource.
func (c *ClusterReconciler) cleanOrphanedResources(ctx context.Context, log logr.Logger) {
	apiObjects, err := c.getAllNamespacedAPIResources(ctx, log)
	if err != nil {
		log.Error(err, "failed to get namespaced API resources from the cluster")
		return
	}

	for _, obj := range apiObjects {
		appName := getArgoCDApplicationName(&obj)
		if appName != "" && obj.GetDeletionTimestamp() != nil {
			// Check if a GitOpsDeployment exists with the UID specified in the Application name.
			gitopsDeplList := &managedgitopsv1alpha1.GitOpsDeploymentList{}
			err = c.client.List(ctx, gitopsDeplList, &client.ListOptions{
				Namespace: obj.GetNamespace(),
			})
			if err != nil {
				log.Error(err, "failed to list GitOpsDeployments", "namespace", obj.GetNamespace())
				continue
			}

			expectedUID := extractUIDFromApplicationName(appName)
			found := false
			for _, gitopsDepl := range gitopsDeplList.Items {
				if gitopsDepl.UID == types.UID(expectedUID) {
					found = true
					break
				}
			}

			// The resource has both deletiontimestamp and label "app.kubernetes.io/instance"
			// But it doesn't have a corresponding GitOpsDeployment so it can deleted.
			if !found {
				if err := c.client.Delete(ctx, &obj); err != nil {
					log.Error(err, "failed to delete object in the orphaned reconciler", "name", obj.GetName(), "namespace", obj.GetNamespace(), "gvk", obj.GroupVersionKind())
					continue
				}

				log.Info("Deleted an orphaned resoure that is not managed by Argo CD anymore", "Name", obj.GetName(), "Namespace", obj.GetNamespace())
			}
		}
	}
}

// getAllNamespacedAPIResources returns all namespace scoped resources from a Kubernetes cluster.
func (c *ClusterReconciler) getAllNamespacedAPIResources(ctx context.Context, log logr.Logger) ([]unstructured.Unstructured, error) {
	apiResourceList, err := c.discoveryClient.ServerPreferredNamespacedResources()
	if err != nil {
		return nil, err
	}

	expectedVerbs := []string{"list", "get", "delete"}
	resources := []unstructured.Unstructured{}
	for _, apiResources := range apiResourceList {
		for _, apiResource := range apiResources.APIResources {
			// Ignore resources that are either cluster scoped or do not support the required verbs
			if !apiResource.Namespaced || !isResourceAllowed(expectedVerbs, apiResource.Verbs) {
				continue
			}

			objList := &unstructured.UnstructuredList{}
			objList.SetAPIVersion(apiResources.GroupVersion)
			objList.SetKind(apiResource.Kind)

			err := c.client.List(ctx, objList)
			if err != nil {
				log.V(logutil.LogLevel_Debug).Error(err, "failed to list resources", "resource", apiResource.Kind)
				continue
			}

			if len(objList.Items) != 0 {
				resources = append(resources, objList.Items...)
			}
		}
	}

	return resources, nil
}

func isResourceAllowed(expectedVerbs []string, verbs []string) bool {
	verbMap := map[string]bool{}
	for _, verb := range verbs {
		verbMap[verb] = true
	}

	for _, expectedVerb := range expectedVerbs {
		if _, found := verbMap[expectedVerb]; !found {
			return false
		}
	}
	return true
}

func getArgoCDApplicationName(obj client.Object) string {
	labelKey := "app.kubernetes.io/instance"

	if value, found := obj.GetLabels()[labelKey]; found {
		if strings.HasPrefix(value, "gitopsdepl-") {
			return value
		}
	}

	return ""
}

func extractUIDFromApplicationName(name string) string {
	content := strings.Split(name, "-")
	if len(content) == 2 {
		return content[1]
	}
	return ""
}
