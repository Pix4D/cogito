// Package retry implements a generic retry mechanism.
//
// Took some inspiration from:
// - https://github.com/eapache/go-resiliency/tree/main/retrier
// - https://github.com/hetznercloud/terraform-provider-hcloud/tree/main/internal/control
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

type Retry struct {
	UpTo         time.Duration
	FirstDelay   time.Duration
	BackoffLimit time.Duration
	Log          *slog.Logger
	// Override only in tests; default value is good.
	SleepFn func(d time.Duration)
}

type BackoffFunc func(first bool, previous, limit time.Duration, err error) time.Duration
type ClassifierFunc func(err error) Action
type WorkFunc func() error

// Do WRITEME
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
