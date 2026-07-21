// Package auth provides the Unibase authorization helpers shared by SDK
// consumers. Two interchangeable credential types — provide ONE of them:
//
//   - Proxy-auth JWT (UNIBASE_PROXY_AUTH): obtained from Unibase Pay via the
//     interactive browser flow. Sent as a Bearer token; the platform resolves
//     the wallet from it.
//   - Wallet private key (UNIBASE_WALLET_PRIVATE_KEY): the SDK derives the
//     wallet address and signs the registration message locally (EIP-191);
//     the platform recovers the wallet from the signature. The key never
//     leaves the machine.
//
// Resolution order: env var -> cached config file -> interactive flow (which
// lets the user pick either method).
package auth

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secpecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
	"golang.org/x/term"
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

func readConfig() map[string]string {
	cfg := map[string]string{}
	data, err := os.ReadFile(ConfigFile())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// writeConfig merges updates into the config file (0600 perms).
func writeConfig(updates map[string]string) error {
	path := ConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cfg := readConfig()
	for k, v := range updates {
		if v != "" {
			cfg[k] = v
		}
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

// LoadToken reads UNIBASE_PROXY_AUTH from the environment, then the config file.
func LoadToken() string {
	if env := os.Getenv("UNIBASE_PROXY_AUTH"); env != "" {
		return env
	}
	return readConfig()["UNIBASE_PROXY_AUTH"]
}

// SaveToken persists the token (and optional agent identity) to the config file.
func SaveToken(token, agentID, agentWallet string) error {
	if err := writeConfig(map[string]string{
		"UNIBASE_PROXY_AUTH": token,
		"AGENT_ID":           agentID,
		"AGENT_WALLET":       agentWallet,
	}); err != nil {
		return err
	}
	fmt.Printf("  saved auth token to %s\n", ConfigFile())
	return nil
}

// LoadPrivateKey reads UNIBASE_WALLET_PRIVATE_KEY from the environment, then
// the config file.
func LoadPrivateKey() string {
	if env := os.Getenv("UNIBASE_WALLET_PRIVATE_KEY"); env != "" {
		return env
	}
	return readConfig()["UNIBASE_WALLET_PRIVATE_KEY"]
}

// SavePrivateKey persists the wallet private key to the config file (0600
// perms). The key is stored locally only — it is never sent to the platform.
func SavePrivateKey(key string) error {
	if err := writeConfig(map[string]string{"UNIBASE_WALLET_PRIVATE_KEY": key}); err != nil {
		return err
	}
	fmt.Printf("  saved wallet key to %s (never sent to the platform)\n", ConfigFile())
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

// WalletFromPrivateKey derives the EIP-55 checksummed wallet address from a
// hex private key. Purely local — the key is never transmitted.
func WalletFromPrivateKey(privateKey string) (string, error) {
	keyHex := strings.TrimPrefix(strings.TrimSpace(privateKey), "0x")
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key hex: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: %d bytes (want 32)", len(keyBytes))
	}
	priv := secp256k1.PrivKeyFromBytes(keyBytes)
	pub := priv.PubKey().SerializeUncompressed() // 65 bytes, 0x04 prefix

	h := sha3.NewLegacyKeccak256()
	h.Write(pub[1:])
	addr := h.Sum(nil)[12:]
	return checksumAddress(addr), nil
}

// SignMessage signs a message with the private key (EIP-191 personal sign,
// offline). The platform recovers the wallet address from this signature
// during token-less registration — the key itself is never transmitted.
// Returns the 65-byte r||s||v signature as 0x-prefixed hex.
func SignMessage(privateKey, message string) (string, error) {
	keyHex := strings.TrimPrefix(strings.TrimSpace(privateKey), "0x")
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key hex: %w", err)
	}
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: %d bytes (want 32)", len(keyBytes))
	}
	priv := secp256k1.PrivKeyFromBytes(keyBytes)

	h := sha3.NewLegacyKeccak256()
	fmt.Fprintf(h, "\x19Ethereum Signed Message:\n%d%s", len(message), message)
	digest := h.Sum(nil)

	// SignCompact returns [27+recid] || r || s; Ethereum wants r || s || v.
	compact := secpecdsa.SignCompact(priv, digest, false)
	eth := append(compact[1:], compact[0])
	return "0x" + hex.EncodeToString(eth), nil
}

// checksumAddress formats a 20-byte address with EIP-55 checksum casing.
func checksumAddress(addr []byte) string {
	lower := hex.EncodeToString(addr)
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(lower))
	hash := hex.EncodeToString(h.Sum(nil))

	out := make([]byte, len(lower))
	for i, c := range []byte(lower) {
		if c >= 'a' && c <= 'f' && hash[i] >= '8' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return "0x" + string(out)
}

// InteractiveAuth runs the first-run flow, letting the user pick a credential
// type: browser authorization (JWT) or a wallet private key.
// Returns ("", wallet) in private-key mode.
func InteractiveAuth(ctx context.Context) (token, wallet string, err error) {
	fmt.Println("\n=== Unibase Authorization ===")
	fmt.Println("Choose an authorization method:")
	fmt.Println("  1) Browser authorization — open a URL, approve, paste the JWT token")
	fmt.Println("  2) Wallet private key — paste a hex private key (stored locally only)")
	fmt.Print("Choice [1]: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	if choice == "2" {
		return interactivePrivateKey()
	}
	return interactiveToken(ctx)
}

func interactiveToken(ctx context.Context) (token, wallet string, err error) {
	fmt.Println("\n[1/3] Fetching authorization URL ...")

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

func interactivePrivateKey() (token, wallet string, err error) {
	fmt.Println("\nPaste your wallet private key (hex, input hidden) and press Enter:")
	fmt.Print("  Private key: ")

	var key string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, rerr := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if rerr != nil {
			return "", "", rerr
		}
		key = strings.TrimSpace(string(raw))
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		key = strings.TrimSpace(scanner.Text())
	}
	if key == "" {
		return "", "", fmt.Errorf("no private key provided — aborted")
	}

	wallet, err = WalletFromPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	fmt.Printf("  wallet: %s\n", wallet)
	_ = SavePrivateKey(key)
	return "", wallet, nil
}

// EnsureAuth returns usable credentials, running the interactive flow if
// nothing is cached. JWT mode returns (token, wallet); private-key mode
// returns ("", wallet) with the address derived locally from the key.
func EnsureAuth(ctx context.Context) (token, wallet string, err error) {
	if token = LoadToken(); token != "" {
		return token, ExtractWallet(token), nil
	}
	if key := LoadPrivateKey(); key != "" {
		wallet, err = WalletFromPrivateKey(key)
		return "", wallet, err
	}
	return InteractiveAuth(ctx)
}
