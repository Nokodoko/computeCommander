package linkedin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// API client for LinkedIn's v2 REST API.
// Handles OAuth2 token management and post publishing.

// OAuthConfig holds LinkedIn OAuth2 application credentials.
type OAuthConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
}

// OAuthToken holds the current access/refresh token pair.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	// LinkedIn member URN (e.g., "urn:li:person:XXXXX")
	MemberURN string `json:"member_urn,omitempty"`
}

// APIClient manages LinkedIn API interactions.
type APIClient struct {
	config     OAuthConfig
	token      *OAuthToken
	httpClient *http.Client
	configDir  string
}

// DefaultConfigDir returns the config directory for LinkedIn credentials.
func DefaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "computecommander", "linkedin")
}

// NewAPIClient creates a LinkedIn API client.
// It loads credentials and tokens from the config directory.
func NewAPIClient() (*APIClient, error) {
	configDir := DefaultConfigDir()

	client := &APIClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		configDir:  configDir,
	}

	// Load OAuth config.
	if err := client.loadConfig(); err != nil {
		return nil, fmt.Errorf("load linkedin config: %w\nRun: cmdr linkedin setup", err)
	}

	// Load token if available.
	_ = client.loadToken() // Non-fatal: token may not exist yet.

	return client, nil
}

