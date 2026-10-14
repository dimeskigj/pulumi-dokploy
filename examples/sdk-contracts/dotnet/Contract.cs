using Pulumi;
using Dokploy = Dimeskigj.Pulumi.Dokploy;

static class Contract
{
    static void GitHub()
    {
        var args = new Dokploy.GitHubIntegrationArgs { Name = "existing", GithubAppName = "app" };
        var options = new CustomResourceOptions { ImportId = "relation-id-placeholder" };
        var resource = new Dokploy.GitHubIntegration("existing", args, options);
        _ = resource.Name; _ = resource.GithubAppName; _ = resource.GithubId; _ = resource.GitProviderId; _ = resource.OrganizationId;
    }

    static void Gitea()
    {
        var args = new Dokploy.GiteaIntegrationArgs
        {
            Name = "gitea", GiteaUrl = "https://gitea.example.invalid", ClientId = "client-id",
            ClientSecret = "secret-placeholder", RedirectUri = "https://dokploy.example.invalid/callback",
            GiteaInternalUrl = "https://internal.example.invalid", OrganizationName = "engineering"
        };
        var resource = new Dokploy.GiteaIntegration("gitea", args, new CustomResourceOptions { ImportId = "relation-id-placeholder" });
        _ = resource.Name; _ = resource.GiteaUrl; _ = resource.ClientId; _ = resource.ClientSecret; _ = resource.RedirectUri;
        _ = resource.GiteaInternalUrl; _ = resource.OrganizationName; _ = resource.GiteaId; _ = resource.GitProviderId; _ = resource.OrganizationId;
    }

    static void Bitbucket()
    {
        var apiTokenArgs = new Dokploy.BitbucketIntegrationArgs
        {
            Name = "bitbucket-api", ApiToken = "token-placeholder", BitbucketEmail = "user@example.invalid", BitbucketWorkspaceName = "workspace"
        };
        var appPasswordArgs = new Dokploy.BitbucketIntegrationArgs
        {
            Name = "bitbucket-app-password", AppPassword = "password-placeholder", BitbucketUsername = "legacy-user"
        };
        var api = new Dokploy.BitbucketIntegration("bitbucket-api", apiTokenArgs, new CustomResourceOptions { ImportId = "relation-id-placeholder" });
        var appPassword = new Dokploy.BitbucketIntegration("bitbucket-app-password", appPasswordArgs, new CustomResourceOptions { ImportId = "relation-id-placeholder" });
        _ = api.Name; _ = api.ApiToken; _ = api.AppPassword; _ = api.BitbucketEmail; _ = api.BitbucketUsername; _ = api.BitbucketWorkspaceName;
        _ = api.BitbucketId; _ = api.GitProviderId; _ = api.OrganizationId; _ = appPassword.Name; _ = appPassword.BitbucketId;
    }
}
