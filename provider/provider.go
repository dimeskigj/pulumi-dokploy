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
			Description: "Pulumi provider for managing Dokploy projects, environments, applications, Compose stacks, Postgres, MySQL, MariaDB, MongoDB, and Redis databases, domains, SSH keys, backup destinations, database backups, and volume backups.",
			Namespace:   "dimeskigj",
			Homepage:    "https://github.com/dimeskigj/pulumi-dokploy",
			Repository:  "https://github.com/dimeskigj/pulumi-dokploy",
			Publisher:   "dimeskigj",
			License:     "Apache-2.0",
			LanguageMap: map[string]any{
				"go":     map[string]any{"importBasePath": "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"},
				"nodejs": map[string]any{"packageName": "@dimeskigj/pulumi-dokploy"},
				"python": map[string]any{"packageName": "pulumi_dokploy", "moduleName": "pulumi_dokploy"},
				"csharp": map[string]any{"packageName": "Pulumi.Dokploy", "rootNamespace": "Pulumi"},
				"java":   map[string]any{"packageName": "net.dimeski.pulumi.dokploy", "basePackage": "net.dimeski.pulumi"},
			},
		},
		Config: infer.Config(&Config{}),
		Resources: []infer.InferredResource{
			infer.Resource(&Project{client: configuredClient}),
			infer.Resource(&Environment{client: configuredClient}),
			infer.Resource(&Application{client: configuredClient}),
			infer.Resource(&Compose{client: configuredClient}),
			infer.Resource(&Postgres{client: configuredClient}),
			infer.Resource(&MySQL{client: configuredClient}),
			infer.Resource(&MariaDB{client: configuredClient}),
			infer.Resource(&MongoDB{client: configuredClient}),
			infer.Resource(&Redis{client: configuredClient}),
			infer.Resource(&Domain{client: configuredClient}),
			infer.Resource(&Destination{client: configuredClient}),
			infer.Resource(&Backup{client: configuredClient}),
			infer.Resource(&VolumeBackup{client: configuredClient}),
			infer.Resource(&SSHKey{client: configuredClient}),
		},
		ModuleMap: map[tokens.ModuleName]tokens.ModuleName{"provider": "index"},
	})
}
