package dokploy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	p "github.com/pulumi/pulumi-go-provider"
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

// Diff reports provider configuration changes as in-place updates.
//
// Without it, infer's default config diff compares the engine's old inputs,
// which still carry `version` and the framework's `__`-prefixed keys, against
// new inputs with those keys stripped, and forces a replace on every such key.
// A plugin version bump then made the engine replace every Dokploy resource.
// Endpoint and API key changes never require recreating Dokploy resources, so
// they only reconfigure the provider.
func (c *Config) Diff(_ context.Context, req infer.DiffRequest[*Config, *Config]) (infer.DiffResponse, error) {
	diff := map[string]p.PropertyDiff{}
	if req.State.Endpoint != req.Inputs.Endpoint {
		diff["endpoint"] = p.PropertyDiff{Kind: p.Update, InputDiff: true}
	}
	if req.State.APIKey != req.Inputs.APIKey {
		diff["apiKey"] = p.PropertyDiff{Kind: p.Update, InputDiff: true}
	}
	return infer.DiffResponse{HasChanges: len(diff) > 0, DetailedDiff: diff}, nil
}

func (c *Config) Configure(ctx context.Context) error {
	_ = ctx
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("endpoint is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("apiKey is required")
	}
	configured, err := client.New(c.Endpoint, c.APIKey, client.WithUserAgentVersion(Version))
	if err != nil {
		return fmt.Errorf("configure Dokploy client: %w", err)
	}
	c.client = configured
	return nil
}
