// The three executables (check, in, out) are symlinked to this file.
// For statically linked binaries like Go, this reduces the size by 2/3.
package main

import (
	"fmt"
	"io"
	"os"
	"path"
)

func main() {
	// The "Concourse resource protocol" expects:
	// - stdin, stdout and command-line arguments for the protocol itself
	// - stderr for logging
	// See: https://concourse-ci.org/implementing-resource-types.html
	if err := run(os.Stdin, os.Stdout, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer, args []string) error {
	command := path.Base(args[0])
	fmt.Fprintln(os.Stderr, "invoked as:", command)
	return nil
}
