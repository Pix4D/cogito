package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Pix4D/cogito/github"
	"github.com/Pix4D/cogito/testhelp"
	"gotest.tools/v3/assert"
)

func TestGenerateInstallationToken(t *testing.T) {
	clientID := "abcd1234"
	installationID := 12345

	privateKey, err := testhelp.GeneratePrivateKey(t, 2048)
	assert.NilError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	app := github.GitHubApp{
		ClientId:       clientID,
		InstallationId: installationID,
		PrivateKey:     testhelp.EncodePrivateKeyToPEM(privateKey),
	}
	err = app.Validate()
	assert.NilError(t, err)

	gotToken, err := github.GenerateInstallationToken(
		ctx,
		ts.Client(),
		ts.URL,
		app,
	)

	assert.NilError(t, err)
	assert.Equal(t, "dummy_installation_token", gotToken)
}

func TestGitHubAppIsZero(t *testing.T) {
	type testCase struct {
		name string
		app  github.GitHubApp
		want bool
	}

	run := func(t *testing.T, tc testCase) {
		got := tc.app.IsZero()
		assert.Equal(t, got, tc.want)
	}

	testCases := []testCase{
		{
			name: "empty app",
			app:  github.GitHubApp{},
			want: true,
		},
		{
			name: "one field set: client-id",
			app:  github.GitHubApp{ClientId: "client-id"},
			want: false,
		},
		{
			name: "all fields set",
			app: github.GitHubApp{
				ClientId:       "client-id",
				InstallationId: 12345,
				PrivateKey:     "dummy-private-key",
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}

func TestGitHubAppValidateSuccess(t *testing.T) {
	privateKey, err := testhelp.GeneratePrivateKey(t, 2048)
	assert.NilError(t, err)

	app := github.GitHubApp{
		ClientId:       "client-id",
		InstallationId: 12345,
		PrivateKey:     testhelp.EncodePrivateKeyToPEM(privateKey),
	}

	err = app.Validate()
	assert.NilError(t, err)
}

func TestGitHubAppValidateFailure(t *testing.T) {
	type testCase struct {
		name      string
		app       github.GitHubApp
		wantError string
	}

	run := func(t *testing.T, tc testCase) {
		got := tc.app.Validate()
		assert.ErrorContains(t, got, tc.wantError)
	}

	testCases := []testCase{
		{
			name:      "one field set: client-id",
			app:       github.GitHubApp{ClientId: "client-id"},
			wantError: "github_app: missing mandatory keys: github_app.installation_id, github_app.private_key",
		},
		{
			name:      "missing single field set: private_key",
			app:       github.GitHubApp{ClientId: "client-id", InstallationId: 12345},
			wantError: "github_app: missing mandatory keys: github_app.private_key",
		},
		{
			name: "all fields set: invalid pem key",
			app: github.GitHubApp{
				ClientId:       "client-id",
				InstallationId: 12345,
				PrivateKey:     "dummy-private-key",
			},
			wantError: "github_app: could not parse private key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { run(t, tc) })
	}
}
