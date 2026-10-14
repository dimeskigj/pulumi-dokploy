from pulumi_dokploy.git_hub_integration import GitHubIntegration, GitHubIntegrationArgs

args = GitHubIntegrationArgs(name="existing", github_app_name="app")
assert args.name is not None and args.github_app_name is not None
for prop in ("name", "github_app_name", "github_id", "git_provider_id", "organization_id"):
    assert hasattr(GitHubIntegration, prop)

from pulumi_dokploy.gitea_integration import GiteaIntegration, GiteaIntegrationArgs

gitea_args = GiteaIntegrationArgs(
    name="gitea",
    gitea_url="https://gitea.example.invalid",
    client_id="client-id",
    client_secret="secret-placeholder",
    redirect_uri="https://dokploy.example.invalid/callback",
    gitea_internal_url="https://internal.example.invalid",
    organization_name="engineering",
)
assert gitea_args.gitea_internal_url is not None and gitea_args.organization_name is not None
for prop in ("name", "gitea_url", "client_id", "client_secret", "redirect_uri", "gitea_internal_url", "organization_name", "gitea_id", "git_provider_id", "organization_id"):
    assert hasattr(GiteaIntegration, prop)

from pulumi_dokploy.bitbucket_integration import BitbucketIntegration, BitbucketIntegrationArgs

api_token_args = BitbucketIntegrationArgs(name="bitbucket-api", api_token="token-placeholder", bitbucket_email="user@example.invalid", bitbucket_workspace_name="workspace")
app_password_args = BitbucketIntegrationArgs(name="bitbucket-app-password", app_password="password-placeholder", bitbucket_username="legacy-user")
assert api_token_args.api_token is not None and api_token_args.bitbucket_email is not None
assert app_password_args.app_password is not None and app_password_args.bitbucket_username is not None
for prop in ("name", "api_token", "app_password", "bitbucket_email", "bitbucket_username", "bitbucket_workspace_name", "bitbucket_id", "git_provider_id", "organization_id"):
    assert hasattr(BitbucketIntegration, prop)
