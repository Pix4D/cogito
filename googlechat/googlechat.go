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
	"time"
)

// BasicMessage is the request for a Google Chat basic message.
type BasicMessage struct {
	Text string `json:"text"`
}

// MessageReply is the reply to [TextMessage].
// Compared to the full API reply, some uninteresting fields are removed.
type MessageReply struct {
	Name       string        `json:"name"` // Absolute message ID.
	Sender     MessageSender `json:"sender"`
	Text       string        `json:"text"` // The message text, as sent.
	Thread     MessageThread `json:"thread"`
	Space      MessageSpace  `json:"space"`
	CreateTime time.Time     `json:"createTime"`
}

// MessageSender is part of [MessageReply].
// Compared to the full API reply, some uninteresting fields are removed.
type MessageSender struct {
	Name        string `json:"name"`        // Absolute user ID.
	DisplayName string `json:"displayName"` // Name of the webhook in the UI.
	Type        string `json:"type"`        // "BOT", ...
}

// MessageThread is part of [MessageReply].
// Compared to the full API reply, some uninteresting fields are removed.
type MessageThread struct {
	Name string `json:"name"` // Absolute thread ID.
}

// MessageSpace is part of [MessageReply].
// Compared to the full API reply, some uninteresting fields are removed.
type MessageSpace struct {
	Name        string `json:"name"`        // Absolute space ID.
	Type        string `json:"type"`        // "ROOM", ...
	Threaded    bool   `json:"threaded"`    // Has the space been created as "threaded"?
	DisplayName string `json:"displayName"` // Name of the space in the UI.
}

// TextMessage sends the one-off message `text` with `threadKey` to webhook `theURL` and
// returns an abridged response.
//
// Note that the Google Chat API encodes the secret in the webhook itself.
//
// Implementation note: if instead we need to send multiple messages, we should reuse the
// http.Client, so we should add another API function to do so.
//
// References:
// REST Resource: v1.spaces.messages
// https://developers.google.com/chat/api/reference/rest
// webhooks: https://developers.google.com/chat/how-tos/webhooks
// payload: https://developers.google.com/chat/api/guides/message-formats/basic
// threadKey: https://developers.google.com/chat/reference/rest/v1/spaces.messages/create
func TextMessage(ctx context.Context, theURL, threadKey, text string) (MessageReply, error) {
	body, err := json.Marshal(BasicMessage{Text: text})
	if err != nil {
		return MessageReply{}, fmt.Errorf("TextMessage: %s", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, theURL,
		bytes.NewBuffer(body))
	if err != nil {
		return MessageReply{},
			fmt.Errorf("TextMessage: new request: %w", RedactErrorURL(err))
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
		return MessageReply{}, fmt.Errorf("TextMessage: send: %s", RedactErrorURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return MessageReply{},
			fmt.Errorf("TextMessage: status: %s; URL: %s; body: %s",
				resp.Status, RedactURL(req.URL), strings.TrimSpace(string(respBody)))
	}

	var reply MessageReply
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&reply); err != nil {
		return MessageReply{},
			fmt.Errorf("HTTP status OK but failed to parse response: %s", err)
	}

	return reply, nil
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
