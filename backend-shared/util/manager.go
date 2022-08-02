package util

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/kcp"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"
)

// GetControllerManager returns the controller-runtime manager for creating controllers
func GetControllerManager(ctx context.Context, restConfig *rest.Config, log *logr.Logger, apiExportName string, opts ctrl.Options) (ctrl.Manager, error) {
	var mgr ctrl.Manager

	isKCPAPIsPresent, err := kcpAPIsGroupPresent(restConfig)
	if err != nil {
		return nil, err
	}

	if isKCPAPIsPresent {
		log.Info("Looking up virtual workspace URL")
		cfg, err := restConfigForAPIExport(ctx, restConfig, apiExportName)
		if err != nil {
			log.Error(err, "error looking up virtual workspace URL")
		}

		log.Info("Using virtual workspace URL", "url", cfg.Host)

		opts.LeaderElectionConfig = restConfig
		mgr, err = kcp.NewClusterAwareManager(cfg, opts)
		if err != nil {
			return nil, fmt.Errorf("unable to start cluster aware manager: %w", err)
		}
	} else {
		log.Info("The apis.kcp.dev group is not present - creating standard manager")
		mgr, err = ctrl.NewManager(restConfig, opts)
		if err != nil {
			return nil, fmt.Errorf("unable to start manager: %w", err)
		}
	}

	return mgr, nil
}

// restConfigForAPIExport returns a *rest.Config properly configured to communicate with the endpoint for the
// APIExport's virtual workspace.
func restConfigForAPIExport(ctx context.Context, cfg *rest.Config, apiExportName string) (*rest.Config, error) {
	scheme := runtime.NewScheme()
	if err := apisv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error adding apis.kcp.dev/v1alpha1 to scheme: %w", err)
	}

	apiExportClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	var apiExport apisv1alpha1.APIExport

	if apiExportName != "" {
		if err := apiExportClient.Get(ctx, types.NamespacedName{Name: apiExportName}, &apiExport); err != nil {
			return nil, fmt.Errorf("error getting APIExport %q: %w", apiExportName, err)
		}
	} else {
		exports := &apisv1alpha1.APIExportList{}
		if err := apiExportClient.List(ctx, exports); err != nil {
			return nil, fmt.Errorf("error listing APIExports: %w", err)
		}
		if len(exports.Items) == 0 {
			return nil, fmt.Errorf("no APIExport found")
		}
		if len(exports.Items) > 1 {
			return nil, fmt.Errorf("more than one APIExport found")
		}
		apiExport = exports.Items[0]
	}

	if len(apiExport.Status.VirtualWorkspaces) < 1 {
		return nil, fmt.Errorf("APIExport %q status.virtualWorkspaces is empty", apiExportName)
	}

	cfg = rest.CopyConfig(cfg)
	// TODO(ncdc): sharding support
	cfg.Host = apiExport.Status.VirtualWorkspaces[0].URL

	return cfg, nil
}

func kcpAPIsGroupPresent(restConfig *rest.Config) (bool, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return false, fmt.Errorf("failed to create discovery client: %w", err)
	}
	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return false, fmt.Errorf("failed to get server groups: %w", err)
	}

	for _, group := range apiGroupList.Groups {
		if group.Name == apisv1alpha1.SchemeGroupVersion.Group {
			for _, version := range group.Versions {
				if version.Version == apisv1alpha1.SchemeGroupVersion.Version {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
