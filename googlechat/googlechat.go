// Package googlechat implements the Google Chat API used by Cogito.
//
// See the README and CONTRIBUTING files for additional information and reference to
// official documentation.
package googlechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// BasicMessage represents the JSON payload of a Google Chat basic message.
type BasicMessage struct {
	Text string `json:"text"`
}

// TextMessage sends a one-off text message with threadKey to webhook theURL.
// Note that the Google Chat API encodes the secret in the webhook itself.
//
// If instead we need to send multiple messages, we should reuse the http.Client,
// so we should add another API function to do so.
//
// References:
// webhooks: https://developers.google.com/chat/how-tos/webhooks
// payload: https://developers.google.com/chat/api/guides/message-formats/basic
// threadKey: https://developers.google.com/chat/reference/rest/v1/spaces.messages/create
func TextMessage(ctx context.Context, theURL string, threadKey string, text string) error {
	body, err := json.Marshal(BasicMessage{Text: text})
	if err != nil {
		return fmt.Errorf("TextMessage: %s", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, theURL,
		bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("TextMessage: new request: %w", RedactErrorURL(err))
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	// Encode the thread Key a URL parameter.
	if threadKey != "" {
		values := req.URL.Query()
		values.Set("threadKey", threadKey)
		req.URL.RawQuery = values.Encode()
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("TextMessage: send: %s", RedactErrorURL(err))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TextMessage: status: %s; URL: %s; body: %s",
			resp.Status, RedactURL(req.URL), strings.TrimSpace(string(respBody)))
	}

	return nil
}

// RedactURL returns a _best effort_ redacted copy of theURL.
//
// Use this workaround only when you are forced to use an API that encodes
// secrets in the URL instead of setting them in the request header.
// If you have control of the API, please never encode secrets in the URL.
//
// Redaction is applied as follows:
// - removal of all query parameters
// - removal of "username:password@" HTTP Basic Authentication
//
// Warning: it is still possible that the redacted URL contains secrets, for
// example if the secret is encoded in the path. Don't do this.
//
// Taken from https://github.com/marco-m/lanterna
func RedactURL(theURL *url.URL) *url.URL {
	copy := *theURL

	// remove all query parameters
	if copy.RawQuery != "" {
		copy.RawQuery = "REDACTED"
	}
	// remove password in user:password@host
	if _, ok := copy.User.Password(); ok {
		copy.User = url.UserPassword("REDACTED", "REDACTED")
	}

	return &copy
}

// RedactErrorURL returns a _best effort_ redacted copy of err. See
// RedactURL for caveats and limitations.
// In case err is not of type url.Error, then it returns the error untouched.
//
// Taken from https://github.com/marco-m/lanterna
func RedactErrorURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		urlErr.URL = RedactURLString(urlErr.URL)
		return urlErr
	}
	return err
}

// RedactURLString returns a _best effort_ redacted copy of theURL. See
// RedactURL for caveats and limitations.
// In case theURL cannot be parsed, then return the parse error string.
//
// Taken from https://github.com/marco-m/lanterna
func RedactURLString(theURL string) string {
	urlo, err := url.Parse(theURL)
	if err != nil {
		return err.Error()
	}
	return RedactURL(urlo).String()
}
