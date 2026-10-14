package contract

import (
	dokploy "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func githubContract(ctx *pulumi.Context) error {
	args := &dokploy.GitHubIntegrationArgs{Name: pulumi.String("existing"), GithubAppName: pulumi.String("app")}
	r, err := dokploy.NewGitHubIntegration(ctx, "existing", args, pulumi.Import(pulumi.ID("relation-id-placeholder")))
	if err != nil { return err }
	_ = r.Name; _ = r.GithubAppName; _ = r.GithubId; _ = r.GitProviderId; _ = r.OrganizationId
	return nil
}
