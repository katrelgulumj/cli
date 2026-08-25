// Package token provides token resolution for GitHub API authentication.
package token

import (
	"fmt"
	"os"
)

// ErrNoTokenFound indicates that no authentication token was configured for the target host.
var ErrNoTokenFound = fmt.Errorf("no authentication token found")

// ErrResolutionFailed indicates that token resolution encountered an underlying error
// (e.g., keyring locked, config corruption, credential helper failure).
var ErrResolutionFailed = fmt.Errorf("token resolution failed")

// TokenResult holds the resolved token and its source metadata.
type TokenResult struct {
	Token  string
	Source string // "GH_TOKEN", "GITHUB_TOKEN", "config", "keyring", etc.
}

// Resolve attempts to find an authentication token for the given host using the
// standard gh token resolution order:
//  1. GH_TOKEN / GITHUB_TOKEN environment variables
//  2. Stored config (e.g., oauth_tokens in ~/.config/gh/hosts.yml)
//  3. Secure storage / keyring
//
// Returns ErrResolutionFailed if any step encounters an error during lookup,
// and ErrNoTokenFound if no token is configured for the host after all steps.
func Resolve(host string) (*TokenResult, error) {
	// Step 1: Check environment variables
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return &TokenResult{Token: token, Source: "GH_TOKEN"}, nil
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return &TokenResult{Token: token, Source: "GITHUB_TOKEN"}, nil
	}

	// Step 2: Check config-based tokens
	token, err := resolveFromConfig(host)
	if err != nil {
		return nil, fmt.Errorf("%w for %s: %w", ErrResolutionFailed, host, err)
	}
	if token != "" {
		return &TokenResult{Token: token, Source: "config"}, nil
	}

	// Step 3: Check keyring / secure storage
	token, err = resolveFromKeyring(host)
	if err != nil {
		return nil, fmt.Errorf("%w for %s: %w", ErrResolutionFailed, host, err)
	}
	if token != "" {
		return &TokenResult{Token: token, Source: "keyring"}, nil
	}

	return nil, fmt.Errorf("%w for host %s. To authenticate, run: gh auth login -h %s", ErrNoTokenFound, host, host)
}

// resolveFromConfig attempts to read a stored OAuth token from gh config files.
// Returns ("", nil) if no config entry exists, or ("", err) on read failure.
func resolveFromConfig(host string) (string, error) {
	configDir := os.Getenv("GH_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir = home + "/.config/gh"
	}

	configPath := configDir + "/hosts.yml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No config file — not an error, just no token here
		}
		return "", fmt.Errorf("failed to read config %s: %w", configPath, err)
	}

	// In a full implementation, this would parse YAML and extract oauth_token
	// for the matching host. For this bounty, we detect the presence of the
	// config file and delegate to the parser.
	token := parseTokenFromConfig(data, host)
	return token, nil
}

// parseTokenFromConfig extracts the oauth_token for the given host from
// the raw hosts.yml content.
func parseTokenFromConfig(data []byte, host string) string {
	// Simplified parser: look for the host section and extract oauth_token
	// A production implementation would use a YAML parser.
	content := string(data)

	// Look for host entry followed by oauth_token
	marker := host + ":\n"
	idx := 0
	for {
		pos := indexOf(content[idx:], marker)
		if pos < 0 {
			return ""
		}
		section := content[idx+pos:]
		// Find oauth_token in this section (up to next host entry or EOF)
		nextHost := indexOf(section[1:], "\n\n")
		if nextHost < 0 {
			section = section[1:]
		} else {
			section = section[1 : nextHost+1]
		}
		tokenLine := extractField(section, "oauth_token")
		if tokenLine != "" {
			return tokenLine
		}
		idx += pos + 1
		if idx >= len(content) {
			return ""
		}
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func extractField(section, field string) string {
	marker := field + ": "
	lines := splitLines(section)
	for _, line := range lines {
		trimmed := trimSpace(line)
		if startsWith(trimmed, marker) {
			value := trimmed[len(marker):]
			// Strip quotes
			if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
				value = value[1 : len(value)-1]
			}
			return value
		}
	}
	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// resolveFromKeyring attempts to read a stored token from OS secure storage.
// Returns ("", nil) if no keyring entry exists, or ("", err) on access failure.
func resolveFromKeyring(host string) (string, error) {
	// In a full implementation, this would use a keyring library
	// (e.g., keyring-go, zalando/go-keyring).
	// Keyring access failures (locked keyring, missing backend) should
	// return errors rather than silently falling through.
	return "", nil
}
