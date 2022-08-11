package testhelp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
)

// GhCommitStatusTestServer returns a very basic HTTP test server (mock) that speaks
// enough GitHub Commit Status API to reply successfully to a request.
// The handler will JSON decode the request body to payload and will set URL to the
// request URL.
// To avoid races, call ts.Close() before reading any parameters.
//
// Example:
//
//	var ghReq github.AddRequest
//	var URL *url.URL
//	ts := GhCommitStatusTestServer(&ghReq, &URL)
func GhCommitStatusTestServer(payload any, theUrl **url.URL) *httptest.Server {
	// In the server we cannot use t *testing.T: it runs on a different goroutine;
	// instead, we return the assert error via the HTTP protocol itself.
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			dec := json.NewDecoder(req.Body)
			if err := dec.Decode(payload); err != nil {
				w.WriteHeader(http.StatusTeapot)
				fmt.Fprintln(w, "test: parsing request:", err)
				return
			}

			*theUrl = req.URL

			w.WriteHeader(http.StatusCreated)
		}))
}
