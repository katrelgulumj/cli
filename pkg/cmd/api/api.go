// Package api implements the core logic for making authenticated GitHub API calls.
// It enforces that token resolution succeeds before dispatching any HTTP request,
// preventing silent fallback to unauthenticated requests.
package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/katrelgulumj/cli/pkg/token"
)

var (
	// ErrUnauthenticated is returned when gh api is invoked without authentication
	// and no --unauthenticated flag is set.
	ErrUnauthenticated = errors.New("unauthenticated access not permitted by default")

	// ErrTokenResolution is returned when the token resolver encounters an error
	// looking up credentials for the target host.
	ErrTokenResolution = errors.New("failed to resolve authentication token")
)

// Options configures an API request.
type Options struct {
	// Host is the GitHub hostname (e.g., "github.com", "github.example.com").
	Host string

	// Method is the HTTP method (GET, POST, etc.).
	Method string

	// Path is the API endpoint path (e.g., "/user", "/repos/owner/repo").
	Path string

	// Unauthenticated, when true, explicitly allows requests without a token.
	Unauthenticated bool

	// HTTPClient is the HTTP client to use. If nil, http.DefaultClient is used.
	HTTPClient *http.Client

	// BaseURL overrides the API base URL. Used for testing.
	// When set, the URL is constructed as BaseURL + Path.
	BaseURL string
}

// Response holds the result of an API call.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// Do performs an authenticated GitHub API call. It resolves the token for the
// target host BEFORE making any HTTP request. If token resolution fails, Do
// returns an error immediately without contacting the server.
//
// Returns ErrTokenResolution if credential lookup fails (keyring error, config
// corruption, etc.), ErrUnauthenticated if no token is found and the request
// is not explicitly unauthenticated, or the underlying HTTP error on network
// failure.
func Do(opts Options) (*Response, error) {
	host := opts.Host
	if host == "" {
		host = "github.com"
	}

	method := opts.Method
	if method == "" {
		method = "GET"
	}

	// --- Token resolution: fail fast, no silent fallback ---
	tokenResult, err := token.Resolve(host)
	if err != nil {
		if errors.Is(err, token.ErrResolutionFailed) {
			return nil, fmt.Errorf("%w for host %s: %w", ErrTokenResolution, host, err)
		}
		if errors.Is(err, token.ErrNoTokenFound) {
			if opts.Unauthenticated {
				// Explicitly unauthenticated — proceed without token
				return doHTTP(opts, host, "")
			}
			return nil, fmt.Errorf("%w: %w. To authenticate, run: gh auth login -h %s or set GH_TOKEN/GITHUB_TOKEN",
				ErrUnauthenticated, err, host)
		}
		return nil, fmt.Errorf("unexpected token resolution error: %w", err)
	}

	// Token resolved successfully — proceed with authentication
	return doHTTP(opts, host, tokenResult.Token)
}

// doHTTP performs the actual HTTP request with the given token.
func doHTTP(opts Options, host, authToken string) (*Response, error) {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	var url string
	if opts.BaseURL != "" {
		url = strings.TrimRight(opts.BaseURL, "/") + opts.Path
	} else if host == "github.com" {
		url = fmt.Sprintf("https://api.github.com%s", opts.Path)
	} else {
		url = fmt.Sprintf("https://%s/api/v3%s", host, opts.Path)
	}

	req, err := http.NewRequest(method(opts.Method), url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gh-cli/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}, nil
}

func method(m string) string {
	m = strings.ToUpper(m)
	if m == "" {
		return "GET"
	}
	return m
}
