package cogito

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/go-hclog"
)

// Get implements the "get" step (the "in" executable).
// For the Cogito resource, this is a no-op.
//
// From https://concourse-ci.org/implementing-resource-types.html#resource-in:
//
// The program is passed a destination directory as command line argument $1, and is
// given on stdin the configured source and a version of the resource to fetch.
//
// The program must fetch the resource and place it in the given directory.
//
// If the desired resource version is unavailable (for example, if it was deleted), the
// script must exit with error.
//
// The program must emit a JSON object containing the fetched version, and may emit
// metadata as a list of key-value pairs.
// This data is intended for public consumption and will be shown on the build page.
func Get(log hclog.Logger, in io.Reader, out io.Writer, args []string) error {
	var request GetRequest
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		return fmt.Errorf("get: parsing request: %s", err)
	}
	request.Env.Fill()

	if err := request.Source.ValidateLog(); err != nil {
		return fmt.Errorf("get: %s", err)
	}
	log = log.Named("get")
	log.SetLevel(hclog.LevelFromString(request.Source.LogLevel))

	log.Debug("started",
		"source", request.Source,
		"version", request.Version,
		"environment", request.Env,
		"args", args)

	if err := request.Source.Validate(); err != nil {
		return fmt.Errorf("get: %s", err)
	}

	if request.Version.Ref == "" {
		return fmt.Errorf("get: empty 'version' field")
	}

	// args[0] contains the path to a Concourse volume and a normal resource would fetch
	// and put there the requested version of the resource.
	// In this resource we do nothing, but we still check for protocol conformance.
	if len(args) == 0 {
		return fmt.Errorf("get: arguments: missing output directory")
	}
	log.Debug("", "output-directory", args[0])

	// Following the protocol for get, we return the same version as the requested one.
	output := Output{Version: request.Version}
	enc := json.NewEncoder(out)
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("get: %s", err)
	}

	log.Debug("success", "output", output)
	return nil
}
