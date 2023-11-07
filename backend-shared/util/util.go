package util

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ArgoCDDefaultDestinationInCluster is 'in-cluster' which is the spec destination value that Argo CD recognizes
	ArgoCDDefaultDestinationInCluster = "in-cluster"

	SelfHealIntervalEnVar = "SELF_HEAL_INTERVAL" // Interval in minutes between self-healing runs
)

const (
	//Secret label key to define secret type.
	ArgoCDSecretTypeIdentifierKey = "argocd.argoproj.io/secret-type" // #nosec G101
	// Secret type for Cluster Secret
	ArgoCDSecretClusterTypeValue = "cluster" // #nosec G101
	// Secret type for Repository Secret
	ArgoCDSecretRepoTypeValue = "repository" // #nosec G101

	ManagedEnvironmentSecretType = "managed-gitops.redhat.com/managed-environment"

	RepositoryCredentialSecretType = "managed-gitops.redhat.com/repository-credential"

	Log_JobKey      = "job"                    // Clean up job key
	Log_JobKeyValue = "managed-gitops-cleanup" // Clean up job value
	Log_JobTypeKey  = "jobType"                // Key to identify clean up job type
)

// ExponentialBackoff: the more times in a row something fails, the longer we wait.
type ExponentialBackoff struct {
	Factor float64
	curr   *time.Duration
	Min    time.Duration
	Max    time.Duration
	Jitter bool
}

func (e *ExponentialBackoff) Reset() {
	e.curr = &e.Min
}

// IncreaseAndReturnNewDuration does NOT sleep (unlike DelayOnFail), instead it just increases the exponential
// back off duration and returns the result, allowing the calling function to use this value for sleeping/scheduling purpose.
func (e *ExponentialBackoff) IncreaseAndReturnNewDuration() time.Duration {
	e.increaseDueToFail()
	return *e.curr
}

func (e *ExponentialBackoff) increaseDueToFail() {
	if e.curr == nil {
		e.curr = &e.Min
	} else {
		newDurationVal := time.Duration(float64(e.Factor) * float64(*e.curr))

		if e.Jitter {
			// Randomly increase the result by up to 10%, to introduce a small amount of jitter
			newDurationVal = time.Duration(float64(newDurationVal) * (1 + (rand.Float64() * float64(0.10)))) // #nosec G404
		}

		e.curr = &newDurationVal
	}

	if *e.curr < e.Min {
		e.curr = &e.Min
	}

	if *e.curr > e.Max {
		e.curr = &e.Max
	}

}

func (e *ExponentialBackoff) DelayOnFail(ctx context.Context) {

	e.increaseDueToFail()

	select {
	case <-ctx.Done():
	case <-time.After(*e.curr):
	}

}

// RunUntilTrue runs the task function until the function returns true, or until the context is cancelled.
func RunTaskUntilTrue(ctx context.Context, backoff *ExponentialBackoff, taskDescription string, log logr.Logger, task func() (bool, error)) error {

	defer backoff.Reset()

	var taskComplete bool
	var err error

outer:
	for {
		// We only return if the context was cancelled.
		select {
		case <-ctx.Done():
			err = fmt.Errorf(taskDescription + ": context cancelled")
			break outer
		default:
		}

		taskComplete, err = task()

		if err != nil {
			log.Error(err, fmt.Sprintf("%s: %v", taskDescription, err))
		}

		if taskComplete {
			break outer
		} else {
			backoff.DelayOnFail(ctx)
		}

	}

	return err
}

// CatchPanic calls f(), and recovers from panic if one occurs.
func CatchPanic(f func() error) (isPanic bool, err error) {

	panicLog := log.FromContext(context.Background()).
		WithName(logutil.LogLogger_managed_gitops)

	isPanic = false

	doRecover := func() {
		recoverRes := recover()

		if recoverRes != nil {
			err = fmt.Errorf("panic: %v", recoverRes)
			panicLog.Error(err, "SEVERE: Panic occurred")
			isPanic = true
		}

	}

	defer doRecover()

	err = f()

	return isPanic, err
}

func GetRESTConfig() (*rest.Config, error) {
	res, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	// Use non-standard rate limiting values
	// TODO: GITOPSRVCE-68 - These values are way too high, need to look at request.go in controller-runtime.
	res.QPS = 100
	res.Burst = 250
	return res, nil
}

func SelfHealInterval(defaultValue time.Duration, logger logr.Logger) time.Duration {
	interval := os.Getenv(SelfHealIntervalEnVar)
	if interval == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(interval)
	if err != nil {
		msg := fmt.Sprintf("value of env var %s can't be converted to int", SelfHealIntervalEnVar)
		logger.Error(err, msg)
		return defaultValue
	}
	return time.Duration(value) * time.Minute
}

// AppProjectIsolationEnabled is a feature flag for AppProject-based isolation. To enable it, set the environment variable on the controllers.
func AppProjectIsolationEnabled() bool {

	// If the environment variable exists, and equals (case insensitive) "true", then enable AppProject-based isolation
	// otherwise, don't.

	return strings.EqualFold(os.Getenv("ENABLE_APPPROJECT_ISOLATION"), "true")
}
