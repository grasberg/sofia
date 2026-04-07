package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// DefaultQwenOAuthAPIBase is the API endpoint used with Qwen OAuth tokens.
const DefaultQwenOAuthAPIBase = "https://portal.qwen.ai/v1"

// QwenOAuthConfig returns the OAuth configuration for Qwen (chat.qwen.ai free tier).
// Uses a browser-based login flow; tokens are used at portal.qwen.ai/v1.
func QwenOAuthConfig() OAuthProviderConfig {
	return OAuthProviderConfig{
		Issuer:   "https://chat.qwen.ai",
		TokenURL: "https://chat.qwen.ai/api/v1/auths/token",
		ClientID: "qwen-code",
		Scopes:   "openid profile",
		Port:     51199,
	}
}

// LoginQwenBrowser opens the Qwen login page in the browser. The user logs in
// (or scans a QR code), and the callback receives the token. Falls back to
// importing an existing ~/.qwen/oauth_creds.json if the browser flow fails.
func LoginQwenBrowser(cfg OAuthProviderConfig) (*AuthCredential, error) {
	// First, try importing existing Qwen Code credentials.
	if cred, err := importQwenCodeCredentials(); err == nil && cred != nil {
		fmt.Println("Imported existing Qwen Code credentials from ~/.qwen/oauth_creds.json")
		return cred, nil
	}

	// Fall back to browser-based PKCE login.
	return LoginBrowser(cfg)
}

// importQwenCodeCredentials checks for an existing ~/.qwen/oauth_creds.json
// (created by Qwen Code CLI) and imports the tokens.
func importQwenCodeCredentials() (*AuthCredential, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := home + "/.qwen/oauth_creds.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiryDate   int64  `json:"expiry_date"`
		ResourceURL  string `json:"resource_url"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.AccessToken == "" {
		return nil, fmt.Errorf("no access token in qwen creds file")
	}

	var expiresAt time.Time
	if raw.ExpiryDate > 0 {
		expiresAt = time.UnixMilli(raw.ExpiryDate)
	}

	return &AuthCredential{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    expiresAt,
		Provider:     "qwen",
		AuthMethod:   "oauth",
	}, nil
}

// RefreshQwenToken refreshes a Qwen OAuth access token.
func RefreshQwenToken(cred *AuthCredential) (*AuthCredential, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available for qwen")
	}

	cfg := QwenOAuthConfig()
	refreshed, err := RefreshAccessToken(cred, cfg)
	if err != nil {
		// If standard refresh fails, try re-importing from qwen-code creds file.
		if imported, impErr := importQwenCodeCredentials(); impErr == nil && imported != nil && !imported.IsExpired() {
			return imported, nil
		}
		return nil, err
	}
	refreshed.Provider = "qwen"
	return refreshed, nil
}

func decodeBase64(s string) string {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return string(data)
}
