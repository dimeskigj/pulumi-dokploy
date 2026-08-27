package dokploy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Config defines provider-level configuration.
type Config struct {
	Endpoint string `pulumi:"endpoint"`
	APIKey   string `pulumi:"apiKey" provider:"secret"`
	client   *client.Client
}

func (c *Config) Annotate(a infer.Annotator) {
	a.Describe(&c.Endpoint, "Base URL of the Dokploy instance.")
	a.Describe(&c.APIKey, "Dokploy API key sent through x-api-key.")
	a.SetDefault(&c.Endpoint, nil, "DOKPLOY_ENDPOINT")
	a.SetDefault(&c.APIKey, nil, "DOKPLOY_API_KEY")
}

func (c *Config) Configure(ctx context.Context) error {
	_ = ctx
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("endpoint is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("apiKey is required")
	}
	configured, err := client.New(c.Endpoint, c.APIKey)
	if err != nil {
		return fmt.Errorf("configure Dokploy client: %w", err)
	}
	c.client = configured
	return nil
}
