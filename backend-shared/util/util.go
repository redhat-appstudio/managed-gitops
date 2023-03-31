package util

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ArgoCDDefaultDestinationInCluster is 'in-cluster' which is the spec destination value that Argo CD recognizes
	ArgoCDDefaultDestinationInCluster = "in-cluster"
)

const (
	ArgoCDSecretTypeIdentifierKey = "argocd.argoproj.io/secret-type" //Secret label key to define secret type.
	ArgoCDSecretClusterTypeValue  = "cluster"                        // Secret type for Cluster Secret
	ArgoCDSecretRepoTypeValue     = "repository"                     // Secret type for Repository Secret

	ManagedEnvironmentSecretType = "managed-gitops.redhat.com/managed-environment"
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

/* #nosec */
func (e *ExponentialBackoff) increaseDueToFail() {
	if e.curr == nil {
		e.curr = &e.Min
	} else {
		newDurationVal := time.Duration(float64(e.Factor) * float64(*e.curr))

		if e.Jitter {
			// Randomly increase the result by up to 10%, to introduce a small amount of jitter
			newDurationVal = time.Duration(float64(newDurationVal) * (1 + (rand.Float64() * float64(0.10))))
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

	panicLog := log.FromContext(context.Background())

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
