package retry

import "time"

func ConstantBackoff(first bool, previous, limit time.Duration, userCtx any) time.Duration {
	return previous
}

func ExponentialBackoff(first bool, previous, limit time.Duration, userCtx any) time.Duration {
	if first {
		return previous
	}
	next := 2 * previous
	return min(next, limit)
}
