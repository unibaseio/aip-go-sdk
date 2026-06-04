// Package exauth holds the shared Unibase authorization helpers used by the
// example programs: loading/saving the proxy-auth JWT, the interactive
// first-run flow, and extracting the wallet from the token's `sub` claim.
package exauth

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFile returns the path to the cached auth config.
func ConfigFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "unibase-aip-sdk", "config.json")
}

func payURL() string {
	if v := os.Getenv("UNIBASE_PAY_URL"); v != "" {
		return v
	}
	return "https://api.pay.unibase.com"
}

// LoadToken reads UNIBASE_PROXY_AUTH from the environment, then the config file.
func LoadToken() string {
	if env := os.Getenv("UNIBASE_PROXY_AUTH"); env != "" {
		return env
	}
	data, err := os.ReadFile(ConfigFile())
	if err != nil {
		return ""
	}
	var cfg map[string]string
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg["UNIBASE_PROXY_AUTH"]
}

// SaveToken persists the token (and optional agent identity) to the config file.
func SaveToken(token, agentID, agentWallet string) error {
	path := ConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := map[string]string{"UNIBASE_PROXY_AUTH": token}
	if agentID != "" {
		data["AGENT_ID"] = agentID
	}
	if agentWallet != "" {
		data["AGENT_WALLET"] = agentWallet
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	fmt.Printf("  saved auth token to %s\n", path)
	return nil
}

// ExtractWallet decodes the JWT payload and returns its `sub` claim.
func ExtractWallet(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		p := parts[1]
		if pad := len(p) % 4; pad != 0 {
			p += strings.Repeat("=", 4-pad)
		}
		payload, err = base64.StdEncoding.DecodeString(p)
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

// InteractiveAuth fetches an authorization URL and reads the signed JWT from stdin.
func InteractiveAuth(ctx context.Context) (token, wallet string, err error) {
	fmt.Println("\n=== Step 1: Interactive Authorization ===")
	fmt.Println("[1/3] Fetching authorization URL ...")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payURL()+"/v1/init", strings.NewReader("true"))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("cannot reach %s: %w", payURL(), err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", err
	}
	authURL, _ := parsed["auth_url"].(string)
	if authURL == "" {
		authURL, _ = parsed["authUrl"].(string)
	}
	if authURL == "" {
		return "", "", fmt.Errorf("no auth URL in response: %s", body)
	}
	fmt.Printf("\n[2/3] Open this URL in your browser and approve:\n\n  %s\n\n", authURL)
	fmt.Println("[3/3] Paste your Authorization token below and press Enter:")
	fmt.Print("  Token: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	token = strings.TrimSpace(scanner.Text())
	if token == "" {
		return "", "", fmt.Errorf("no token provided — aborted")
	}
	wallet = ExtractWallet(token)
	_ = SaveToken(token, "", "")
	return token, wallet, nil
}

// EnsureAuth returns a usable token + wallet, running interactive auth if no
// cached token is found.
func EnsureAuth(ctx context.Context) (token, wallet string, err error) {
	if token = LoadToken(); token != "" {
		fmt.Println("loaded cached UNIBASE_PROXY_AUTH")
		return token, ExtractWallet(token), nil
	}
	return InteractiveAuth(ctx)
}
