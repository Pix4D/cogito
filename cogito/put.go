package cogito

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

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

	// args[0] contains the path to a directory containing all the "put inputs".
	if len(args) == 0 {
		return fmt.Errorf("put: arguments: missing input directory")
	}
	inputDir := args[0]
	log.Debug("", "input-directory", inputDir)

	buildState := pi.Params.State
	if err := buildState.Validate(); err != nil {
		return fmt.Errorf("put: params: %s", err)
	}
	log.Debug("", "state", buildState)

	if err := validateInputDir(inputDir, pi.Source.Owner, pi.Source.Repo); err != nil {
		return fmt.Errorf("put: validating the input dir: %s", err)
	}

	// Following the protocol for put, we return the version and metadata.
	// For Cogito, the metadata contains the Concourse build state.
	output := Output{
		Version:  DummyVersion,
		Metadata: []Metadata{{Name: KeyState, Value: string(buildState)}},
	}
	enc := json.NewEncoder(out)
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	log.Debug("success", "output", output)
	return nil
}

// validateInputDir checks whether dir, the "put input", conforms to what we expect.
func validateInputDir(dir string, owner string, repo string) error {
	inputDirs, err := collectInputDirs(dir)
	if err != nil {
		return err
	}
	if len(inputDirs) != 1 {
		return fmt.Errorf(
			"found %d input dirs: %v. Want exactly 1, corresponding to the GitHub repo %s/%s",
			len(inputDirs), inputDirs, owner, repo)
	}

	return nil
}

// collectInputDirs returns a list of all directories below dir (non-recursive).
func collectInputDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("collecting directories in %v: %w", dir, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}
