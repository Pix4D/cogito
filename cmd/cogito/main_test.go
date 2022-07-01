package main

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestRunSmoke(t *testing.T) {
	err := run(nil, nil, []string{"foo"})

	assert.NilError(t, err)
}
