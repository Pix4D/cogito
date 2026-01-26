package retry

import (
	"time"
)

// Deprecated: Package retry is replaced by github.com/Pix4D/go-kit/retry.
func ConstantBackoff(first bool, previous, limit time.Duration, err error) time.Duration {
	return min(previous, limit)
}

// Deprecated: Package retry is replaced by github.com/Pix4D/go-kit/retry.
func ExponentialBackoff(first bool, previous, limit time.Duration, err error) time.Duration {
	if first {
		return previous
	}
	next := 2 * previous
	return min(next, limit)
}
