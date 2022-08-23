package testhelp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
)

// SpyHttpServer returns a very basic HTTP test server (spy).
// The handler will JSON decode the request body into `request` and will set `theUrl`
// to the request URL.
// On JSON decode success, the handler will return to the client the HTTP status code
// `successCode`. On JSON decode failure, the handler will return 418 I am a teapot.
// If `reply` is not nil, the handler will send it to the client, JSON encoded.
// To avoid races, call ts.Close() before reading any parameters.
//
// Example:
//
//	var ghReq github.AddRequest
//	var URL *url.URL
//	ts := SpyHttpServer(&ghReq, nil, &URL, http.StatusCreated)
func SpyHttpServer(request any, reply any, theUrl **url.URL, successCode int,
) *httptest.Server {
	// In the server we cannot use t *testing.T: it runs on a different goroutine;
	// instead, we return the assert error via the HTTP protocol itself.
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			dec := json.NewDecoder(req.Body)
			if err := dec.Decode(request); err != nil {
				w.WriteHeader(http.StatusTeapot)
				fmt.Fprintln(w, "test: decoding request:", err)
				return
			}

			if reply != nil {
				enc := json.NewEncoder(w)
				if err := enc.Encode(reply); err != nil {
					w.WriteHeader(http.StatusTeapot)
					fmt.Fprintln(w, "test: encoding response:", err)
					return
				}
			}

			*theUrl = req.URL

			w.WriteHeader(successCode)
		}))
}
