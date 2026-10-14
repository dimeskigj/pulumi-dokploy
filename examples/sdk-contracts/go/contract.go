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

func giteaContract(ctx *pulumi.Context) error {
	args := &dokploy.GiteaIntegrationArgs{
		Name: pulumi.String("gitea"), GiteaUrl: pulumi.String("https://gitea.example.invalid"),
		ClientId: pulumi.String("client-id"), ClientSecret: pulumi.String("secret-placeholder"),
		RedirectUri: pulumi.String("https://dokploy.example.invalid/callback"),
		GiteaInternalUrl: pulumi.StringPtr("https://internal.example.invalid"), OrganizationName: pulumi.StringPtr("engineering"),
	}
	r, err := dokploy.NewGiteaIntegration(ctx, "gitea", args, pulumi.Import(pulumi.ID("relation-id-placeholder")))
	if err != nil { return err }
	_ = r.Name; _ = r.GiteaUrl; _ = r.ClientId; _ = r.ClientSecret; _ = r.RedirectUri; _ = r.GiteaInternalUrl
	_ = r.OrganizationName; _ = r.GiteaId; _ = r.GitProviderId; _ = r.OrganizationId
	return nil
}

func bitbucketContracts(ctx *pulumi.Context) error {
	apiTokenArgs := &dokploy.BitbucketIntegrationArgs{
		Name: pulumi.String("bitbucket-api"), ApiToken: pulumi.StringPtr("token-placeholder"),
		BitbucketEmail: pulumi.StringPtr("user@example.invalid"), BitbucketWorkspaceName: pulumi.StringPtr("workspace"),
	}
	apiToken, err := dokploy.NewBitbucketIntegration(ctx, "bitbucket-api", apiTokenArgs, pulumi.Import(pulumi.ID("relation-id-placeholder")))
	if err != nil { return err }
	appPasswordArgs := &dokploy.BitbucketIntegrationArgs{
		Name: pulumi.String("bitbucket-app-password"), AppPassword: pulumi.StringPtr("password-placeholder"),
		BitbucketUsername: pulumi.StringPtr("legacy-user"),
	}
	appPassword, err := dokploy.NewBitbucketIntegration(ctx, "bitbucket-app-password", appPasswordArgs, pulumi.Import(pulumi.ID("relation-id-placeholder")))
	if err != nil { return err }
	for _, resource := range []*dokploy.BitbucketIntegration{apiToken, appPassword} {
		_ = resource.Name; _ = resource.ApiToken; _ = resource.AppPassword; _ = resource.BitbucketEmail
		_ = resource.BitbucketUsername; _ = resource.BitbucketWorkspaceName; _ = resource.BitbucketId
		_ = resource.GitProviderId; _ = resource.OrganizationId
	}
	return nil
}