// loadConfig reads the OAuth2 client credentials.
func (c *APIClient) loadConfig() error {
	path := filepath.Join(c.configDir, "oauth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return json.Unmarshal(data, &c.config)
}

// loadToken reads the saved access/refresh token.
func (c *APIClient) loadToken() error {
	path := filepath.Join(c.configDir, "token.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &c.token)
}

// saveToken persists the token to disk.
func (c *APIClient) saveToken() error {
	if c.token == nil {
		return fmt.Errorf("no token to save")
	}
	path := filepath.Join(c.configDir, "token.json")
	data, err := json.MarshalIndent(c.token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// AuthURL returns the URL the user must visit to authorize the app.
func (c *APIClient) AuthURL() string {
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {c.config.ClientID},
		"redirect_uri":  {c.config.RedirectURI},
		"scope":         {"openid profile w_member_social"},
		"state":         {fmt.Sprintf("cmdr-%d", time.Now().Unix())},
	}
	return "https://www.linkedin.com/oauth/v2/authorization?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for access/refresh tokens.
func (c *APIClient) ExchangeCode(code string) error {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"redirect_uri":  {c.config.RedirectURI},
	}

	resp, err := c.httpClient.PostForm("https://www.linkedin.com/oauth/v2/accessToken", data)
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}

	c.token = &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	// Fetch the member URN.
	if err := c.fetchMemberURN(); err != nil {
		return fmt.Errorf("fetch member URN: %w", err)
	}

	return c.saveToken()
}

// RefreshAccessToken uses the refresh token to get a new access token.
func (c *APIClient) RefreshAccessToken() error {
	if c.token == nil || c.token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available; re-authorize with: cmdr linkedin setup")
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.token.RefreshToken},
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
	}

	resp, err := c.httpClient.PostForm("https://www.linkedin.com/oauth/v2/accessToken", data)
	if err != nil {
		return fmt.Errorf("token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}

	c.token.AccessToken = tokenResp.AccessToken
	c.token.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.RefreshToken != "" {
		c.token.RefreshToken = tokenResp.RefreshToken
	}

	return c.saveToken()
}

// ensureValidToken refreshes the access token if it is expired or near expiry.
func (c *APIClient) ensureValidToken() error {
	if c.token == nil {
		return fmt.Errorf("not authenticated; run: cmdr linkedin setup")
	}

	// Refresh if token expires within 5 minutes.
	if time.Until(c.token.ExpiresAt) < 5*time.Minute {
		if err := c.RefreshAccessToken(); err != nil {
			return err
		}
	}

	return nil
}

// fetchMemberURN gets the authenticated user's LinkedIn member URN.
func (c *APIClient) fetchMemberURN() error {
	req, err := http.NewRequest("GET", "https://api.linkedin.com/v2/userinfo", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("userinfo failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var profile struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return err
	}

	c.token.MemberURN = "urn:li:person:" + profile.Sub
	return nil
}

// linkedInPostPayload is the API payload for creating a text post.
type linkedInPostPayload struct {
	Author     string                 `json:"author"`
	Commentary string                 `json:"commentary"`
	Visibility linkedInPostVisibility `json:"visibility"`
	// Distribution controls how the post is shared.
	Distribution linkedInPostDistribution `json:"distribution"`
	LifecycleState string               `json:"lifecycleState"`
}

type linkedInPostVisibility struct {
	MemberNetworkVisibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
}

type linkedInPostDistribution struct {
	FeedDistribution               string `json:"feedDistribution"`
	TargetEntities                  []any  `json:"targetEntities"`
	ThirdPartyDistributionChannels  []any  `json:"thirdPartyDistributionChannels"`
}

// Publish posts content to LinkedIn using the Posts API (v2).
// Returns the post URN on success.
func (c *APIClient) Publish(content string) (string, error) {
	if err := c.ensureValidToken(); err != nil {
		return "", err
	}

	if c.token.MemberURN == "" {
		return "", fmt.Errorf("member URN not set; re-authorize with: cmdr linkedin setup")
	}

	// LinkedIn Posts API (Community Management API)
	payload := map[string]any{
		"author":         c.token.MemberURN,
		"commentary":     content,
		"visibility":     "PUBLIC",
		"distribution": map[string]any{
			"feedDistribution":              "MAIN_FEED",
			"targetEntities":                []any{},
			"thirdPartyDistributionChannels": []any{},
		},
		"lifecycleState": "PUBLISHED",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.linkedin.com/rest/posts", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LinkedIn-Version", "202401")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post to linkedin: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		// The post URN is in the x-restli-id header.
		postURN := resp.Header.Get("x-restli-id")
		if postURN == "" {
			// Try parsing from response body.
			var result map[string]any
			if err := json.Unmarshal(body, &result); err == nil {
				if id, ok := result["id"].(string); ok {
					postURN = id
				}
			}
		}
		return postURN, nil
	}

	return "", fmt.Errorf("linkedin publish failed (HTTP %d): %s", resp.StatusCode, string(body))
}

// IsConfigured checks if LinkedIn API credentials are set up.
func IsConfigured() bool {
	path := filepath.Join(DefaultConfigDir(), "oauth.json")
	_, err := os.Stat(path)
	return err == nil
}

// IsAuthenticated checks if a valid token exists.
func (c *APIClient) IsAuthenticated() bool {
	return c.token != nil && c.token.AccessToken != ""
}

// TokenStatus returns a human-readable status of the current token.
func (c *APIClient) TokenStatus() string {
	if c.token == nil {
		return "not authenticated"
	}
	if time.Now().After(c.token.ExpiresAt) {
		return "expired (needs refresh)"
	}
	remaining := time.Until(c.token.ExpiresAt)
	if remaining < 24*time.Hour {
		return fmt.Sprintf("valid (expires in %s)", remaining.Round(time.Minute))
	}
	return fmt.Sprintf("valid (expires %s)", c.token.ExpiresAt.Format("2006-01-02"))
}

// SetupCredentials saves OAuth2 client credentials to disk.
func SetupCredentials(clientID, clientSecret, redirectURI string) error {
	configDir := DefaultConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	config := OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(configDir, "oauth.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write oauth config: %w", err)
	}

	// Ensure the file has restrictive permissions (secrets).
	return os.Chmod(path, 0600)
}

// PostURL returns the LinkedIn URL for a given post URN.
func PostURL(postURN string) string {
	// Post URN format: urn:li:share:XXXXX or urn:li:ugcPost:XXXXX
	parts := strings.Split(postURN, ":")
	if len(parts) >= 4 {
		return fmt.Sprintf("https://www.linkedin.com/feed/update/%s/", postURN)
	}
	return postURN
}
