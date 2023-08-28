package eventloop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argocdutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Interval in Hours to reconcile ClusterReconciler.
	orphanedResourcesCleanUpInterval = 2 * time.Hour
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

//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims;persistentvolumes;secrets;configmaps;pods;endpoints;services;serviceaccounts,verbs=get;list;delete
//+kubebuilder:rbac:groups="apps",resources=replicasets;statefulsets;daemonsets;deployments,verbs=get;list;delete
//+kubebuilder:rbac:groups="discovery.k8s.io",resources=endpointslices,verbs=get;list;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses;ingressclasses,verbs=get;list;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings;roles,verbs=get;list;delete
//+kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;delete
//+kubebuilder:rbac:groups="triggers.tekton.dev",resources=eventlisteners;triggertemplates,verbs=get;list;delete
//+kubebuilder:rbac:groups="pipelinesascode.tekton.dev",resources=repositories,verbs=get;list;delete

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
// 2. It doesn't have a corresponding GitOpsDeployment resource.
func (c *ClusterReconciler) cleanOrphanedResources(ctx context.Context, log logr.Logger) {
	// Use a label selector to filter resources managed by Argo CD
	labelSelector, err := labels.Parse(argocdutil.ArgocdResourceLabel)
	if err != nil {
		log.Error(err, "failed to create a label selector for listing resources managed by Argo CD")
		return
	}

	listOpts := &client.ListOptions{
		LabelSelector: labelSelector,
	}

	apiObjects, err := c.getAllNamespacedAPIResources(ctx, log, listOpts)
	if err != nil {
		log.Error(err, "failed to get namespaced API resources from the cluster")
		return
	}

	for i, obj := range apiObjects {

		// Skip all openshift namespces
		if strings.HasPrefix(obj.GetNamespace(), "openshift-") {
			continue
		}

		// Skip all non-tenant Namespaces
		if !strings.HasSuffix(obj.GetNamespace(), "-tenant") {
			continue
		}

		appName := argocdutil.GetArgoCDApplicationName(obj.GetLabels())

		// If the Application wasn't deployed by Argo CD and the GitOps Service, then skip it
		if appName == "" {
			continue
		}

		// Skip resources that are already being deleted
		if obj.GetDeletionTimestamp() != nil {
			continue
		}

		// Check if a GitOpsDeployment exists with the UID specified in the Application name.
		gitopsDeplList := &managedgitopsv1alpha1.GitOpsDeploymentList{}

		if err := c.client.List(ctx, gitopsDeplList, &client.ListOptions{
			Namespace: obj.GetNamespace(),
		}); err != nil {
			log.Error(err, "failed to list GitOpsDeployments", "namespace", obj.GetNamespace())
			continue
		}

		expectedUID := argocdutil.ExtractUIDFromApplicationName(appName)
		if expectedUID == "" {
			log.Error(nil, "unable to extract UID from application name in cleanOrphanedResources", "appName", appName)
			continue
		}

		found := false
		for _, gitopsDepl := range gitopsDeplList.Items {
			if string(gitopsDepl.UID) == expectedUID {
				found = true
				break
			}
		}

		// The resource was managed by Argo CD i.e. it has label "app.kubernetes.io/instance"
		// But it doesn't have a corresponding GitOpsDeployment so it can be deleted.
		if !found {
			if err := c.client.Delete(ctx, &apiObjects[i]); err != nil {
				log.Error(err, "failed to delete object in the orphaned reconciler", "name", obj.GetName(), "namespace", obj.GetNamespace(), "gvk", obj.GroupVersionKind())
				continue
			}

			log.Info("Deleted an orphaned resource that is not managed by Argo CD anymore", "Name", obj.GetName(), "Namespace", obj.GetNamespace(), "gvk", obj.GroupVersionKind())
		}
	}
}

// getAllNamespacedAPIResources returns all namespace scoped resources from a Kubernetes cluster.
func (c *ClusterReconciler) getAllNamespacedAPIResources(ctx context.Context, log logr.Logger, opts ...client.ListOption) ([]unstructured.Unstructured, error) {
	apiResourceList, err := c.discoveryClient.ServerPreferredNamespacedResources()
	if err != nil {
		return nil, err
	}

	expectedVerbs := []string{"list", "get", "delete"}
	resources := []unstructured.Unstructured{}

	for _, apiResources := range apiResourceList {
		for _, apiResource := range apiResources.APIResources {

			// Skip volumeclaims, which are known to be contain stateful data and thus may not be easily replaced if unexpectedly deleted
			if apiResource.Name == "persistentvolumeclaims" {
				continue
			}

			// Ignore resources that are either cluster scoped or do not support the required verbs
			if !apiResource.Namespaced || !isResourceAllowed(expectedVerbs, apiResource.Verbs) {
				continue
			}

			objList := &unstructured.UnstructuredList{}
			objList.SetAPIVersion(apiResources.GroupVersion)
			objList.SetKind(apiResource.Kind)

			if err := c.client.List(ctx, objList, opts...); err != nil {
				log.V(logutil.LogLevel_Debug).Info("failed to list resources", "resource", apiResource.Kind, "error", fmt.Sprintf("%v", err))
				continue
			}

			resources = append(resources, objList.Items...)

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
