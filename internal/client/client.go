package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
)

// Option customizes a Client during construction.
type Option func(*clientOptions)

type clientOptions struct {
	httpClient  *http.Client
	retryPolicy RetryPolicy
}

// WithHTTPClient supplies the underlying HTTP client (principally for tests).
func WithHTTPClient(httpClient *http.Client) Option {
	return func(options *clientOptions) { options.httpClient = httpClient }
}

// WithRetryPolicy supplies the bounded GET retry policy.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(options *clientOptions) { options.retryPolicy = policy }
}

// Client is the authenticated Dokploy API client.
type Client struct {
	*generated.ClientWithResponses
	endpoint string
}

// New creates an authenticated Dokploy API client.
func New(endpoint, apiKey string, options ...Option) (*Client, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, errors.New("endpoint is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("apiKey is required")
	}
	normalized, err := normalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	opts := clientOptions{retryPolicy: defaultRetryPolicy()}
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	httpClient := opts.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	} else {
		copy := *httpClient
		httpClient = &copy
		if httpClient.Timeout == 0 {
			httpClient.Timeout = 30 * time.Second
		}
	}
	if httpClient.Transport == nil {
		httpClient.Transport = http.DefaultTransport
	}
	httpClient.Transport = newRetryTransport(httpClient.Transport, opts.retryPolicy)
	generatedClient, err := generated.NewClientWithResponses(normalized,
		generated.WithHTTPClient(httpClient), generated.WithRequestEditorFn(apiKeyEditor(apiKey)))
	if err != nil {
		return nil, err
	}
	return &Client{ClientWithResponses: generatedClient, endpoint: normalized}, nil
}

func normalizeEndpoint(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("endpoint must be a valid HTTP(S) URL without query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/api") {
		parsed.Path += "/api"
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func apiKeyEditor(apiKey string) generated.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("Accept", "application/json")
		return nil
	}
}
