package cogito_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/testhelp"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestSourceValidateLogSuccess(t *testing.T) {
	type testCase struct {
		name   string
		source cogito.Source
		want   cogito.Source
	}

	test := func(t *testing.T, tc testCase) {

		err := tc.source.ValidateLog()

		assert.NilError(t, err)
		assert.DeepEqual(t, tc.source, tc.want)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:   "apply defaults",
			source: baseSource,
			want:   testhelp.MergeStructs(baseSource, cogito.Source{LogLevel: "info"}),
		},
		{
			name:   "override defaults",
			source: testhelp.MergeStructs(baseSource, cogito.Source{LogLevel: "debug"}),
			want:   testhelp.MergeStructs(baseSource, cogito.Source{LogLevel: "debug"}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestSourceValidateLogFailure(t *testing.T) {
	type testCase struct {
		name    string
		source  cogito.Source
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")

		err := tc.source.ValidateLog()

		assert.Error(t, err, tc.wantErr)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:    "invalid log level",
			source:  testhelp.MergeStructs(baseSource, cogito.Source{LogLevel: "pippo"}),
			wantErr: "source: invalid log_level: pippo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestSourceValidationSuccess(t *testing.T) {
	type testCase struct {
		name   string
		source cogito.Source
	}

	test := func(t *testing.T, tc testCase) {
		err := tc.source.Validate()

		assert.NilError(t, err)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:   "only mandatory keys",
			source: baseSource,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestSourceValidationFailure(t *testing.T) {
	type testCase struct {
		name    string
		source  cogito.Source
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")

		err := tc.source.Validate()

		assert.Error(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "missing mandatory source keys",
			source:  cogito.Source{},
			wantErr: "source: missing keys: owner, repo, access_token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

// The majority of tests for failure are done in TestSourceValidationFailure, which limits
// the input since it uses a struct. Thus, we also test with some raw JSON input text.
func TestSourceParseRawFailure(t *testing.T) {
	type testCase struct {
		name    string
		input   string
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		assert.Assert(t, tc.wantErr != "")
		in := strings.NewReader(tc.input)
		var source cogito.Source
		dec := json.NewDecoder(in)
		dec.DisallowUnknownFields()

		err := dec.Decode(&source)

		assert.Error(t, err, tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "empty input",
			input:   ``,
			wantErr: `EOF`,
		},
		{
			name:    "malformed input",
			input:   `pizza`,
			wantErr: `invalid character 'p' looking for beginning of value`,
		},
		{
			name: "JSON types validation is automatic (since Go is statically typed)",
			input: `
{
  "owner": 123
}`,
			wantErr: `json: cannot unmarshal number into Go struct field Source.owner of type string`,
		},
		{
			name: "Unknown fields are caught automatically by the JSON decoder",
			input: `
{
  "owner": "the-owner",
  "repo": "the-repo",
  "access_token": "the-token",
  "hello": "I am an unknown key"
}`,
			wantErr: `json: unknown field "hello"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestSourcePrintLogRedaction(t *testing.T) {
	source := cogito.Source{
		Owner:          "the-owner",
		Repo:           "the-repo",
		AccessToken:    "sensitive-the-access-token",
		GChatWebHook:   "sensitive-gchat-webhook",
		LogLevel:       "debug",
		ContextPrefix:  "the-prefix",
		NotifyOnStates: []cogito.BuildState{cogito.StateSuccess, cogito.StateFailure},
	}

	t.Run("fmt.Print redacts fields", func(t *testing.T) {
		want := `owner:            the-owner
repo:             the-repo
access_token:     ***REDACTED***
gchat_webhook:    ***REDACTED***
log_level:        debug
context_prefix:   the-prefix
notify_on_states: [success failure]`

		have := fmt.Sprint(source)

		assert.Equal(t, have, want)
	})

	t.Run("empty fields are not marked as redacted", func(t *testing.T) {
		input := cogito.Source{
			Owner: "the-owner",
		}
		want := `owner:            the-owner
repo:             
access_token:     
gchat_webhook:    
log_level:        
context_prefix:   
notify_on_states: []`

		have := fmt.Sprint(input)

		assert.Equal(t, have, want)
	})

	t.Run("hclog redacts fields", func(t *testing.T) {
		var logBuf bytes.Buffer
		log := hclog.New(&hclog.LoggerOptions{Output: &logBuf})

		log.Info("log test", "source", source)
		have := logBuf.String()

		assert.Assert(t, cmp.Contains(have, "| access_token:     ***REDACTED***"))
		assert.Assert(t, cmp.Contains(have, "| gchat_webhook:    ***REDACTED***"))
		assert.Assert(t, !strings.Contains(have, "sensitive"))
	})
}

func TestVersion_String(t *testing.T) {
	version := cogito.Version{Ref: "pizza"}

	have := fmt.Sprint(version)

	assert.Equal(t, have, "ref: pizza")
}

func TestEnvironment(t *testing.T) {
	t.Setenv("BUILD_NAME", "banana-mango")
	env := cogito.Environment{}

	env.Fill()

	have := fmt.Sprint(env)
	assert.Assert(t, strings.Contains(have, "banana-mango"), "\n%s", have)
}

func TestBuildStateUnmarshalJSONSuccess(t *testing.T) {
	var state cogito.BuildState

	err := state.UnmarshalJSON([]byte(`"pending"`))

	assert.NilError(t, err)
	assert.Equal(t, state, cogito.StatePending)
}

func TestBuildStateUnmarshalJSONFailure(t *testing.T) {
	type testCase struct {
		name    string
		data    []byte
		wantErr string
	}

	test := func(t *testing.T, tc testCase) {
		var state cogito.BuildState

		assert.Error(t, state.UnmarshalJSON(tc.data), tc.wantErr)
	}

	testCases := []testCase{
		{
			name:    "no input",
			data:    nil,
			wantErr: "unexpected end of JSON input",
		},
		{
			name:    "",
			data:    []byte(`"pizza"`),
			wantErr: "invalid build state: pizza",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { test(t, tc) })
	}
}
