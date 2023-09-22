package retry_test

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/Pix4D/cogito/retry"
)

func TestRetrySuccessOnFirstAttempt(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		UpTo:         5 * time.Second,
		FirstDelay:   1 * time.Second,
		BackoffLimit: 1 * time.Minute,
		Log:          makeLog(),
		SleepFn:      func(d time.Duration) { sleeps = append(sleeps, d) },
	}
	workFn := func() error { return nil }

	err := rtr.Do(retry.ConstantBackoff, retryOnError, workFn)

	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(sleeps))
}

func TestRetrySuccessOnThirdAttempt(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		UpTo:         5 * time.Second,
		FirstDelay:   1 * time.Second,
		BackoffLimit: 1 * time.Minute,
		Log:          makeLog(),
		SleepFn:      func(d time.Duration) { sleeps = append(sleeps, d) },
	}
	attempt := 0
	workFn := func() error {
		attempt++
		if attempt == 3 {
			return nil
		}
		return fmt.Errorf("attempt %d", attempt)
	}

	err := rtr.Do(retry.ConstantBackoff, retryOnError, workFn)

	qt.Assert(t, qt.IsNil(err))
	wantSleeps := []time.Duration{time.Second, time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

func TestRetryFailureRunOutOfTime(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		UpTo:         5 * time.Second,
		FirstDelay:   1 * time.Second,
		BackoffLimit: 1 * time.Minute,
		Log:          makeLog(),
		SleepFn:      func(d time.Duration) { sleeps = append(sleeps, d) },
	}
	ErrAlwaysFail := errors.New("I always fail")
	workFn := func() error { return ErrAlwaysFail }

	err := rtr.Do(retry.ConstantBackoff, retryOnError, workFn)

	qt.Assert(t, qt.ErrorIs(err, ErrAlwaysFail))
	wantSleeps := []time.Duration{
		time.Second, time.Second, time.Second, time.Second, time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

func TestRetryExponentialBackOff(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		FirstDelay:   1 * time.Second,
		BackoffLimit: 4 * time.Second,
		UpTo:         11 * time.Second,
		SleepFn:      func(d time.Duration) { sleeps = append(sleeps, d) },
		Log:          makeLog(),
	}
	ErrAlwaysFail := errors.New("I always fail")
	workFn := func() error { return ErrAlwaysFail }

	err := rtr.Do(retry.ExponentialBackoff, retryOnError, workFn)

	qt.Assert(t, qt.ErrorIs(err, ErrAlwaysFail))
	wantSleeps := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

func TestRetryFailureHardFailOnSecondAttempt(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		UpTo:         5 * time.Second,
		FirstDelay:   1 * time.Second,
		BackoffLimit: 1 * time.Minute,
		Log:          makeLog(),
		SleepFn:      func(d time.Duration) { sleeps = append(sleeps, d) },
	}
	ErrUnrecoverable := errors.New("I am unrecoverable")
	classifierFn := func(err error) retry.Action {
		if errors.Is(err, ErrUnrecoverable) {
			return retry.HardFail
		}
		if err != nil {
			return retry.SoftFail
		}
		return retry.Success
	}
	attempt := 0
	workFn := func() error {
		attempt++
		if attempt == 2 {
			return ErrUnrecoverable
		}
		return fmt.Errorf("attempt %d", attempt)
	}

	err := rtr.Do(retry.ConstantBackoff, classifierFn, workFn)

	qt.Assert(t, qt.ErrorIs(err, ErrUnrecoverable))
	wantSleeps := []time.Duration{time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

func retryOnError(err error) retry.Action {
	if err != nil {
		return retry.SoftFail
	}
	return retry.Success
}

func makeLog() *slog.Logger {
	out := io.Discard
	if testing.Verbose() {
		out = os.Stdout
	}
	return slog.New(slog.NewTextHandler(out, nil))
}
