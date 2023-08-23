package retry_test

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/Pix4D/cogito/retry"
)

// Any function in a test package with prefix "Example" is a "testable example".
// It will be run as a test and the output must match the "Output:" in the comment.
// See https://go.dev/blog/examples.
func ExampleRetry() {
	rtr := retry.Retry{
		FirstDelay: 1 * time.Second,
		UpTo:       5 * time.Second,
		Log: slog.New(slog.NewTextHandler(os.Stdout,
			&slog.HandlerOptions{ReplaceAttr: removeTime})),
	}

	workFn := func(userCtx any) error {
		// Do work...
		return nil
	}

	err := rtr.Do(retry.ConstantBackoff, retry.SimpleClassifier, workFn, nil)
	if err != nil {
		// Handle error...
	}

	// Output:
	// level=INFO msg=success system=retry attempt=1 cumulativeDelay=0s
}

// Hope that this justifies the complexity of the API...
func ExampleRetryCustomFunctions() {
	rtr := retry.Retry{
		FirstDelay: 1 * time.Second,
		UpTo:       30 * time.Second,
		Log: slog.New(slog.NewTextHandler(os.Stdout,
			&slog.HandlerOptions{ReplaceAttr: removeTime})),
		SleepFn: func(d time.Duration) {}, // Only for the test!
	}

	type Response struct {
		Foo int
	}
	response := &Response{}

	attempt := 0
	workFn := func(userCtx any) error {
		attempt++
		response := userCtx.(*Response)
		response.Foo = attempt
		if attempt == 3 {
			response.Foo = 42
		}
		return nil
	}

	backoffFn := func(first bool, previous, limit time.Duration, userCtx any) time.Duration {
		if first {
			return previous
		}
		response := userCtx.(*Response)
		if response.Foo < 42 {
			return time.Duration(response.Foo+7) * time.Second
		}
		return 0
	}

	classifierFn := func(err error, userCtx any) retry.Action {
		response := userCtx.(*Response)
		if response.Foo == 42 {
			return retry.Success
		}
		return retry.SoftFail
	}

	err := rtr.Do(backoffFn, classifierFn, workFn, response)
	if err != nil {
		// Handle error...
	}

	// Output:
	//level=INFO msg=waiting system=retry delay=1s attempt=1
	//level=INFO msg=waiting system=retry delay=9s attempt=2
	//level=INFO msg=success system=retry attempt=3 cumulativeDelay=10s
}

func TestRetrySuccessOnFirstAttempt(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		FirstDelay: 1 * time.Second,
		UpTo:       5 * time.Second,
		SleepFn:    func(d time.Duration) { sleeps = append(sleeps, d) },
		Log:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	workFn := func(userCtx any) error { return nil }

	err := rtr.Do(retry.ConstantBackoff, retry.SimpleClassifier, workFn, nil)

	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(sleeps))
}

func TestRetrySuccessOnThirdAttempt(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		FirstDelay: 1 * time.Second,
		UpTo:       5 * time.Second,
		SleepFn:    func(d time.Duration) { sleeps = append(sleeps, d) },
		Log:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	attempt := 0
	workFn := func(userCtx any) error {
		attempt++
		if attempt == 3 {
			return nil
		}
		return fmt.Errorf("attempt %d", attempt)
	}

	err := rtr.Do(retry.ConstantBackoff, retry.SimpleClassifier, workFn, nil)

	qt.Assert(t, qt.IsNil(err))
	wantSleeps := []time.Duration{time.Second, time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

func TestRetryFailureRunOutOfTime(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		FirstDelay: 1 * time.Second,
		UpTo:       5 * time.Second,
		SleepFn:    func(d time.Duration) { sleeps = append(sleeps, d) },
		Log:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	ErrAlwaysFail := errors.New("I always fail")
	workFn := func(userCtx any) error { return ErrAlwaysFail }

	err := rtr.Do(retry.ConstantBackoff, retry.SimpleClassifier, workFn, nil)

	qt.Assert(t, qt.ErrorIs(err, ErrAlwaysFail))
	wantSleeps := []time.Duration{
		time.Second, time.Second, time.Second, time.Second, time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

// FIXME can we factor out the backoffs somehow??? table driven test...
// FIXME Jitter???
func TestRetryExponentialBackOff(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		FirstDelay:   1 * time.Second,
		BackoffLimit: 4 * time.Second,
		UpTo:         11 * time.Second,
		SleepFn:      func(d time.Duration) { sleeps = append(sleeps, d) },
		Log:          slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	ErrAlwaysFail := errors.New("I always fail")
	workFn := func(userCtx any) error { return ErrAlwaysFail }

	err := rtr.Do(retry.ExponentialBackoff, retry.SimpleClassifier, workFn, nil)

	qt.Assert(t, qt.ErrorIs(err, ErrAlwaysFail))
	wantSleeps := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

func TestRetryFailureHardFailOnSecondAttempt(t *testing.T) {
	var sleeps []time.Duration
	rtr := retry.Retry{
		FirstDelay: 1 * time.Second,
		UpTo:       5 * time.Second,
		SleepFn:    func(d time.Duration) { sleeps = append(sleeps, d) },
		Log:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
	ErrUnrecoverable := errors.New("I am unrecoverable")
	classifierFn := func(err error, userCtx any) retry.Action {
		if errors.Is(err, ErrUnrecoverable) {
			return retry.HardFail
		}
		if err != nil {
			return retry.SoftFail
		}
		return retry.Success
	}
	attempt := 0
	workFn := func(userCtx any) error {
		attempt++
		if attempt == 2 {
			return ErrUnrecoverable
		}
		return fmt.Errorf("attempt %d", attempt)
	}

	err := rtr.Do(retry.ConstantBackoff, classifierFn, workFn, nil)

	qt.Assert(t, qt.ErrorIs(err, ErrUnrecoverable))
	wantSleeps := []time.Duration{time.Second}
	qt.Assert(t, qt.DeepEquals(sleeps, wantSleeps))
}

// removeTime removes time-dependent attributes from log/slog records, making
// the output of testable examples [1] deterministic.
// [1]: https://go.dev/blog/examples
func removeTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	//if a.Key == "elapsed" {
	//	return slog.Attr{}
	//}
	return a
}
