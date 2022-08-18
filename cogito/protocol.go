// This file implements the Concourse resource protocol described at
// https://concourse-ci.org/implementing-resource-types.html

package cogito

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DummyVersion is the version always returned by the Cogito resource.
// DO NOT REASSIGN!
var DummyVersion = Version{Ref: "dummy"}

// CheckRequest contains the JSON object passed on the stdin of the "check" executable
// (Source and Version) and the build metadata (Env, environment variables).
//
// See https://concourse-ci.org/implementing-resource-types.html#resource-check
type CheckRequest struct {
	Source Source `json:"source"`
	// Concourse will omit field Version from the first request.
	Version Version `json:"version"`
	Env     Environment
}

// GetRequest contains the JSON object passed on the stdin of the "request" executable
// (Source and Version) and the build metadata (Env, environment variables).
//
// See https://concourse-ci.org/implementing-resource-types.html#resource-in
type GetRequest struct {
	Source  Source  `json:"source"`
	Version Version `json:"version"`
	// Cogito does not support get params; a resource supporting them would have the
	// following line uncommented:
	// Params  GetParams `json:"params"`
	Env Environment
}

// PutRequest contains the JSON object passed to the stdin of the "out" executable
// (Source and Params) and the build metadata (Env, environment variables).
//
// See https://concourse-ci.org/implementing-resource-types.html#resource-out
type PutRequest struct {
	Source Source    `json:"source"`
	Params PutParams `json:"params"`
	Env    Environment
}

// DO NOT REASSIGN.
var defaultNotifyStates = []BuildState{StateAbort, StateError, StateFailure}

// Source is the "source:" block in a pipeline "resources:" block for the Cogito resource.
type Source struct {
	//
	// Mandatory
	//
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	AccessToken string `json:"access_token"` // SENSITIVE
	//
	// Optional
	//
	GChatWebHook   string       `json:"gchat_webhook"` // SENSITIVE
	LogLevel       string       `json:"log_level"`
	LogUrl         string       `json:"log_url"` // DEPRECATED
	ContextPrefix  string       `json:"context_prefix"`
	NotifyOnStates []BuildState `json:"notify_on_states"`
}

// String renders Source, redacting the sensitive fields.
func (src Source) String() string {
	var bld strings.Builder

	fmt.Fprintf(&bld, "owner:            %s\n", src.Owner)
	fmt.Fprintf(&bld, "repo:             %s\n", src.Repo)
	fmt.Fprintf(&bld, "access_token:     %s\n", redact(src.AccessToken))
	fmt.Fprintf(&bld, "gchat_webhook:    %s\n", redact(src.GChatWebHook))
	fmt.Fprintf(&bld, "log_level:        %s\n", src.LogLevel)
	fmt.Fprintf(&bld, "context_prefix:   %s\n", src.ContextPrefix)
	// Last one: no newline
	fmt.Fprintf(&bld, "notify_on_states: %s", src.NotifyOnStates)

	return bld.String()
}

// redact returns a redacted version of s. If s is empty, it returns the empty string.
func redact(s string) string {
	if s != "" {
		s = "***REDACTED***"
	}
	return s
}

// ValidateLog validates and applies defaults for the logging configuration of Source.
//
// This chicken-and-egg problem is due to the fact that logging configuration is passed
// too late, at the same time as all the other resource Source configuration, so to give
// as much debugging information as possible we need to get the log level as soon as
// possible, also if the Source has other errors. This cannot be simplified, we are
// working within the limits of the Concourse resource protocol.
func (src *Source) ValidateLog() error {
	// Normally we would leave this validation directly to the logging package, but since
	// the log level names are part of the Cogito API and predate the removal of ofcourse,
	// we need to handle the mapping and the error message, to avoid confusing the user.
	logAdapter := map[string]string{
		"debug":  "debug",
		"info":   "info",
		"warn":   "warn",
		"error":  "error",
		"silent": "off", // different
	}
	if src.LogLevel != "" {
		if _, ok := logAdapter[src.LogLevel]; !ok {
			return fmt.Errorf("source: invalid log_level: %s", src.LogLevel)
		}
		src.LogLevel = logAdapter[src.LogLevel]
	}

	//
	// Apply defaults for logging.
	//
	if src.LogLevel == "" {
		src.LogLevel = "info"
	}

	return nil
}

