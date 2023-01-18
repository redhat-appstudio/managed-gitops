package metrics

import (
	"fmt"
	"sync"

	metric "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Gitopsdepl = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "active_gitopsDeployments",
			Help:        "Total number of active GitopsDeployments",
			ConstLabels: map[string]string{"gitopsDeployment": "success"},
		},
	)
	GitopsdeplFailures = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "gitopsDeployments_failures",
			Help:        "Total number of GitOpsDeployments with an error status",
			ConstLabels: map[string]string{"gitopsDeployment": "fail"},
		},
	)

	activeGitOpsDeployments = activeGitOpsDeploymentSet{
		mutex:             sync.Mutex{},
		gitOpsDeployments: map[string]bool{},
	}
)

type activeGitOpsDeploymentSet struct {
	mutex sync.Mutex

	// gitOpsDeployments contains a set of GitOpsDeployments which are currently known to exist.
	// NOTE: Before reading/writing from this list, acquire the mutex.
	// - key: string: (resource name)-(resource namespace)-(resource namespace uid)
	// - value: whether the GitOpsDeployment is in an error state; true if in error state, otherwise false.
	gitOpsDeployments map[string]bool
}

const (
	// maxTrackedDeployments ensures that we will keep track of at most 5,000 GitOpsDeployment UIDs.
	// We don't want to overwhelm our Go process by asking it to hold too many strings.
	// Assuming 400 bytes per string, this would be ~2MB of memory.
	// If/when the GitOps Service has a larger number of users, this should be increased.
	maxTrackedDeployments = 5000
)

func generateMapKey(resourceName string, resourceNamespace string, resourceNamespaceUID string) string {
	// TODO: GITOPSRVCE-68: PERF - Use a more memory efficient key
	return fmt.Sprintf("(%s)-(%s)-(%s)", resourceName, resourceNamespace, resourceNamespaceUID)
}

func AddOrUpdateGitOpsDeployment(resourceName string, resourceNamespace string, resourceNamespaceUID string) {
	activeGitOpsDeployments.mutex.Lock()
	defer activeGitOpsDeployments.mutex.Unlock()

	mapKey := generateMapKey(resourceName, resourceNamespace, resourceNamespaceUID)

	// Add the key to the list of tracked GitOpsDeployments, but only if there are less than 'maxTrackedDeployments'
	if len(activeGitOpsDeployments.gitOpsDeployments) <= maxTrackedDeployments {
		if _, exists := activeGitOpsDeployments.gitOpsDeployments[mapKey]; !exists {
			activeGitOpsDeployments.gitOpsDeployments[mapKey] = false
		}
	}

	Gitopsdepl.Set((float64)(len(activeGitOpsDeployments.gitOpsDeployments)))
}

func RemoveGitOpsDeployment(resourceName string, resourceNamespace string, resourceNamespaceUID string) {

	activeGitOpsDeployments.mutex.Lock()
	defer activeGitOpsDeployments.mutex.Unlock()

	mapKey := generateMapKey(resourceName, resourceNamespace, resourceNamespaceUID)

	// If the GitOpsDeployment is in our tracked list, and we know it is in error state,
	// then decrement the error metric
	inErrorState, exists := activeGitOpsDeployments.gitOpsDeployments[mapKey]
	if exists {
		if inErrorState {
			GitopsdeplFailures.Dec()
		}
	}

	delete(activeGitOpsDeployments.gitOpsDeployments, mapKey)

	// Update the total number of GitOpsDeployments, now that it has changed.
	Gitopsdepl.Set((float64)(len(activeGitOpsDeployments.gitOpsDeployments)))

}

func SetErrorState(resourceName string, resourceNamespace string, resourceNamespaceUID string, newInErrorState bool) {
	activeGitOpsDeployments.mutex.Lock()
	defer activeGitOpsDeployments.mutex.Unlock()

	mapKey := generateMapKey(resourceName, resourceNamespace, resourceNamespaceUID)

	inErrorState, exists := activeGitOpsDeployments.gitOpsDeployments[mapKey]

	if !exists {
		// If we aren't tracking the resource, then we don't need to update the metric for it
		return
	}

	activeGitOpsDeployments.gitOpsDeployments[mapKey] = newInErrorState

	if inErrorState && !newInErrorState {
		// An error has converted to a non-error, so decrement
		GitopsdeplFailures.Dec()
	} else if !inErrorState && newInErrorState {
		// A non-error has converted to an error, so increment
		GitopsdeplFailures.Inc()
	}

}

func ClearMetrics() {
	Gitopsdepl.Set(0)
	GitopsdeplFailures.Set(0)
	activeGitOpsDeployments.mutex.Lock()
	defer activeGitOpsDeployments.mutex.Unlock()

	activeGitOpsDeployments.gitOpsDeployments = map[string]bool{}
}

func init() {
	metric.Registry.MustRegister(Gitopsdepl, GitopsdeplFailures)
}
