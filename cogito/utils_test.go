// This file contains test utilities.

package cogito_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/imdario/mergo"
	"gotest.tools/v3/assert"
)

// toJSON returns the JSON encoding of thing.
func toJSON(t *testing.T, thing any) []byte {
	t.Helper()
	buf, err := json.Marshal(thing)
	assert.NilError(t, err)
	return buf
}

// fromJSON unmarshals the JSON-encoded data into thing.
func fromJSON(t *testing.T, data []byte, thing any) {
	t.Helper()
	err := json.Unmarshal(data, thing)
	assert.NilError(t, err)
}

// mergeStructs merges the second element of s into the first one and returns the
// merged copy. If s has only one element, mergeStructs returns it unmodified.
// Used to express succinctly the delta in the test cases.
func mergeStructs[E any](t *testing.T, s []E) E {
	t.Helper()
	assert.Assert(t, len(s) == 1 || len(s) == 2, len(s))
	want := s[0]
	if len(s) == 2 {
		err := mergo.Merge(&want, s[1], mergo.WithOverride)
		assert.NilError(t, err)
	}
	return want
}

// failingWriter is an io.Writer that always returns an error.
type failingWriter struct{}

func (t *failingWriter) Write([]byte) (n int, err error) {
	return 0, errors.New("test write error")
}
