package cogito

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/go-hclog"
)

// Put implements the "put" step (the "out" executable).
//
// From https://concourse-ci.org/implementing-resource-types.html#resource-out:
//
// The out script is passed a path to the directory containing the build's full set of
// sources as command line argument $1, and is given on stdin the configured params and
// the resource's source configuration.
//
// The script must emit the resulting version of the resource.
//
// Additionally, the script may emit metadata as a list of key-value pairs. This data is
// intended for public consumption and will make it upstream, intended to be shown on the
// build's page.
func Put(log hclog.Logger, in io.Reader, out io.Writer, args []string) error {
	var pi PutInput
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pi); err != nil {
		return fmt.Errorf("put: parsing JSON from stdin: %s", err)
	}
	pi.Env.Fill()

	if err := pi.Source.ValidateLog(); err != nil {
		return fmt.Errorf("put: %s", err)
	}
	log = log.Named("put")
	log.SetLevel(hclog.LevelFromString(pi.Source.LogLevel))

	log.Debug("started",
		"source", pi.Source,
		"params", pi.Params,
		"environment", pi.Env,
		"args", args)

	if err := pi.Source.Validate(); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	// args[0] contains the path to a Concourse volume containing all the resource
	// "put inputs".
	if len(args) == 0 {
		return fmt.Errorf("put: missing input directory from arguments")
	}
	inputDir := args[0]
	log.Debug("", "input-directory", inputDir)

	// Following the protocol for put, we return a dummy version and metadata.
	output := Output{
		// TODO Version: constant dummy
		// TODO Metadata
	}
	enc := json.NewEncoder(out)
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	log.Debug("success", "output", output)
	return nil
}
