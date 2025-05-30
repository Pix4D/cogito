// This file implements the Concourse resource protocol described at
// https://concourse-ci.org/implementing-resource-types.html

package cogito

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/sets"
)

var hostnameRegexp = regexp.MustCompile(`^(?P<host>[a-zA-Z0-9.-]+)(?::(?P<port>\d+))?$`)

// DummyVersion is the version always returned by the Cogito resource.
// DO NOT REASSIGN!
var DummyVersion = Version{Ref: "dummy"}

// CheckRequest contains the JSON object passed on the stdin of the "check" executable
// (Source and Version) and the build metadata (Env, environment variables).
// Use [NewCheckRequest] to instantiate.
//
// See https://concourse-ci.org/implementing-resource-types.html#resource-check
type CheckRequest struct {
	Source Source `json:"source"`
	// Concourse will omit field Version from the first request.
	Version Version `json:"version"`
	Env     Environment
}

// NewCheckRequest returns a [CheckRequest] ready to be used.
func NewCheckRequest(input []byte) (CheckRequest, error) {
	var request CheckRequest
	// Since we also want to enforce the parser to fail if it encounters unknown fields,
	// we cannot use the customary json.Unmarshal(data, &aux) but we have to go through
	// a json decoder.
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		return CheckRequest{}, fmt.Errorf("check: parsing request: %s", err)
	}

	if err := request.Source.Validate(); err != nil {
		return CheckRequest{}, fmt.Errorf("check: %s", err)
	}

	request.Env.Fill()

	return request, nil
}

// GetRequest contains the JSON object passed on the stdin of the "request" executable
// (Source and Version) and the build metadata (Env, environment variables).
// Use [NewGetRequest] to instantiate.
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

// NewGetRequest returns a [GetRequest] ready to be used.
func NewGetRequest(input []byte) (GetRequest, error) {
	var request GetRequest
	// Since we also want to enforce the parser to fail if it encounters unknown fields,
	// we cannot use the customary json.Unmarshal(data, &aux) but we have to go through
	// a json decoder.
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		return GetRequest{}, fmt.Errorf("get: parsing request: %s", err)
	}

	if err := request.Source.Validate(); err != nil {
		return GetRequest{}, fmt.Errorf("get: %s", err)
	}

	request.Env.Fill()

	return request, nil
}

// PutRequest contains the JSON object passed to the stdin of the "out" executable
// (Source and Params) and the build metadata (Env, environment variables).
// Use [NewPutRequest] to instantiate.
//
// See https://concourse-ci.org/implementing-resource-types.html#resource-out
type PutRequest struct {
	Source Source    `json:"source"`
	Params PutParams `json:"params"`
	Env    Environment
}

// NewPutRequest returns a [PutRequest] ready to be used.
func NewPutRequest(input []byte) (PutRequest, error) {
	var request PutRequest
	// Since we also want to enforce the parser to fail if it encounters unknown fields,
	// we cannot use the customary json.Unmarshal(data, &aux) but we have to go through
	// a json decoder.
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		return PutRequest{}, fmt.Errorf("put: parsing request: %s", err)
	}

	if err := request.Source.Validate(); err != nil {
		return PutRequest{}, fmt.Errorf("put: %s", err)
	}

	request.Env.Fill()

	return request, nil
}

func (req *PutRequest) UnmarshalJSON(data []byte) error {
	type request PutRequest // Alias to avoid infinite loops.

	//
	// Parse Source. The method [Source.UnmarshalJSON] will set the needed defaults.
	//
	var aux1 request
	if err := json.Unmarshal(data, &aux1); err != nil {
		return err
	}
	req.Source = aux1.Source

	//
	// Parse Params with default values set from Source.
	//
	aux2 := request{
		Params: PutParams{
			ChatAppendSummary: req.Source.ChatAppendSummary, // default value
		},
	}
	// Since we also want to enforce the parser to fail if it encounters unknown fields,
	// we cannot use the customary json.Unmarshal(data, &aux) but we have to go through
	// a json decoder :-/
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux2); err != nil {
		return err
	}
	req.Params = aux2.Params

	return nil
}

// DO NOT REASSIGN.
var defaultNotifyStates = []BuildState{StateAbort, StateError, StateFailure}

// Source is the "source:" block in a pipeline "resources:" block for the Cogito resource.
type Source struct {
	//
	// Mandatory
	//
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	// Mandatory if not using github app auth
	AccessToken string `json:"access_token"` // SENSITIVE

	// Mandatory if using github app auth
	GitHubApp github.GitHubApp `json:"github_app"`

	//
	// Optional
	//
	GhHostname         string       `json:"github_hostname"`
	GChatWebHook       string       `json:"gchat_webhook"` // SENSITIVE
	LogLevel           string       `json:"log_level"`
	LogUrl             string       `json:"log_url"` // DEPRECATED
	ContextPrefix      string       `json:"context_prefix"`
	OmitTargetURL      bool         `json:"omit_target_url"`
	ChatAppendSummary  bool         `json:"chat_append_summary"`
	ChatNotifyOnStates []BuildState `json:"chat_notify_on_states"`
	Sinks              []string     `json:"sinks"`
}

