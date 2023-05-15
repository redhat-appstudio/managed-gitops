package util

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
)

// GetControllerManager returns a manager for running controllers
func GetControllerManager(ctx context.Context, cfg *rest.Config, log *logr.Logger, apiExportName string, opts ctrl.Options) (ctrl.Manager, error) {
	scheme := runtime.NewScheme()

	apiExportClient, err := client.New(cfg, client.Options{Scheme: scheme})
	apiExportClient = IfEnabledSimulateUnreliableClient(apiExportClient)
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	return getControllerManager(ctx, cfg, apiExportClient, discoveryClient, log, apiExportName, opts)
}

// getControllerManager returns a standard controller-runtime manager.
func getControllerManager(ctx context.Context, restConfig *rest.Config, apiExportClient client.Client,
	discoveryClient discovery.DiscoveryInterface, log *logr.Logger, apiExportName string,
	opts ctrl.Options) (ctrl.Manager, error) {

	log.Info("creating standard manager")
	mgr, err := ctrl.NewManager(restConfig, opts)
	if err != nil {
		return nil, fmt.Errorf("unable to start manager: %w", err)
	}

	return mgr, nil
}
