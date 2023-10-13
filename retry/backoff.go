package retry

import (
	"time"
)

func ConstantBackoff(first bool, previous, limit time.Duration, err error) time.Duration {
	return min(previous, limit)
}

func ExponentialBackoff(first bool, previous, limit time.Duration, err error) time.Duration {
	if first {
		return previous
	}
	next := 2 * previous
	return min(next, limit)
}
