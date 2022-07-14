package cogito_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Pix4D/cogito/cogito"
	"github.com/hashicorp/go-hclog"
	"gotest.tools/v3/assert"
)

func TestNewCheckInputSuccess(t *testing.T) {
	type testCase struct {
		name   string
		source []cogito.Source // 1st element is the default; 2nd is the override
		want   []cogito.Source // 1st element is the default; 2nd is the override
	}

	test := func(t *testing.T, tc testCase) {
		source := mergeStructs(t, tc.source)
		in := bytes.NewReader(toJSON(t, cogito.CheckInput{Source: source}))

		have, err := cogito.NewCheckInput(in)

		assert.NilError(t, err)
		want := mergeStructs(t, tc.want)
		assert.DeepEqual(t, have.Source, want)
	}

	baseSource := cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}

	testCases := []testCase{
		{
			name:   "only mandatory keys and defaults",
			source: []cogito.Source{baseSource},
			want:   []cogito.Source{baseSource, {LogLevel: "info"}},
		},
		{
			name:   "override defaults",
			source: []cogito.Source{baseSource, {LogLevel: "debug"}},
			want:   []cogito.Source{baseSource, {LogLevel: "debug"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

func TestNewCheckInputFailure(t *testing.T) {
	type testCase struct {
		name   string
		source cogito.Source
		want   string
	}

	test := func(t *testing.T, tc testCase) {
		in := bytes.NewReader(toJSON(t, cogito.CheckInput{Source: tc.source}))

		_, err := cogito.NewCheckInput(in)

		assert.Error(t, err, tc.want)
	}

	testCases := []testCase{
		{
			name:   "missing mandatory source keys",
			source: cogito.Source{},
			want:   "source: missing keys: owner, repo, access_token",
		},
		{
			name: "invalid log_level",
			source: cogito.Source{
				Owner:       "the-owner",
				Repo:        "the-repo",
				AccessToken: "the-token",
				LogLevel:    "pippo",
			},
			want: "source: invalid log_level: pippo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test(t, tc)
		})
	}
}

// The majority of tests for failure are done in TestNewCheckInputFailure, which limits
// the input since it uses a struct. Thus, we also test with some raw JSON input text.
func TestNewCheckInputRawFailure(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "empty input",
			input:   ``,
			wantErr: `parsing JSON from stdin: EOF`,
		},
		{
			name:    "malformed input",
			input:   `pizza`,
			wantErr: `parsing JSON from stdin: invalid character 'p' looking for beginning of value`,
		},
		{
			name: "JSON types validation is automatic (since Go is statically typed)",
			input: `
{
  "source": {
    "owner": 123
  }
}`,
			wantErr: `parsing JSON from stdin: json: cannot unmarshal number into Go struct field Source.source.owner of type string`,
		},
		{
			name: "Unknown fields are caught automatically by the JSON decoder",
			input: `
{
  "source": {
    "owner": "the-owner",
    "repo": "the-repo",
    "access_token": "the-token",
    "hello": "I am an unknown key"
  }
}`,
			wantErr: `parsing JSON from stdin: json: unknown field "hello"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(tc.input)

			_, err := cogito.NewCheckInput(in)

			assert.Error(t, err, tc.wantErr)
		})
	}
}

func TestLogRedaction(t *testing.T) {
	input := cogito.Source{
		Owner:         "the-owner",
		Repo:          "the-repo",
		AccessToken:   "sensitive-the-access-token",
		GChatWebHook:  "sensitive-gchat-webhook",
		LogLevel:      "debug",
		ContextPrefix: "the-prefix",
	}

	t.Run("fmt.Print redacts fields", func(t *testing.T) {
		want := `owner:          the-owner
repo:           the-repo
access_token:   ***REDACTED***
gchat_webhook:  ***REDACTED***
log_level:      debug
context_prefix: the-prefix`

		have := fmt.Sprint(input)

		assert.Equal(t, have, want)
	})

	t.Run("empty fields are not marked as redacted", func(t *testing.T) {
		input := cogito.Source{
			Owner: "the-owner",
		}
		want := `owner:          the-owner
repo:           
access_token:   
gchat_webhook:  
log_level:      
context_prefix: `

		have := fmt.Sprint(input)

		assert.Equal(t, have, want)
	})

	t.Run("hclog redacts fields", func(t *testing.T) {
		var logBuf bytes.Buffer
		log := hclog.New(&hclog.LoggerOptions{Output: &logBuf})

		log.Info("log test", "input", input)
		have := logBuf.String()

		assert.Assert(t, strings.Contains(have, "| access_token:   ***REDACTED***"))
		assert.Assert(t, strings.Contains(have, "| gchat_webhook:  ***REDACTED***"))
		assert.Assert(t, !strings.Contains(have, "sensitive"))
	})
}
