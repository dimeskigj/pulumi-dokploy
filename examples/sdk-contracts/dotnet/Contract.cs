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
}
