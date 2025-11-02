package githubapp

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// GitHubAPIBaseURL is the GitHub API base URL
	GitHubAPIBaseURL = "https://api.github.com"

	// TokenExpiration is how long JWT tokens are valid
	TokenExpiration = 10 * time.Minute
)

// App represents a GitHub App
type App struct {
	appID      int64
	privateKey *rsa.PrivateKey
	httpClient *http.Client
}

// Installation represents a GitHub App installation
type Installation struct {
	ID     int64  `json:"id"`
	Token  string `json:"-"` // Access token (not from API)
	Expiry time.Time
}

// NewApp creates a new GitHub App client
func NewApp(appID int64, privateKey *rsa.PrivateKey) *App {
	return &App{
		appID:      appID,
		privateKey: privateKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// generateJWT creates a JWT for authenticating as the GitHub App
func (a *App) generateJWT() (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenExpiration)),
		Issuer:    strconv.FormatInt(a.appID, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(a.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

// GetInstallation gets installation details
func (a *App) GetInstallation(ctx context.Context, installationID int64) (*Installation, error) {
	jwtToken, err := a.generateJWT()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/app/installations/%d", GitHubAPIBaseURL, installationID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var installation Installation
	if err := json.NewDecoder(resp.Body).Decode(&installation); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &installation, nil
}

// CreateInstallationToken creates an access token for an installation
func (a *App) CreateInstallationToken(ctx context.Context, installationID int64) (string, error) {
	jwtToken, err := a.generateJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", GitHubAPIBaseURL, installationID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return tokenResp.Token, nil
}

// ListInstallations lists all installations of this GitHub App
func (a *App) ListInstallations(ctx context.Context) ([]Installation, error) {
	jwtToken, err := a.generateJWT()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/app/installations", GitHubAPIBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var installations []Installation
	if err := json.NewDecoder(resp.Body).Decode(&installations); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return installations, nil
}

// GetRepositories gets all repositories accessible to an installation
func (a *App) GetRepositories(ctx context.Context, installationToken string) ([]Repository, error) {
	url := fmt.Sprintf("%s/installation/repositories", GitHubAPIBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+installationToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var repoResp struct {
		Repositories []Repository `json:"repositories"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&repoResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return repoResp.Repositories, nil
}

// Repository represents a GitHub repository
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
}
