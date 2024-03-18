package testhelp

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// MakeTestLog returns a *slog.Logger adapted for tests: it never reports the
// timestamp and by default it discards all the output. If on the other hand
// the tests are invoked in verbose mode (go test -v), then the logger will
// log normally.
func MakeTestLog() *slog.Logger {
	out := io.Discard
	if testing.Verbose() {
		out = os.Stdout
	}
	return slog.New(slog.NewTextHandler(
		out,
		&slog.HandlerOptions{
			ReplaceAttr: RemoveTime,
		}))
}

// RemoveTime removes the "time" attribute from the output of a slog.Logger.
func RemoveTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}
