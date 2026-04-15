// The three executables (check, in, out) are symlinked to this file.
// For statically linked binaries like Go, this reduces the size by 2/3.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"

	"github.com/Pix4D/cogito/cogito"
	"github.com/Pix4D/go-kit/sets"
)

func main() {
	if err := mainErr(os.Stdin, os.Stdout, os.Stderr, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "cogito: error: %s\n", err)
		os.Exit(1)
	}
}

// The "Concourse resource protocol" expects:
//   - stdin, stdout and command-line arguments for the protocol itself
//   - stderr for logging
//
// See: https://concourse-ci.org/implementing-resource-types.html
func mainErr(stdin io.Reader, stdout io.Writer, stderr io.Writer, args []string) error {
	cmd := path.Base(args[0])
	validCmds := sets.From("check", "in", "out")
	if !validCmds.Contains(cmd) {
		return fmt.Errorf("invoked as '%s'; want: one of %v", cmd, validCmds)
	}

	input, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %s", err)
	}

	logLevel, err := peekLogLevel(input)
	if err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return fmt.Errorf("%s. (valid: debug, info, warn, error)", err)
	}
	log := makeProdLogger(stderr, level)
	log.Info(cogito.BuildInfo())

	switch cmd {
	case "check":
		return cogito.Check(log, input, stdout, args[1:])
	case "in":
		return cogito.Get(log, input, stdout, args[1:])
	case "out":
		putter := cogito.NewPutter(log)
		return cogito.Put(log, input, stdout, args[1:], putter)
	default:
		return fmt.Errorf("cli wiring error; please report")
	}
}

// Strings constants for maximum performance.
const (
	logDBG     = "DBG"
	logINF     = "INF"
	logWRN     = "WRN"
	logERR     = "ERR"
	logUNKNOWN = "UNKNOWN"
)

func makeProdLogger(stderr io.Writer, level slog.Level) *slog.Logger {
	replaceAttrs := func(groups []string, a slog.Attr) slog.Attr {
		// Formatting time.
		// One can change the timezone (default: local), but the format is fixed:
		// RFC3339 (same as ISO 8601) with millisecond precision.
		// To change the format, the Attr.Value must implement encoding.TextMarshaler.
		// See: https://pkg.go.dev/log/slog#TextHandler.Handle
		// if a.Key == slog.TimeKey {
		// 	// This has no effect, see explanation above.
		// 	a.Value = slog.TimeValue(a.Value.Time().Round(time.Second))
		// 	return a
		// }

		// Format log levels to have all the same length.
		if a.Key == slog.LevelKey {
			level := a.Value.Any().(slog.Level)
			switch level {
			case slog.LevelDebug:
				a.Value = slog.StringValue(logDBG)
			case slog.LevelInfo:
				a.Value = slog.StringValue(logINF)
			case slog.LevelWarn:
				a.Value = slog.StringValue(logWRN)
			case slog.LevelError:
				a.Value = slog.StringValue(logERR)
			default:
				a.Value = slog.StringValue(logUNKNOWN)
			}
			return a
		}

		return a
	}

	return slog.New(slog.NewTextHandler(
		stderr,
		&slog.HandlerOptions{
			Level:       level,
			ReplaceAttr: replaceAttrs,
		}))
}

// peekLogLevel decodes 'input' as JSON and looks for key source.log_level. If 'input'
// is not JSON, peekLogLevel will return an error. If 'input' is JSON but does not
// contain key source.log_level, peekLogLevel returns "info" as default value.
//
// Rationale: depending on the Concourse step we are invoked for, the JSON object we get
// from stdin is different, but it always contains a struct with name "source", thus we
// can peek into it to gather the log level as soon as possible.
func peekLogLevel(input []byte) (string, error) {
	type Peek struct {
		Source struct {
			LogLevel string `json:"log_level"`
		} `json:"source"`
	}
	var peek Peek
	peek.Source.LogLevel = "info" // default value
	if err := json.Unmarshal(input, &peek); err != nil {
		return "", fmt.Errorf("peeking into JSON for log_level: %s", err)
	}

	return peek.Source.LogLevel, nil
}
