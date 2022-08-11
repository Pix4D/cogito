package testhelp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
)

// SpyHttpServer returns a very basic HTTP test server (spy).
// The handler will JSON decode the request body to payload and will set theUrl to the
// request URL. On JSON decode success, the handler will return the HTTP status code
// successCode. On JSON decode failure, the handler will return 418 I am a teapot.
// To avoid races, call ts.Close() before reading any parameters.
//
// Example:
//
//	var ghReq github.AddRequest
//	var URL *url.URL
//	ts := SpyHttpServer(&ghReq, &URL, http.StatusCreated)
func SpyHttpServer(payload any, theUrl **url.URL, successCode int) *httptest.Server {
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

			w.WriteHeader(successCode)
		}))
}
