package github

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type GitHubApp struct {
	ClientId       string `json:"client_id"`
	InstallationId int    `json:"installation_id"`
	PrivateKey     string `json:"private_key"` // SENSITIVE
	parsedRSAKey   rsa.PrivateKey
}

func (app *GitHubApp) IsZero() bool {
	if app.ClientId != "" {
		return false
	}
	if app.InstallationId != 0 {
		return false
	}
	if app.PrivateKey != "" {
		return false
	}
	return true
}

// Validate validates the GitHubApp configuration. Returns an error if
// GitHubApp is misconfigured.
func (app *GitHubApp) Validate() error {
	var mandatory []string

	if app.ClientId == "" {
		mandatory = append(mandatory, "github_app.client_id")
	}
	if app.InstallationId == 0 {
		mandatory = append(mandatory, "github_app.installation_id")
	}
	if app.PrivateKey == "" {
		mandatory = append(mandatory, "github_app.private_key")
	}

	if len(mandatory) > 0 {
		return fmt.Errorf("github_app: missing mandatory keys: %s", strings.Join(mandatory, ", "))
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(app.PrivateKey))
	if err != nil {
		return fmt.Errorf("github_app: could not parse private key: %w", err)
	}
	app.parsedRSAKey = *key
	return nil
}

// generateJWTtoken returns a signed JWT token used to authenticate as GitHub App
func generateJWTtoken(clientId string, privateKey *rsa.PrivateKey) (string, error) {
	// GitHub rejects expiresAt (exp) and issuedAt (iat) timestamps that are not an integer,
	// while the jwt-go library serializes to fractional timestamps.
	// Truncate them before passing to jwt-go.
	// Additionally, GitHub recommends setting this value to 60 seconds in the past.
	issuedAt := time.Now().Add(-60 * time.Second).Truncate(time.Second)
	// Github set the maximum validity of a token to 10 minutes. Here, we reduce it to 1 minute
	// (we set expiresAt to 2 minutes, but we start 1 minute in the past).
	expiresAt := issuedAt.Add(2 * time.Minute)
	// Docs: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app#about-json-web-tokens-jwts
	claims := &jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		// The client ID or application ID of your GitHub App. Use of the client ID is recommended.
		Issuer: clientId,
	}

	// GitHub JWT must be signed using the RS256 algorithm.
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("could not sign the JWT token: %w", err)
	}
	return token, nil
}

// GenerateInstallationToken returns an installation token used to authenticate as GitHub App installation
func GenerateInstallationToken(ctx context.Context, client *http.Client, server string, app GitHubApp) (string, error) {
	// API: POST /app/installations/{installationId}/access_tokens
	installationId := strconv.Itoa(app.InstallationId)
	url := server + path.Join("/app/installations", installationId, "/access_tokens")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("github post: new request: %s", err)
	}
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	jwtToken, err := generateJWTtoken(app.ClientId, &app.parsedRSAKey)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	// FIXME: add retry here...
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http client Do: %s", err)
	}
	defer resp.Body.Close()

	body, errBody := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		if errBody != nil {
			return "", fmt.Errorf("generate github app installation token: status code: %d (%s)", resp.StatusCode, errBody)
		}
		return "", fmt.Errorf("generate github app installation token: status code: %d (%s)", resp.StatusCode, string(body))
	}
	if errBody != nil {
		return string(body), fmt.Errorf("generate github app installation token: read body: %s", errBody)
	}

	var token struct {
		Value string `json:"token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return "", fmt.Errorf("error: json unmarshal: %s", err)
	}
	return token.Value, nil
}
