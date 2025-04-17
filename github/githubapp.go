package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type GitHubApp struct {
	ClientId       string `json:"client_id"`
	InstallationId int64  `json:"installation_id"`
	PrivateKey     string `json:"private_key"` // SENSITIVE
}

// generateJWTtoken returns a signed JWT token used to authenticate as GitHub App
func generateJWTtoken(clientId, privateKey string) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKey))
	if err != nil {
		return "", fmt.Errorf("could not parse private key: %w", err)
	}
	// GitHub rejects expiry and issue timestamps that are not an integer,
	// while the jwt-go library serializes to fractional timestamps.
	// Truncate them before passing to jwt-go.
	// Additionally, GitHub recommends setting this value 60 seconds in the past.
	iat := time.Now().Add(-60 * time.Second).Truncate(time.Second)
	// maximum validity 10 minutes. Here, we reduce it to 2 minutes.
	exp := iat.Add(2 * time.Minute)
	// Docs: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app#about-json-web-tokens-jwts
	claims := &jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(iat),
		ExpiresAt: jwt.NewNumericDate(exp),
		// The client ID or application ID of your GitHub App.
		// Use of the client ID is recommended.
		Issuer: clientId,
	}

	// GitHub JWT must be signed using the RS256 algorithm.
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		return "", fmt.Errorf("could not sign the JWT token: %w", err)
	}
	return token, nil
}

// GenerateInstallationToken returns an installation token used to authenticate as GitHub App installation
func GenerateInstallationToken(server string, app GitHubApp) (string, error) {
	// API: POST /app/installations/{installationId}/access_tokens
	installationId := strconv.FormatInt(app.InstallationId, 10)
	url := server + path.Join("/app/installations", installationId, "/access_tokens")

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("github post: new request: %s", err)
	}
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	jtwToken, err := generateJWTtoken(app.ClientId, app.PrivateKey)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jtwToken)

	client := &http.Client{Timeout: time.Second * 5}

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
