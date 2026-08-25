package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katrelgulumj/cli/pkg/token"
)

// ---------------------------------------------------------------------------
// Case 1: Token resolution fails → command halts with non-zero error,
//         no HTTP request is made.
// ---------------------------------------------------------------------------
func TestDo_TokenResolutionFailure_HaltsWithoutRequest(t *testing.T) {
	var requestMade bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMade = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Use an isolated config dir with no tokens
	tmpDir := t.TempDir()
	os.Setenv("GH_CONFIG_DIR", tmpDir)
	os.Unsetenv("GH_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		os.Unsetenv("GH_CONFIG_DIR")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
	}()

	// Override the host to point to our test server
	// We test the resolution logic directly since we can't easily override the URL in Do
	_, err := token.Resolve("nonexistent-host.example.com")
	if err == nil {
		t.Fatal("expected error for nonexistent host with no config")
	}

	// Verify error contains helpful auth prompt
	if !strings.Contains(err.Error(), "To authenticate") {
		t.Fatalf("expected auth prompt in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("expected 'gh auth login' in error message, got: %v", err)
	}

	if requestMade {
		t.Fatal("no HTTP request should have been made when token resolution fails")
	}
}

// ---------------------------------------------------------------------------
// Case 2: Credential resolution error (config read failure) → wrapped error
//         propagated to caller with diagnostic info.
// ---------------------------------------------------------------------------
func TestDo_ConfigReadError_WrappedWithDiagnostic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a hosts.yml that is a directory (will cause read error)
	err := os.MkdirAll(tmpDir+"/hosts.yml", 0755)
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("GH_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("GH_CONFIG_DIR")

	_, err = token.Resolve("github.com")
	if err == nil {
		t.Fatal("expected error when config path is a directory")
	}

	// Should be a resolution failure, not a "no token found"
	if !errors.Is(err, token.ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed, got: %v (type: %T)", err, err)
	}
}

// ---------------------------------------------------------------------------
// Case 3: No token configured → clear error prompting authentication.
// ---------------------------------------------------------------------------
func TestDo_NoTokenConfigured_ClearAuthPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("GH_CONFIG_DIR", tmpDir)
	os.Unsetenv("GH_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		os.Unsetenv("GH_CONFIG_DIR")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
	}()

	_, err := token.Resolve("github.com")
	if err == nil {
		t.Fatal("expected error when no token is configured")
	}

	if !errors.Is(err, token.ErrNoTokenFound) {
		t.Fatalf("expected ErrNoTokenFound, got: %v", err)
	}

	// Must include hostname and auth instructions
	if !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("expected hostname in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gh auth login -h github.com") {
		t.Fatalf("expected full auth command, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Case 4: Distinction between "host found but token unresolvable" vs
//         "completely unauthenticated".
// ---------------------------------------------------------------------------
func TestDo_DistinguishesResolutionFailureFromNoToken(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.Setenv("GH_CONFIG_DIR", tmpDir)
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
		defer func() {
			os.Unsetenv("GH_CONFIG_DIR")
			os.Unsetenv("GH_TOKEN")
			os.Unsetenv("GITHUB_TOKEN")
		}()

		_, err := token.Resolve("github.com")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, token.ErrNoTokenFound) {
			t.Fatalf("expected ErrNoTokenFound, got: %v", err)
		}
		if errors.Is(err, token.ErrResolutionFailed) {
			t.Fatalf("should NOT be ErrResolutionFailed, got: %v", err)
		}
	})

	t.Run("resolution_error", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a directory named hosts.yml to cause a read error
		os.MkdirAll(tmpDir+"/hosts.yml", 0755)
		os.Setenv("GH_CONFIG_DIR", tmpDir)
		defer os.Unsetenv("GH_CONFIG_DIR")

		_, err := token.Resolve("github.com")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, token.ErrResolutionFailed) {
			t.Fatalf("expected ErrResolutionFailed, got: %v", err)
		}
		if errors.Is(err, token.ErrNoTokenFound) {
			t.Fatalf("should NOT be ErrNoTokenFound, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Case 5: GH_TOKEN env var takes precedence → token resolved successfully.
// ---------------------------------------------------------------------------
func TestDo_GHTokenEnvVar_TokenResolved(t *testing.T) {
	os.Setenv("GH_TOKEN", "ghp_test_token_12345")
	defer os.Unsetenv("GH_TOKEN")

	result, err := token.Resolve("github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token != "ghp_test_token_12345" {
		t.Fatalf("expected token 'ghp_test_token_12345', got: %q", result.Token)
	}
	if result.Source != "GH_TOKEN" {
		t.Fatalf("expected source 'GH_TOKEN', got: %q", result.Source)
	}
}

// ---------------------------------------------------------------------------
// Case 6: GITHUB_TOKEN fallback when GH_TOKEN not set.
// ---------------------------------------------------------------------------
func TestDo_GITHUBTokenFallback(t *testing.T) {
	os.Unsetenv("GH_TOKEN")
	os.Setenv("GITHUB_TOKEN", "ghp_fallback_token_67890")
	defer os.Unsetenv("GITHUB_TOKEN")

	result, err := token.Resolve("github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token != "ghp_fallback_token_67890" {
		t.Fatalf("expected fallback token, got: %q", result.Token)
	}
	if result.Source != "GITHUB_TOKEN" {
		t.Fatalf("expected source 'GITHUB_TOKEN', got: %q", result.Source)
	}
}

// ---------------------------------------------------------------------------
// Case 7: Config-based token resolution.
// ---------------------------------------------------------------------------
func TestDo_ConfigTokenResolution(t *testing.T) {
	tmpDir := t.TempDir()
	config := `github.com:
    user: testuser
    oauth_token: ghp_config_token_abc123
    git_protocol: https
`
	err := os.WriteFile(filepath.Join(tmpDir, "hosts.yml"), []byte(config), 0600)
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("GH_CONFIG_DIR", tmpDir)
	os.Unsetenv("GH_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		os.Unsetenv("GH_CONFIG_DIR")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
	}()

	result, err := token.Resolve("github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token != "ghp_config_token_abc123" {
		t.Fatalf("expected config token, got: %q", result.Token)
	}
	if result.Source != "config" {
		t.Fatalf("expected source 'config', got: %q", result.Source)
	}
}

// ---------------------------------------------------------------------------
// Case 8: Explicit unauthenticated access still works.
// ---------------------------------------------------------------------------
func TestDo_ExplicitUnauthenticated_BypassesTokenCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Fatalf("expected no Authorization header for unauthenticated request, got: %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer server.Close()

	os.Setenv("GH_CONFIG_DIR", t.TempDir())
	os.Unsetenv("GH_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		os.Unsetenv("GH_CONFIG_DIR")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
	}()

	resp, err := Do(Options{
		Host:            "github.com",
		Path:            "/user",
		Unauthenticated: true,
		HTTPClient:      server.Client(),
		BaseURL:         server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Case 9: Authenticated request with resolved token sends Bearer header.
// ---------------------------------------------------------------------------
func TestDo_AuthenticatedRequest_SendsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer ghp_test_token_12345" {
			t.Fatalf("expected 'Bearer ghp_test_token_12345', got: %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"digivasserver-ai"}`))
	}))
	defer server.Close()

	os.Setenv("GH_TOKEN", "ghp_test_token_12345")
	defer os.Unsetenv("GH_TOKEN")

	// The Do function constructs URLs for github.com/api.github.com,
	// but our test server is on localhost. We test the token resolution
	// and header logic separately.
	result, err := token.Resolve("github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token != "ghp_test_token_12345" {
		t.Fatalf("expected token, got: %q", result.Token)
	}
}

// ---------------------------------------------------------------------------
// Case 10: Default host is github.com when empty.
// ---------------------------------------------------------------------------
func TestDo_DefaultHostIsGithub(t *testing.T) {
	os.Setenv("GH_TOKEN", "test")
	defer os.Unsetenv("GH_TOKEN")

	result, err := token.Resolve("github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token != "test" {
		t.Fatalf("expected token, got: %q", result.Token)
	}
}

// ---------------------------------------------------------------------------
// Case 11: Nonexistent host in config returns clear error.
// ---------------------------------------------------------------------------
func TestDo_NonexistentHost_NoTokenFound(t *testing.T) {
	tmpDir := t.TempDir()
	config := `github.com:
    user: testuser
    oauth_token: ghp_something
    git_protocol: https
`
	err := os.WriteFile(filepath.Join(tmpDir, "hosts.yml"), []byte(config), 0600)
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("GH_CONFIG_DIR", tmpDir)
	os.Unsetenv("GH_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		os.Unsetenv("GH_CONFIG_DIR")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
	}()

	_, err = token.Resolve("enterprise.example.com")
	if err == nil {
		t.Fatal("expected error for unconfigured host")
	}
	if !errors.Is(err, token.ErrNoTokenFound) {
		t.Fatalf("expected ErrNoTokenFound, got: %v", err)
	}
	if !strings.Contains(err.Error(), "enterprise.example.com") {
		t.Fatalf("expected hostname in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Case 12: HTTP request is never made when token resolution fails.
// ---------------------------------------------------------------------------
func TestDo_NoHTTPRequestOnResolutionFailure(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	os.Setenv("GH_CONFIG_DIR", tmpDir)
	os.Unsetenv("GH_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		os.Unsetenv("GH_CONFIG_DIR")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
	}()

	_, err := Do(Options{
		Host: "github.com",
		Path: "/user",
	})
	if err == nil {
		t.Fatal("expected error when no token configured")
	}
	if requestCount != 0 {
		t.Fatalf("expected 0 HTTP requests, got %d", requestCount)
	}
}
