package client

import (
	"errors"
	"net/url"
	"strings"
)

// Client is the initial Dokploy HTTP client configuration.
type Client struct {
	endpoint string
	apiKey   string
}

// New validates and stores the Dokploy endpoint and API key.
func New(endpoint, apiKey string) (*Client, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, errors.New("endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("endpoint must be a valid URL")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("apiKey is required")
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), apiKey: apiKey}, nil
}
