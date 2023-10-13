// Package retry implements a generic and customizable retry mechanism.
//
// Took some inspiration from:
// - https://github.com/eapache/go-resiliency/tree/main/retrier
package retry

import (
	"errors"
	"log/slog"
	"time"
)

// Action is returned by a ClassifierFunc to indicate to Retry how to proceed.
type Action int

const (
	// Success informs Retry that the attempt has been a success.
	Success Action = iota
	// HardFail informs Retry that the attempt has been a hard failure and
	// thus should abort retrying.
	HardFail
	// SoftFail informs Retry that the attempt has been a soft failure and
	// thus should keep retrying.
	SoftFail
)

// Retry is the controller of the retry mechanism.
// See the examples in file retry_example_test.go.
type Retry struct {
	UpTo         time.Duration // Total maximum duration of the retries.
	FirstDelay   time.Duration // Duration of the first backoff.
	BackoffLimit time.Duration // Upper bound duration of a backoff.
	Log          *slog.Logger
	SleepFn      func(d time.Duration) // Optional; used only to override in tests.
}

// BackoffFunc returns the next backoff duration; called by [Retry.Do].
// You can use one of the ready-made functions [ConstantBackoff],
// [ExponentialBackoff] or write your own.
// Parameter err allows to optionally inspect the error that caused the retry
// and return a custom delay; this can be used in special cases such as when
// rate-limited with a fixed window; for an example see
// [github.com/Pix4D/cogito/github.Backoff].
type BackoffFunc func(first bool, previous, limit time.Duration, err error) time.Duration

// ClassifierFunc decides whether to proceed or not; called by [Retry.Do].
// Parameter err allows to inspect the error; for an example see
// [github.com/Pix4D/cogito/github.Classifier]
type ClassifierFunc func(err error) Action

// WorkFunc does the unit of work that might fail and need to be retried; called
// by [Retry.Do].
type WorkFunc func() error

// Do is the loop of [Retry].
// See the examples in file retry_example_test.go.
func (rtr Retry) Do(
	backoffFn BackoffFunc,
	classifierFn ClassifierFunc,
	workFn WorkFunc,
) error {
	if rtr.FirstDelay <= 0 {
		return errors.New("FirstDelay must be positive")
	}
	if rtr.BackoffLimit <= 0 {
		return errors.New("BackoffLimit must be positive")
	}
	if rtr.SleepFn == nil {
		rtr.SleepFn = time.Sleep
	}
	rtr.Log = rtr.Log.With("system", "retry") // FIXME maybe better constructor???

	delay := rtr.FirstDelay
	totalDelay := 0 * time.Second

	for attempt := 1; ; attempt++ {
		err := workFn()
		switch classifierFn(err) {
		case Success:
			rtr.Log.Info("success", "attempt", attempt, "totalDelay", totalDelay)
			return err
		case HardFail:
			return err
		case SoftFail:
			delay = backoffFn(attempt == 1, delay, rtr.BackoffLimit, err)
			totalDelay += delay
			if totalDelay > rtr.UpTo {
				rtr.Log.Error("would wait for too long", "attempt", attempt,
					"delay", delay, "totalDelay", totalDelay, "UpTo", rtr.UpTo)
				return err
			}
			rtr.Log.Info("waiting", "attempt", attempt, "delay", delay,
				"totalDelay", totalDelay)
			rtr.SleepFn(delay)
		default:
			return errors.New("retry: internal error, please report")
		}
	}
}
