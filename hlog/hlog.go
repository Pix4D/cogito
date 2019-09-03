// This is a quick and dirty debugging aid for the `check` of a resource, since Concourse doesn't
// report to the user any logging in `check` :-(
// It should evolve to a more generic API.

package hlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// FIXME Missing log level filtering, will always log for the time being.
func Errorf(logUrl string, format string, a ...interface{}) {
	if logUrl == "" {
		return
	}
	format = "ERR " + format
	send(logUrl, fmt.Sprintf(format, a...))
}

// FIXME Missing log level filtering, will always log for the time being.
func Warnf(logUrl string, format string, a ...interface{}) {
	if logUrl == "" {
		return
	}
	format = "WRN " + format
	send(logUrl, fmt.Sprintf(format, a...))
}

// FIXME Missing log level filtering, will always log for the time being.
func Infof(logUrl string, format string, a ...interface{}) {
	if logUrl == "" {
		return
	}
	format = "INF " + format
	send(logUrl, fmt.Sprintf(format, a...))
}

// FIXME Missing log level filtering, will always log for the time being.
func Debugf(logUrl string, format string, a ...interface{}) {
	if logUrl == "" {
		return
	}
	format = "DBG " + format
	send(logUrl, fmt.Sprintf(format, a...))
}

// send sends a message to the logUrl URL via HTTP POST.
// Currently only Hangouts Chat is supported.
func send(logUrl string, text string) error {
	type Msg struct {
		Text string `json:"text"`
	}
	msg := Msg{Text: text}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(msg)
	var err error
	resp, err := http.Post(logUrl, "application/json; charset=UTF-8", b)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("Message: HTTP %v", resp.StatusCode)
	}
	return err
}
