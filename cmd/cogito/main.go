// The three executables (check, in, out) are symlinked to this file.
// For statically linked binaries like Go, this reduces the size by 2/3.
package main

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/cogito/github"
	"github.com/hashicorp/go-hclog"
)

func main() {
	// The "Concourse resource protocol" expects:
	// - stdin, stdout and command-line arguments for the protocol itself
	// - stderr for logging
	// See: https://concourse-ci.org/implementing-resource-types.html
	if err := run(os.Stdin, os.Stdout, os.Stderr, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer, logOut io.Writer, args []string) error {
	log := hclog.New(&hclog.LoggerOptions{
		Name:        "cogito",
		Output:      logOut,
		DisableTime: true,
	})
	log.Info(cogito.BuildInfo())

	command := path.Base(args[0])
	switch command {
	case "check":
		return cogito.Check(log, in, out, args[1:])
	case "in":
		return cogito.Get(log, in, out, args[1:])
	case "out":
		putter := cogito.NewPutter(github.API, log)
		return cogito.Put(log, in, out, args[1:], putter)
	default:
		return fmt.Errorf(
			"cogito: unexpected invocation as '%s'; want: one of 'check', 'in', 'out'; (command-line: %s)",
			command, args)
	}
}
