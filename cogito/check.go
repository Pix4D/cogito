package cogito

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/go-hclog"
)

// Check implements the "check" step (the "check" executable).
// For the Cogito resource, this is a no-op.
//
// From https://concourse-ci.org/implementing-resource-types.html#resource-check:
//
// A resource type's check script is invoked to detect new versions of the resource.
// It is given the configured source and current version on stdin, and must print the
// array of new versions, in chronological order (oldest first), to stdout, including
// the requested version if it is still valid.
func Check(log hclog.Logger, input []byte, out io.Writer, args []string) error {
	log = log.Named("check")
	log.Debug("started")
	defer log.Debug("finished")

	request, err := NewCheckRequest(input)
	if err != nil {
		return err
	}
	log.Debug("parsed check request",
		"source", request.Source,
		"version", request.Version,
		"environment", request.Env,
		"args", args)

	// We don't validate the presence of field request.Version because Concourse will
	// omit it from the _first_ request of the check step.

	// Here a normal resource would fetch a list of the latest versions.
	// In this resource, we do nothing.

	// Since there is no meaningful real version for this resource, we return always the
	// same dummy version.
	// NOTE I _think_ that when I initially wrote this, the JSON array of the versions
	// could not be empty. Now (2022-07) it seems that it could indeed be empty.
	// For the time being we keep it as-is because this maintains the previous behavior.
	// This will be investigated by PCI-2617.
	versions := []Version{DummyVersion}
	enc := json.NewEncoder(out)
	if err := enc.Encode(versions); err != nil {
		return fmt.Errorf("check: preparing output: %s", err)
	}

	log.Debug("success", "output.version", versions)
	return nil
}
