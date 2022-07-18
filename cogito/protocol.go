// This file implements the Concourse resource protocol described at
// https://concourse-ci.org/implementing-resource-types.html

package cogito

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// CheckInput is the JSON object passed to the stdin of the "check" executable plus
// build metadata (environment variables).
//
// See https://concourse-ci.org/implementing-resource-types.html#resource-check
//
type CheckInput struct {
	Source Source `json:"source"`
	// Concourse will omit field Version from the first request.
	Version Version `json:"version"`
	Env     Environment
}

func NewCheckInput(in io.Reader) (CheckInput, error) {
	var ci CheckInput

	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ci); err != nil {
		return ci, fmt.Errorf("parsing JSON from stdin: %s", err)
	}

	if err := ci.Source.Init(); err != nil {
		return ci, err
	}

	// We don't validate the presence of field Version because, only for the check step,
	// Concourse will omit it from the _first_ request.

	ci.Env.Fill()

	return ci, nil
}

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
	GChatWebHook  string `json:"gchat_webhook"` // SENSITIVE
	LogLevel      string `json:"log_level"`
	LogUrl        string `json:"log_url"` // DEPRECATED
	ContextPrefix string `json:"context_prefix"`
}

// String renders Source, redacting the sensitive fields.
func (src Source) String() string {
	var bld strings.Builder

	fmt.Fprintln(&bld, "owner:         ", src.Owner)
	fmt.Fprintln(&bld, "repo:          ", src.Repo)
	fmt.Fprintln(&bld, "access_token:  ", redact(src.AccessToken))
	fmt.Fprintln(&bld, "gchat_webhook: ", redact(src.GChatWebHook))
	fmt.Fprintln(&bld, "log_level:     ", src.LogLevel)
	fmt.Fprint(&bld, "context_prefix: ", src.ContextPrefix)

	return bld.String()
}

// redact returns a redacted version of s. If s is empty, it returns the empty string.
func redact(s string) string {
	if s != "" {
		s = "***REDACTED***"
	}
	return s
}

// Init validates and applies defaults for Source.
func (src *Source) Init() error {
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
	// Validate optional fields.
	//

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
	// Apply defaults.
	//
	if src.LogLevel == "" {
		src.LogLevel = "info"
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
