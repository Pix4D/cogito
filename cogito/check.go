package cogito

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/go-hclog"
)

// Check implements the "check" step (the "check" executable).
//
// From https://concourse-ci.org/implementing-resource-types.html#resource-check:
//
// A resource type's check script is invoked to detect new versions of the resource.
// It is given the configured source and current version on stdin, and must print the
// array of new versions, in chronological order (oldest first), to stdout, including
// the requested version if it is still valid.
//
func Check(log hclog.Logger, in io.Reader, out io.Writer, args []string) error {
	ci, err := NewCheckInput(in)
	if err != nil {
		return fmt.Errorf("check: %s", err)
	}

	log = log.Named("check")
	log.SetLevel(hclog.LevelFromString(ci.Source.LogLevel))
	log.Debug("started",
		"source", ci.Source, "version", ci.Version, "environment", ci.Env, "args", args)
	defer log.Debug("finished")

	// Since there is no meaningful real version for this resource, we return always the
	// same dummy version (we need to return a non-empty list to keep Concourse happy).
	versions := []Version{{Ref: "dummy"}}
	enc := json.NewEncoder(out)
	if err := enc.Encode(versions); err != nil {
		return err
	}

	log.Debug("success", "output", versions)
	return nil
}