// String renders Source, redacting the sensitive fields.
func (src Source) String() string {
	var bld strings.Builder

	fmt.Fprintf(&bld, "owner:                 %s\n", src.Owner)
	fmt.Fprintf(&bld, "repo:                  %s\n", src.Repo)
	fmt.Fprintf(&bld, "github_hostname:       %s\n", src.GhHostname)
	fmt.Fprintf(&bld, "access_token:          %s\n", redact(src.AccessToken))
	fmt.Fprintf(&bld, "gchat_webhook:         %s\n", redact(src.GChatWebHook))
	fmt.Fprintf(&bld, "github_app.client_id:        %s\n", src.GitHubApp.ClientId)
	fmt.Fprintf(&bld, "github_app.installation_id:  %d\n", src.GitHubApp.InstallationId)
	fmt.Fprintf(&bld, "github_app.private_key:      %s\n", redact(src.GitHubApp.PrivateKey))
	fmt.Fprintf(&bld, "log_level:             %s\n", src.LogLevel)
	fmt.Fprintf(&bld, "context_prefix:        %s\n", src.ContextPrefix)
	fmt.Fprintf(&bld, "omit_target_url:       %t\n", src.OmitTargetURL)
	fmt.Fprintf(&bld, "chat_append_summary:   %t\n", src.ChatAppendSummary)
	fmt.Fprintf(&bld, "chat_notify_on_states: %s\n", src.ChatNotifyOnStates)
	// Last one: no newline.
	fmt.Fprintf(&bld, "sinks: %s", src.Sinks)

	return bld.String()
}

// UnmarshalJSON is used to set some default values of the struct.
// See https://www.orsolabs.com/post/go-json-default-values/
func (src *Source) UnmarshalJSON(data []byte) error {
	type source Source // Alias to avoid infinite loop.

	// Set the default value before JSON parsing.
	aux := source{
		ChatAppendSummary: true,
	}
	// Since we also want to enforce the parser to fail if it encounters unknown fields,
	// we cannot use the customary json.Unmarshal(data, &aux) but we have to go through
	// a json decoder :-/
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return err
	}
	*src = Source(aux)

	return nil
}

// Validate verifies the Source configuration and applies defaults.
func (src *Source) Validate() error {
	//
	// Validate mandatory fields.
	//
	var mandatory []string

	// This is the Source validation about custom sinks,
	// the second argument represents the put params that is nil
	// at this stage.
	_, err := MergeAndValidateSinks(src.Sinks, nil)
	if err != nil {
		return fmt.Errorf("source: invalid sink(s): %w", err)
	}

	sinks := sets.From(src.Sinks...)
	if sinks.Size() == 0 || sinks.Contains("github") {
		// Cogito commit Github status mandatory fields.
		if !src.GitHubApp.IsZero() && src.AccessToken != "" {
			return fmt.Errorf("source: cannot specify both github_app and access_token")
		}

		// either access_token or github_app must be specified
		if src.AccessToken == "" && src.GitHubApp.IsZero() {
			return fmt.Errorf("source: one of access_token or github_app must be specified")
		}

		if src.Owner == "" {
			mandatory = append(mandatory, "owner")
		}
		if src.Repo == "" {
			mandatory = append(mandatory, "repo")
		}

		if !src.GitHubApp.IsZero() {
			if src.GitHubApp.ClientId == "" {
				mandatory = append(mandatory, "github_app.client_id")
			}
			if src.GitHubApp.InstallationId == 0 {
				mandatory = append(mandatory, "github_app.installation_id")
			}
			if src.GitHubApp.PrivateKey == "" {
				mandatory = append(mandatory, "github_app.private_key")
			}
		}
	}

	if sinks.Contains("gchat") {
		// Gchat is explicitly required so makes its setting mandatory.
		if src.GChatWebHook == "" {
			mandatory = append(mandatory, "gchat_webhook")
		}
	}

	if len(mandatory) > 0 {
		return fmt.Errorf("source: missing keys: %s", strings.Join(mandatory, ", "))
	}

	//
	// Validate optional fields.
	//
	// In this case, nothing to validate.

	//
	// Apply defaults.
	//
	if src.LogLevel == "" {
		src.LogLevel = "info"
	}
	if len(src.ChatNotifyOnStates) == 0 {
		src.ChatNotifyOnStates = defaultNotifyStates
	}
	if src.GhHostname == "" {
		src.GhHostname = github.GhDefaultHostname
	}
	// Validate src.GhHostname
	if !hostnameRegexp.MatchString(src.GhHostname) {
		return fmt.Errorf("source: invalid github_api_hostname: %s. Don't configure the schema or the path", src.GhHostname)
	}

	return nil
}

// redact returns a redacted version of s. If s is empty, it returns the empty string.
func redact(s string) string {
	if s != "" {
		s = "***REDACTED***"
	}
	return s
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
	Context           string   `json:"context"`
	ChatMessage       string   `json:"chat_message"`
	ChatMessageFile   string   `json:"chat_message_file"`
	ChatAppendSummary bool     `json:"chat_append_summary"`
	GChatWebHook      string   `json:"gchat_webhook"` // SENSITIVE
	Sinks             []string `json:"sinks"`
}

// String renders PutParams, redacting the sensitive fields.
func (params PutParams) String() string {
	var bld strings.Builder

	fmt.Fprintf(&bld, "state:               %s\n", params.State)
	fmt.Fprintf(&bld, "context:             %s\n", params.Context)
	fmt.Fprintf(&bld, "chat_message:        %s\n", params.ChatMessage)
	fmt.Fprintf(&bld, "chat_message_file:   %s\n", params.ChatMessageFile)
	fmt.Fprintf(&bld, "chat_append_summary: %v\n", params.ChatAppendSummary)
	fmt.Fprintf(&bld, "gchat_webhook:       %s\n", redact(params.GChatWebHook))
	// Last one: no newline.
	fmt.Fprintf(&bld, "sinks:               %s", params.Sinks)

	return bld.String()
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
