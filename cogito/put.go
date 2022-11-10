package cogito

import (
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/go-hclog"
)

// Putter represents the put step of a Concourse resource.
// Note: The methods will be called in the same order as they are listed here.
type Putter interface {
	// LoadConfiguration parses the resource source configuration and put params.
	LoadConfiguration(input []byte, args []string) error
	// ProcessInputDir validates and extract the needed information from the "put input".
	ProcessInputDir() error
	// Sinks return the list of configured sinks. It also validates that they are supported.
	Sinks() ([]Sinker, error)
	// Output emits the version and metadata required by the Concourse protocol.
	Output(out io.Writer) error
}

// Sinker represents a sink: an endpoint to send a message.
type Sinker interface {
	// Send posts the information extracted by the Putter to a specific sink.
	Send() error
}

// Put implements the "put" step (the "out" executable).
//
// From https://concourse-ci.org/implementing-resource-types.html#resource-out:
//
// The out script is passed a path to the directory containing the build's full set of
// inputs as command line argument $1, and is given on stdin the configured params and
// the resource's source configuration.
//
// The script must emit the resulting version of the resource.
//
// Additionally, the script may emit metadata as a list of key-value pairs. This data is
// intended for public consumption and will make it upstream, intended to be shown on the
// build's page.
func Put(log hclog.Logger, input []byte, out io.Writer, args []string, putter Putter) error {
	if err := putter.LoadConfiguration(input, args); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	if err := putter.ProcessInputDir(); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	// We invoke all the sinks and keep going also if some of them return an error.
	var sinkErrors []error
	sinks, err := putter.Sinks()
	if err != nil {
		return fmt.Errorf("put: %s", err)
	}
	for _, sink := range sinks {
		if err := sink.Send(); err != nil {
			sinkErrors = append(sinkErrors, err)
		}
	}
	if len(sinkErrors) > 0 {
		return fmt.Errorf("put: %s", multiErrString(sinkErrors))
	}

	if err := putter.Output(out); err != nil {
		return fmt.Errorf("put: %s", err)
	}

	return nil
}

// multiErrString takes a slice of errors and returns a formatted string.
func multiErrString(errs []error) string {
	if len(errs) == 1 {
		return errs[0].Error()
	}
	bld := new(strings.Builder)
	bld.WriteString("multiple errors:")
	for _, err := range errs {
		bld.WriteString("\n\t")
		bld.WriteString(err.Error())
	}
	return bld.String()
}
