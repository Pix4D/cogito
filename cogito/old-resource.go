// Package resource is a Concourse resource to update the GitHub status.
//
// See the README file for additional information.
package cogito

const (
	gchatWebhookKey = "gchat_webhook"

	stateKey = "state"

	abortState   = "abort"
	errorState   = "error"
	failureState = "failure"
	pendingState = "pending"
	successState = "success"
)

var (
	// States that will trigger a chat notification by default.
	statesToNotifyChat = []string{abortState, errorState, failureState}
)
