// Package dokploy implements the Dokploy Pulumi provider.
package dokploy

import (
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/middleware/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// Version is initialized by the Go linker to contain the semver of this build.
var Version string

// Name controls how this provider is referenced in package names and elsewhere.
const Name = "dokploy"

// Provider creates a new instance of the provider.
func Provider() p.Provider {
	return infer.Provider(infer.Options{
		Metadata: schema.Metadata{
			DisplayName: "Dokploy",
			Namespace:   "gjorgjidimeski",
			Homepage:    "https://github.com/gjorgjidimeski/pulumi-dokploy",
			LanguageMap: map[string]any{"go": map[string]any{
				"importBasePath": "github.com/gjorgjidimeski/pulumi-dokploy/sdk/go/dokploy",
			}},
		},
		Config: infer.Config(&Config{}),
		Resources: []infer.InferredResource{
			infer.Resource(Project{client: configuredClient}),
			infer.Resource(Environment{client: configuredClient}),
			infer.Resource(Application{client: configuredClient}),
			infer.Resource(Compose{client: configuredClient}),
			infer.Resource(Postgres{client: configuredClient}),
			infer.Resource(Redis{client: configuredClient}),
			infer.Resource(Domain{client: configuredClient}),
		},
		ModuleMap: map[tokens.ModuleName]tokens.ModuleName{"provider": "index"},
	})
}