// Validate verifies the Source configuration and applies defaults.
func (src *Source) Validate() error {
	//
	// Validate mandatory fields.
	//
	var mandatory []string
	if src.Owner == "" {
		mandatory = append(mandatory, "owner")
	}
	if src.Repo == "" {
		mandatory = append(mandatory, "repo")
	}
	if src.AccessToken == "" {
		mandatory = append(mandatory, "access_token")
	}
	if len(mandatory) > 0 {
		return fmt.Errorf("source: missing keys: %s", strings.Join(mandatory, ", "))
	}

	//
	// Validate optional fields. In this case, nothing to do.
	//

	//
	// Apply defaults.
	//
	if len(src.NotifyOnStates) == 0 {
		src.NotifyOnStates = defaultNotifyStates
	}

	return nil
}

// Version is a JSON object part of the Concourse resource protocol. The only requirement
// is that the fields must be of type string, but the keys can be anything.
// For Cogito, we have one key, "ref".
type Version struct {
	Ref string `json:"ref"`
}

// String renders Version.
func (ver Version) String() string {
	return fmt.Sprint("ref: ", ver.Ref)
}

// Output is the JSON object emitted by the get and put step.
type Output struct {
	Version  Version    `json:"version"`
	Metadata []Metadata `json:"metadata"`
}

// Metadata is an element of a list of indirect k/v pairs, part of the Concourse protocol.
//
// Note that Concourse confusingly uses the term "metadata" for two completely different
// concepts: (1) the environment variables made available from Concourse to the check, get
// and put steps and (2) the metadata k/v map outputted by the get and put steps.
type Metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// GetParams is the "params:" block in a pipeline get step for the Cogito resource.
// Cogito doesn't accept any get step parameters, so to give a slightly better error
// message from the JSON parsing, instead of declaring it an empty struct, we do not
// declare it at all.
// type GetParams struct{}

// BuildState is a pseudo-enum representing the valid values of PutParams.State
type BuildState string

// NOTE: this list must be kept in sync with the custom JSON methods of [BuildState].
const (
	StateAbort   BuildState = "abort"
	StateError   BuildState = "error"
	StateFailure BuildState = "failure"
	StatePending BuildState = "pending"
	StateSuccess BuildState = "success"
)

const KeyState = "state"

func (bs *BuildState) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	*bs = BuildState(str)

	switch *bs {
	case StateAbort, StateError, StateFailure, StatePending, StateSuccess:
		return nil
	default:
		return fmt.Errorf("invalid build state: %s", str)
	}
}

func (bs BuildState) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(bs))
}

// PutParams is the "params:" block in a pipeline put step for the Cogito resource.
type PutParams struct {
	//
	// Mandatory
	//
	State BuildState `json:"state"`
	//
	// Optional
	//
	Context string `json:"context"`
}

// Environment represents the environment variables made available to the program.
// Depending on the type of build and on the step, only some variables could be set.
// See https://concourse-ci.org/implementing-resource-types.html#resource-metadata
type Environment struct {
	BuildId                   string
	BuildName                 string
	BuildJobName              string
	BuildPipelineName         string
	BuildPipelineInstanceVars string
	BuildTeamName             string
	BuildCreatedBy            string
	AtcExternalUrl            string
}

// Fill fills Environment by reading the associated environment variables.
func (env *Environment) Fill() {
	env.BuildId = os.Getenv("BUILD_ID")
	env.BuildName = os.Getenv("BUILD_NAME")
	env.BuildJobName = os.Getenv("BUILD_JOB_NAME")
	env.BuildPipelineName = os.Getenv("BUILD_PIPELINE_NAME")
	env.BuildPipelineInstanceVars = os.Getenv("BUILD_PIPELINE_INSTANCE_VARS")
	env.BuildTeamName = os.Getenv("BUILD_TEAM_NAME")
	env.BuildCreatedBy = os.Getenv("BUILD_CREATED_BY")
	env.AtcExternalUrl = os.Getenv("ATC_EXTERNAL_URL")
}

// String renders Environment.
func (env Environment) String() string {
	var bld strings.Builder

	fmt.Fprintln(&bld, "BUILD_ID:                    ", env.BuildId)
	fmt.Fprintln(&bld, "BUILD_NAME:                  ", env.BuildName)
	fmt.Fprintln(&bld, "BUILD_JOB_NAME:              ", env.BuildJobName)
	fmt.Fprintln(&bld, "BUILD_PIPELINE_NAME:         ", env.BuildPipelineName)
	fmt.Fprintln(&bld, "BUILD_PIPELINE_INSTANCE_VARS:", env.BuildPipelineInstanceVars)
	fmt.Fprintln(&bld, "BUILD_TEAM_NAME:             ", env.BuildTeamName)
	fmt.Fprintln(&bld, "BUILD_CREATED_BY:            ", env.BuildCreatedBy)
	fmt.Fprint(&bld, "ATC_EXTERNAL_URL:            ", env.AtcExternalUrl)

	return bld.String()
}
