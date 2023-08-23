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

type Retry struct {
	FirstDelay   time.Duration
	BackoffLimit time.Duration
	UpTo         time.Duration
	SleepFn      func(d time.Duration)
	Log          *slog.Logger
}

// Do WRITEME
func (rtr Retry) Do(
	backoffFn func(first bool, previous, limit time.Duration, userCtx any) time.Duration,
	classifierFn func(err error, userCtx any) Action,
	workFn func(userCtx any) error,
	userCtx any,
) error {
	if rtr.FirstDelay <= 0 {
		return errors.New("FirstDelay must be positive")
	}
	if rtr.SleepFn == nil {
		rtr.SleepFn = time.Sleep
	}
	rtr.Log = rtr.Log.With("system", "retry") // FIXME maybe better constructor???
	delay := rtr.FirstDelay
	cumulativeDelay := 0 * time.Second
	for attempt := 1; ; attempt++ {
		err := workFn(userCtx)
		switch classifierFn(err, userCtx) {
		case Success:
			rtr.Log.Info("success", "attempt", attempt, "cumulativeDelay", cumulativeDelay)
			return err
		case HardFail:
			return err
		case SoftFail:
			delay = backoffFn(attempt == 1, delay, rtr.BackoffLimit, userCtx)
			cumulativeDelay += delay
			if cumulativeDelay > rtr.UpTo {
				rtr.Log.Error("would wait for too long",
					"delay", delay, "cumulativeDelay", cumulativeDelay, "UpTo", rtr.UpTo)
				return err
			}
			// FIXME do we want the reason?
			//rtr.Log.Info("waiting for", "delay", delay, "reason", reason)
			rtr.Log.Info("waiting", "delay", delay, "attempt", attempt)
			rtr.SleepFn(delay)
		default:
			return errors.New("retry: internal error, please report")
		}
	}
}
