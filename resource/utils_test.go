package resource

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRedact(t *testing.T) {
	secrets := map[string]struct{}{
		"redactme!": {},
	}

	testCases := []struct {
		name  string
		dirty map[string]any
		want  map[string]any
	}{
		{
			name: "redact one entry",
			dirty: map[string]any{
				"redactme!": "the secret",
				"mango":     42,
			},
			want: map[string]any{
				"redactme!": "REDACTED",
				"mango":     42,
			},
		},
		{
			name: "cannot redact: value to redact is not a string",
			dirty: map[string]any{
				"redactme!": 1,
			},
			want: map[string]any{
				"redactme!": 1,
			},
		},
		{
			name: "empty string is not redacted",
			dirty: map[string]any{
				"redactme!": "",
			},
			want: map[string]any{
				"redactme!": "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			have := redact(tc.dirty, secrets)

			if diff := cmp.Diff(tc.want, have); diff != "" {
				t.Fatalf("redact: (-want +have):\n%s", diff)
			}
		})
	}
}
