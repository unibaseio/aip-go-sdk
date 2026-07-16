// Package exauth holds the shared Unibase authorization helpers used by the
// example programs. It is a thin wrapper around the SDK's public auth
// package; import github.com/unibaseio/aip-go-sdk/auth directly in real code.
package exauth

import (
	"context"
	"fmt"

	"github.com/unibaseio/aip-go-sdk/auth"
)

// ConfigFile returns the path to the cached auth config.
func ConfigFile() string { return auth.ConfigFile() }

// LoadToken reads UNIBASE_PROXY_AUTH from the environment, then the config file.
func LoadToken() string { return auth.LoadToken() }

// SaveToken persists the token (and optional agent identity) to the config file.
func SaveToken(token, agentID, agentWallet string) error {
	return auth.SaveToken(token, agentID, agentWallet)
}

// ExtractWallet decodes the JWT payload and returns its `sub` claim.
func ExtractWallet(token string) string { return auth.ExtractWallet(token) }

// InteractiveAuth fetches an authorization URL and reads the signed JWT from stdin.
func InteractiveAuth(ctx context.Context) (token, wallet string, err error) {
	return auth.InteractiveAuth(ctx)
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
