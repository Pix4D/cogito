package retry

// Action is the type returned by a Classifier to indicate how the Retrier should proceed.
type Action int

const (
	Success  Action = iota // Succeed indicates the Retrier should treat this value as a success.
	HardFail               // Fail indicates the Retrier should treat this value as a hard failure and not retry.
	SoftFail               // Retry indicates the Retrier should treat this value as a soft failure and retry.
)

func SimpleClassifier(err error, userCtx any) Action {
	if err != nil {
		return SoftFail
	}
	return Success
}
