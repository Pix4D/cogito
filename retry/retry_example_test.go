package retry_test

// Go testable examples.
// Any function in a test package with prefix "Example" is a "testable example".
// It will be run as a test and the output must match the "Output:" in the comment.
// See https://go.dev/blog/examples.

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Pix4D/cogito/retry"
)

func ExampleRetry() {
	rtr := retry.Retry{
		UpTo:         5 * time.Second,
		FirstDelay:   1 * time.Second,
		BackoffLimit: 1 * time.Second,
		Log: slog.New(slog.NewTextHandler(os.Stdout,
			&slog.HandlerOptions{ReplaceAttr: removeTime})),
	}

	workFn := func() error {
		// Do work...
		// If something fails, as usual, return error.

		// Everything went well.
		return nil
	}
	classifierFn := func(err error) retry.Action {
		if err != nil {
			return retry.SoftFail
		}
		return retry.Success
	}

	err := rtr.Do(retry.ConstantBackoff, classifierFn, workFn)
	if err != nil {
		// Handle error...
		fmt.Println("error:", err)
	}

	// Output:
	// level=INFO msg=success system=retry attempt=1 totalDelay=0s
}

// Used in [ExampleRetry_CustomClassifier].
var ErrBananaUnavailable = errors.New("banana service unavailable")

// Embedded in [BananaResponseError].
type BananaResponse struct {
	Amount int
	// In practice, more fields here...
}

// Used in [ExampleRetry_CustomClassifier].
type BananaResponseError struct {
	Response *BananaResponse
	// In practice, more fields here...
}

func (eb BananaResponseError) Error() string {
	return "look at my fields, there is more information there"
}

func Example_retryCustomClassifier() {
	rtr := retry.Retry{
		UpTo:         30 * time.Second,
		FirstDelay:   2 * time.Second,
		BackoffLimit: 1 * time.Minute,
		Log: slog.New(slog.NewTextHandler(os.Stdout,
			&slog.HandlerOptions{ReplaceAttr: removeTime})),
		SleepFn: func(d time.Duration) {}, // Only for the test!
	}

	attempt := 0
	workFn := func() error {
		attempt++
		if attempt == 3 {
			// Error wrapping is optional; we do it to show that it works also.
			return fmt.Errorf("workFn: %w",
				BananaResponseError{Response: &BananaResponse{Amount: 42}})
		}
		if attempt < 5 {
			return ErrBananaUnavailable
		}
		// On 5th attempt we finally succeed.
		return nil
	}

	classifierFn := func(err error) retry.Action {
		var bananaResponseErr BananaResponseError
		if errors.As(err, &bananaResponseErr) {
			response := bananaResponseErr.Response
			if response.Amount == 42 {
				return retry.SoftFail
			}
			return retry.HardFail
		}
		if errors.Is(err, ErrBananaUnavailable) {
			return retry.SoftFail
		}
		if err != nil {
			return retry.HardFail
		}
		return retry.Success
	}

	err := rtr.Do(retry.ExponentialBackoff, classifierFn, workFn)
	if err != nil {
		// Handle error...
		fmt.Println("error:", err)
	}

	// Output:
	// level=INFO msg=waiting system=retry attempt=1 delay=2s totalDelay=2s
	// level=INFO msg=waiting system=retry attempt=2 delay=4s totalDelay=6s
	// level=INFO msg=waiting system=retry attempt=3 delay=8s totalDelay=14s
	// level=INFO msg=waiting system=retry attempt=4 delay=16s totalDelay=30s
	// level=INFO msg=success system=retry attempt=5 totalDelay=30s
}

// removeTime removes time-dependent attributes from log/slog records, making
// the output of testable examples [1] deterministic.
// [1]: https://go.dev/blog/examples
func removeTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	// if a.Key == "elapsed" {
	//	return slog.Attr{}
	// }
	return a
}
