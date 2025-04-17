package github_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
	"gotest.tools/v3/assert"
)

func TestGenerateInstallationToken(t *testing.T) {
	clientID := "abcd1234"
	var installationID int64 = 12345

	privateKey, err := testhelp.GeneratePrivateKey(t, 2048)
	assert.NilError(t, err)

	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintln(w, "wrong HTTP method")
			return
		}

		claims := testhelp.DecodeJWT(t, r, privateKey)
		if claims.Issuer != clientID {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "unauthorized: wrong JWT token")
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"token": "dummy_installation_token"}`)
	}

	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	gotToken, err := github.GenerateInstallationToken(
		ts.URL,
		github.GitHubApp{
			ClientId:       clientID,
			InstallationId: installationID,
			PrivateKey:     string(testhelp.EncodePrivateKeyToPEM(privateKey)),
		},
	)

	assert.NilError(t, err)
	assert.Equal(t, "dummy_installation_token", gotToken)
}
