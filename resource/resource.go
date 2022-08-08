// Package resource is a Concourse resource to update the GitHub status.
//
// See the README file for additional information.
package resource

import (
	"fmt"

	oc "github.com/cloudboss/ofcourse/ofcourse"
)

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

// Resource satisfies the ofcourse.Resource interface.
type Resource struct {
}

// Out satisfies ofcourse.Resource.Out(), corresponding to the /opt/resource/out command.
func (r *Resource) Out(
	inputDir string, // All the resource "put inputs" are below this directory.
	source oc.Source,
	params oc.Params,
	env oc.Environment,
	log *oc.Logger,
) (oc.Version, oc.Metadata, error) {

	// STUFF DELETED

	//
	// Post the status to all sinks and collect the sinkErrors.
	//
	var sinkErrors = map[string]error{}

	//
	// Post the status to chat sink.
	//
	gitRef := "dummy"
	err := sendToChat(source, params, env, log, gitRef)
	if err != nil {
		sinkErrors["google chat"] = err
	}

	// We treat all sinks as equal: it is enough for one to fail to cause the put
	// operation to fail.
	if len(sinkErrors) > 0 {
		return nil, nil, fmt.Errorf("out: %s", stringify(sinkErrors))
	}

	state, _ := params[stateKey].(string)
	metadata := oc.Metadata{}
	metadata = append(metadata, oc.NameVal{Name: stateKey, Value: state})

	return oc.Version{}, metadata, nil
}
