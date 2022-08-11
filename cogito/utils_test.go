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

// mergeStructs merges b into a and returns the merged copy.
// Used to express succinctly the delta in the test cases.
// Since it is a test helper, it will panic in case of error.
func mergeStructs[T any](a, b T) T {
	if err := mergo.Merge(&a, b, mergo.WithOverride); err != nil {
		panic(err)
	}
	return a
}

// failingWriter is an io.Writer that always returns an error.
type failingWriter struct{}

func (t *failingWriter) Write([]byte) (n int, err error) {
	return 0, errors.New("test write error")
}
